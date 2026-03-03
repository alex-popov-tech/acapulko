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
	Demo    bool          `json:"demo,omitempty"`
}

func demoState(grid, address string) PowerState {
	now := nowKyiv()
	h := func(dur time.Duration) datetime { return datetime{now.Add(-dur)} }
	tp := func(dur time.Duration) *datetime { d := h(dur); return &d }

	history := []HistoryItem{
		{State: "off", From: h(23 * time.Hour), To: tp(21*time.Hour + 45*time.Minute)},
		{State: "on", From: h(21*time.Hour + 45*time.Minute), To: tp(18 * time.Hour)},
		{State: "off", From: h(18 * time.Hour), To: tp(14*time.Hour + 30*time.Minute)},
		{State: "on", From: h(14*time.Hour + 30*time.Minute), To: tp(6 * time.Hour)},
		{State: "off", From: h(6 * time.Hour), To: tp(4*time.Hour + 45*time.Minute)},
	}

	state := PowerState{
		Grid:    grid,
		Address: address,
		Version: version,
		Demo:    true,
	}

	if grid == "on" {
		history = append(history, HistoryItem{
			State: "on",
			From:  h(4*time.Hour + 45*time.Minute),
		})
	} else {
		history = append(history,
			HistoryItem{
				State: "on",
				From:  h(4*time.Hour + 45*time.Minute),
				To:    tp(35 * time.Minute),
			},
			HistoryItem{
				State: "off",
				From:  h(35 * time.Minute),
			},
		)
		outFrom := h(35 * time.Minute)
		outTo := datetime{now.Add(1*time.Hour + 25*time.Minute)}
		state.Outage = &Outage{Type: "emergency", From: &outFrom, To: &outTo}
	}

	state.History = history
	return state
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

	gridHistoryService.Start([]func(state []HistoryItem){
		func(state []HistoryItem) {
			lock.Lock()
			defer lock.Unlock()
			data.History = state
		},
	})
	data.History = gridHistoryService.State()

	outageService.Start(ctx, []func(o *Outage){func(o *Outage) {
		lock.Lock()
		defer lock.Unlock()
		data.Outage = o
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
		return c.JSON(http.StatusOK, data)
	})

	e.POST("/api/webhook/grid", func(c *echo.Context) error {
		var payload struct {
			State string `json:"state"`
		}
		if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		}
		if payload.State != "on" && payload.State != "off" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "state must be 'on' or 'off'"})
		}

		lock.Lock()
		changed := data.Grid != payload.State
		data.Grid = payload.State
		lock.Unlock()

		if changed {
			slog.Info("grid state changed via webhook", "state", payload.State)
			go historyUpdateFn(payload.State)
		}

		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	e.GET("/example/on", func(c *echo.Context) error {
		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return tmpl.Execute(c.Response(), demoState("on", data.Address))
	})
	e.GET("/example/off", func(c *echo.Context) error {
		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return tmpl.Execute(c.Response(), demoState("off", data.Address))
	})

	sc := echo.StartConfig{Address: ":" + cfg.Port}
	slog.Info("starting server", "version", version, "address", ":"+cfg.Port)
	if err := sc.Start(ctx, e); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "error", err)
	}
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
