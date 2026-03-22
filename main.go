package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

var version = "dev"

type PowerState struct {
	Outage  *Outage       `json:"outage"`
	Grid    string        `json:"grid"` // on/off
	History []HistoryItem `json:"history"`
	Address string        `json:"address"`
	Version string        `json:"version"`
}

func main() {
	_ = godotenv.Load()
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	telegram := &Telegram{config: cfg}

	subs := &sync.Map{}

	if err := sentry.Init(sentry.ClientOptions{
		Dsn:         cfg.SentryDSN,
		Environment: cfg.SentryEnv,
		Release:     version,
	}); err != nil {
		log.Fatalf("sentry.Init: %s", err)
	}
	defer sentry.Flush(2 * time.Second)

	data := PowerState{
		Outage:  nil,
		Grid:    "pending",
		History: []HistoryItem{},
		Address: cfg.DTEKCity + ", " + cfg.DTEKStreet + ", " + cfg.DTEKBuilding,
		Version: version,
	}
	dataJSON, _ := json.Marshal(data)
	lock := sync.Mutex{}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	outageService := NewOutageService(
		cfg.DTEKBaseURL+"/api/status",
		cfg.DTEKRegion,
		cfg.DTEKCity,
		cfg.DTEKStreet,
		cfg.DTEKBuilding,
		cfg.DTEKPollInterval,
	)

	gridHistoryService := NewGridHistoryService(cfg.HistoryFilePath, cfg.HistoryWindow)

	gridHistoryService.Start(ctx, []func(state []HistoryItem){
		func(state []HistoryItem) {
			lock.Lock()
			defer lock.Unlock()
			data.History = state
			dataJSON, _ = json.Marshal(data)
			broadcast(subs, dataJSON)
		},
	})
	data.History = gridHistoryService.State()

	outageService.Start(ctx, []func(o *Outage){func(o *Outage) {
		lock.Lock()
		if o != data.Outage {
			reportOutageToTelegram(telegram, o)
		}
		data.Outage = o
		dataJSON, _ = json.Marshal(data)
		broadcast(subs, dataJSON)
		defer lock.Unlock()
	}})

	// Prepare history update callback for webhook handler
	historyUpdateFn := gridHistoryService.OnHistoryUpdate(ctx)

	// One-shot fetch of initial grid state from HA
	haEntityURL := cfg.HABaseURL + "/api/states/" + cfg.HAEntity
	initialState, err := fetchInitialGridState(ctx, haEntityURL, cfg.HAToken)
	if err != nil {
		slog.Warn("initial grid state fetch failed, starting as pending", "error", err)
	} else {
		lock.Lock()
		data.Grid = initialState
		dataJSON, _ = json.Marshal(data)
		lock.Unlock()
		go historyUpdateFn(initialState)
	}

	tmpl := template.Must(
		template.New("index.html").Funcs(template.FuncMap{
			"json": func(v any) template.JS {
				b, _ := json.Marshal(v)
				return template.JS(b) //nolint:gosec // data is server-controlled, not user input
			},
		}).ParseFiles("templates/index.html"),
	)

	e := echo.New()

	e.Use(middleware.Recover())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogMethod:   true,
		LogURIPath:  true,
		LogStatus:   true,
		LogLatency:  true,
		LogRemoteIP: true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			slog.Info("request",
				"method", v.Method,
				"path", v.URIPath,
				"status", v.Status,
				"latency", v.Latency.String(),
				"ip", v.RemoteIP,
			)
			return nil
		},
	}))

	e.GET("/", func(c *echo.Context) error {
		lock.Lock()
		snapshot := data
		lock.Unlock()
		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return tmpl.Execute(c.Response(), snapshot)
	})
	e.Static("/static", "static")

	// Serve service worker from root so its scope covers the entire app
	e.GET("/sw.js", func(c *echo.Context) error {
		return c.File("static/sw.js")
	})

	// Serve icons at conventional root paths that browsers probe automatically
	e.GET("/favicon.ico", func(c *echo.Context) error {
		return c.File("static/icons/icon-192.png")
	})
	e.GET("/apple-touch-icon.png", func(c *echo.Context) error {
		return c.File("static/icons/icon-192.png")
	})
	e.GET("/apple-touch-icon-precomposed.png", func(c *echo.Context) error {
		return c.File("static/icons/icon-192.png")
	})

	e.GET("/api/state", func(c *echo.Context) error {
		lock.Lock()
		defer lock.Unlock()
		return c.JSONBlob(http.StatusOK, dataJSON)
	})

	e.GET("/api/state/stream", func(c *echo.Context) error {
		userid := c.QueryParam("userid")
		slog.Info("SSE client connected", "ip", c.RealIP(), "userid", userid)
		subs.Store(userid, c.Response())

		w := c.Response()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		err = pushEvent(dataJSON, w)
		if err != nil {
			return err
		}

		<-c.Request().Context().Done()
		subs.Delete(userid)
		slog.Info("SSE client disconnected", "ip", c.RealIP(), "userid", userid)
		return nil
	})

	e.POST("/api/webhook/grid", func(c *echo.Context) error {
		var payload struct {
			State   string `json:"state"`
			Hours   int    `json:"hours"`
			Minutes int    `json:"minutes"`
			Seconds int    `json:"seconds"`
		}
		if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		}
		if payload.State != "on" && payload.State != "off" {
			return c.JSON(
				http.StatusBadRequest,
				map[string]string{"error": "state must be 'on' or 'off'"},
			)
		}

		const gridChangeThreshold = 10 // seconds; ignore sub-threshold flickers

		durationSec := payload.Hours*3600 + payload.Minutes*60 + payload.Seconds

		lock.Lock()
		changed := data.Grid != payload.State && durationSec > gridChangeThreshold
		if changed {
			data.Grid = payload.State
			dataJSON, _ = json.Marshal(data)
		}
		lock.Unlock()

		if changed {
			slog.Info("grid state changed via webhook", "state", payload.State)
			broadcast(subs, dataJSON)
			go historyUpdateFn(payload.State)
			go reportGridStateToTelegram(telegram, payload.State, payload.Hours, payload.Minutes, payload.Seconds)
		}

		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	sc := echo.StartConfig{Address: ":" + cfg.Port}
	slog.Info("starting server", "version", version, "address", ":"+cfg.Port)
	if err := sc.Start(ctx, e); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "error", err)
	}
}

func broadcast(subs *sync.Map, data []byte) {
	subs.Range(func(key, value any) bool {
		if err := pushEvent(data, value.(http.ResponseWriter)); err != nil {
			slog.Error("failed to send event", "error", err)
		}
		return true
	})
}

func pushEvent(data []byte, w http.ResponseWriter) error {
	event := Event{Data: data}
	if err := event.MarshalTo(w); err != nil {
		return err
	}
	if err := http.NewResponseController(w).Flush(); err != nil {
		return err
	}
	return nil
}

func reportOutageToTelegram(t *Telegram, o *Outage) {
	message := ""
	if o == nil {
		message = "✅ ДТЕК: аварійне відключення за адресою відсутнє"
	} else {
		message = fmt.Sprintf(
			"⚠️ ДТЕК: зафіксоване аварійне відключення з %s до %s",
			o.From.Format("02.01.2006 15:04"),
			o.To.Format("02.01.2006 15:04"),
		)
	}

	t.sendMessage(message)
}

func reportGridStateToTelegram(t *Telegram, state string, hours, minutes, seconds int) {
	var duration string
	switch {
	case hours > 0:
		duration = fmt.Sprintf("%d год %d хв", hours, minutes)
	case minutes > 0:
		duration = fmt.Sprintf("%d хв %d сек", minutes, seconds)
	default:
		duration = fmt.Sprintf("%d сек", seconds)
	}

	var message string
	if state == "on" {
		message = fmt.Sprintf("🔌 Електропостачання відновлено\n⏱ Світла не було %s", duration)
	} else {
		message = fmt.Sprintf("⚡ Зафіксовано відключення\n⏱ Світло було %s", duration)
	}

	t.sendMessage(message)
}

func fetchInitialGridState(ctx context.Context, haURL, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", haURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("cannot construct HA request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req) //nolint:gosec // URL is from server config, not user input
	if err != nil {
		return "", fmt.Errorf("failed to GET grid state: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return "", fmt.Errorf("unexpected status code from HA: %d", res.StatusCode)
	}

	var body struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("failed to decode HA response: %w", err)
	}
	if body.State != "on" && body.State != "off" {
		return "", fmt.Errorf("unexpected grid state value: %s", body.State)
	}
	return body.State, nil
}
