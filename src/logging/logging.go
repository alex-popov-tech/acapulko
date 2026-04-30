package logging

import (
	"log/slog"
	"os"
)

func init() {
	Setup("info")
}

func Setup(level string) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(level),
	})))
}

func New(service string) func() *slog.Logger {
	return func() *slog.Logger {
		return slog.Default().With("service", service)
	}
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
