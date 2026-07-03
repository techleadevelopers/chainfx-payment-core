package logger

import (
	"log/slog"
	"os"
	"strings"
)

func Configure() {
	level := new(slog.LevelVar)
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		level.Set(slog.LevelDebug)
	case "warn":
		level.Set(slog.LevelWarn)
	case "error":
		level.Set(slog.LevelError)
	default:
		level.Set(slog.LevelInfo)
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     level,
		AddSource: strings.EqualFold(os.Getenv("LOG_SOURCE"), "true"),
	})
	slog.SetDefault(slog.New(handler))
}
