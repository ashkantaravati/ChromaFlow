package observability

import (
	"log/slog"
	"os"
)

func NewLogger(service string, version string) *slog.Logger {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: false,
		Level:     slog.LevelInfo,
	})).With(
		slog.String("service", service),
		slog.String("version", version),
	)
	slog.SetDefault(logger)
	return logger
}
