package observability

import (
	"log/slog"
	"os"
	"strings"
)

func NewLogger(service string, version string, level string) *slog.Logger {
	logLevel := parseLogLevel(level)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: false,
		Level:     logLevel,
	})).With(
		slog.String("service", service),
		slog.String("version", version),
	)
	slog.SetDefault(logger)
	return logger
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
