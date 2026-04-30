package server

import (
	"context"
	"log/slog"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

type Server struct {
	e    *echo.Echo
	port string
}

func (s *Server) Start(ctx context.Context) error {
	return echo.StartConfig{Address: ":" + s.port}.Start(ctx, s.e)
}

func (s *Server) GET(path string, handler func(c *echo.Context) error) echo.RouteInfo {
	return s.e.GET(path, handler)
}

func (s *Server) POST(path string, handler func(c *echo.Context) error) echo.RouteInfo {
	return s.e.POST(path, handler)
}

func New(port string) *Server {
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

	return &Server{e: e, port: port}
}
