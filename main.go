package main

import (
	"context"
	"encoding/json"
	"errors"
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

	gridStateService := NewGridStateService(
		cfg.HABaseURL+"/api/states/"+cfg.HAEntity,
		cfg.HAToken,
		cfg.HAPollInterval,
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
	gridStateService.Start(ctx, []func(state string){
		gridHistoryService.OnHistoryUpdate(ctx),
		func(state string) {
			lock.Lock()
			defer lock.Unlock()
			data.Grid = state
		},
	})

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

	e.GET("/api/state", func(c *echo.Context) error {
		lock.Lock()
		defer lock.Unlock()
		return c.JSON(http.StatusOK, data)
	})

	sc := echo.StartConfig{Address: ":" + cfg.Port}
	slog.Info("starting server", "version", version, "address", ":"+cfg.Port)
	if err := sc.Start(ctx, e); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "error", err)
	}
}
