package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/alex-popov-tech/acapulko/src/config"
	"github.com/alex-popov-tech/acapulko/src/logging"
	"github.com/alex-popov-tech/acapulko/src/server"
	"github.com/alex-popov-tech/acapulko/src/services/dtek"
	"github.com/alex-popov-tech/acapulko/src/services/homeassistant"
	"github.com/alex-popov-tech/acapulko/src/services/telegram"
)

var version = "dev"

type AppState struct {
	Outage  *dtek.Outage `json:"outage"`
	Grid    string       `json:"grid"` // on/off
	Address string       `json:"address"`
	Version string       `json:"version"`
}

func formatDuration(hours, minutes, seconds int) string {
	switch {
	case hours > 0:
		if minutes > 0 {
			return fmt.Sprintf("%d год %d хв", hours, minutes)
		}
		return fmt.Sprintf("%d год", hours)
	case minutes > 0:
		if seconds > 0 {
			return fmt.Sprintf("%d хв %d сек", minutes, seconds)
		}
		return fmt.Sprintf("%d хв", minutes)
	default:
		return fmt.Sprintf("%d сек", seconds)
	}
}

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	c, err := config.Load()
	if err != nil {
		slog.Error("startup failed", "stage", "config", "error", err)
		return err
	}

	logging.Setup(c.LogLevel)

	var stateLock sync.RWMutex
	state := AppState{
		Outage:  nil,
		Grid:    "pending",
		Address: c.DTEKCity + ", " + c.DTEKStreet + ", " + c.DTEKBuilding,
		Version: version,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s := server.New(c.Port)
	tg := telegram.New(c.TGBaseURL, c.TGBotToken)

	outageUpdates := make(chan *dtek.Outage)
	initialOutage, err := dtek.Start(
		ctx,
		c.DTEKBaseURL,
		c.DTEKRegion,
		c.DTEKCity,
		c.DTEKStreet,
		c.DTEKBuilding,
		c.DTEKPollInterval,
		outageUpdates,
	)
	if err != nil {
		slog.Error("startup failed", "stage", "dtek", "error", err)
		return err
	}

	state.Outage = initialOutage
	slog.Info("dtek initial outage fetched", "outage_present", initialOutage != nil)
	go outageHandler(outageUpdates, &state, &stateLock, tg, c.TGChatID)

	gridUpdates := make(chan homeassistant.WebhookPayload)
	initialGridState, err := homeassistant.Attach(
		s,
		c.HABaseURL,
		c.HAToken,
		gridUpdates,
	)
	if err != nil {
		slog.Error("startup failed", "stage", "homeassistant", "error", err)
		return err
	}

	state.Grid = initialGridState.State
	slog.Info("homeassistant attached", "initial_state", initialGridState.State)
	go gridUpdateHandler(gridUpdates, &state, &stateLock, c.TGChatID, tg)

	s.GET("/api/state", func(c *echo.Context) error {
		stateLock.RLock()
		defer stateLock.RUnlock()
		b, err := json.Marshal(state)
		if err != nil {
			return c.JSONBlob(http.StatusInternalServerError, b)
		}
		return c.JSONBlob(http.StatusOK, b)
	})

	s.GET("/api/state/stream", func(c *echo.Context) error {
		userid := c.QueryParam("userid")
		slog.Info("SSE client connected", "ip", c.RealIP(), "userid", userid)

		w := c.Response()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		t := time.NewTicker(time.Second * 5)
		defer t.Stop()
		for {
			select {
			case <-c.Request().Context().Done():
				slog.Info("SSE client disconnected", "ip", c.RealIP(), "userid", userid)
				return nil
			case <-t.C:
				stateLock.RLock()
				b, err := json.Marshal(state)
				stateLock.RUnlock()
				if err != nil {
					return err
				}
				event := server.Event{Data: b}
				if err := event.MarshalTo(w); err != nil {
					return err
				}
				if err := http.NewResponseController(w).Flush(); err != nil {
					return err
				}
			}
		}
	})

	slog.Info("starting server", "port", c.Port, "version", version, "log_level", c.LogLevel)
	if err = s.Start(ctx); err != nil {
		slog.Error("server exited", "error", err)
		return err
	}
	return nil
}

func outageHandler(
	outages <-chan *dtek.Outage,
	state *AppState,
	lock *sync.RWMutex,
	tg *telegram.Client,
	tgChatID string,
) {
	slog.Info("outage handler started")
	for o := range outages {
		go func() {
			lock.Lock()
			state.Outage = o
			lock.Unlock()

			message := ""
			if o == nil {
				slog.Info("outage cleared")
				message = "✅ ДТЕК: аварійне відключення за адресою відсутнє"
			} else {
				slog.Info("outage detected",
					"type", o.Type,
					"from", o.From.Format("02.01.2006 15:04"),
					"to", o.To.Format("02.01.2006 15:04"),
				)
				message = fmt.Sprintf(
					"⚠️ ДТЕК: зафіксоване аварійне відключення з %s до %s",
					o.From.Format("02.01.2006 15:04"),
					o.To.Format("02.01.2006 15:04"),
				)
			}
			go tg.SendMessage(tgChatID, message)
		}()
	}
}

func gridUpdateHandler(
	gridUpdates <-chan homeassistant.WebhookPayload,
	state *AppState,
	lock *sync.RWMutex,
	tgChatID string,
	tg *telegram.Client,
) {
	slog.Info("grid handler started")
	for gridEvent := range gridUpdates {
		go func() {
			lock.Lock()
			prev := state.Grid
			state.Grid = gridEvent.State
			lock.Unlock()

			duration := formatDuration(gridEvent.Hours, gridEvent.Minutes, gridEvent.Seconds)

			slog.Info("grid state changed",
				"from", prev,
				"to", gridEvent.State,
				"duration", duration,
			)

			var message string
			if gridEvent.State == "on" {
				message = fmt.Sprintf(
					"🔌 Електропостачання відновлено\n⏱ Світла не було %s",
					duration,
				)
			} else {
				message = fmt.Sprintf("⚡ Зафіксовано відключення\n⏱ Світло було %s", duration)
			}
			go tg.SendMessage(tgChatID, message)
		}()
	}
}
