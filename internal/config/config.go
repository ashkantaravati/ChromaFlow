package config

import (
	"log/slog"
	"os"
	"runtime"

	"github.com/caarlos0/env/v10"
)

type Config struct {
	Port        int    `env:"PORT" envDefault:"8080"`
	NumWorkers  int    `env:"NUM_WORKERS" envDefault:"0"` // 0 = auto
	QueueSize   int    `env:"QUEUE_SIZE" envDefault:"100"`
	PageTimeout int    `env:"PAGE_TIMEOUT" envDefault:"30"` // seconds
	ResultTTL   int    `env:"RESULT_TTL" envDefault:"3600"` // seconds (not used in v0, but future-proof)
	ChromeWSURL string `env:"CHROME_WS_URL" envDefault:""`  // empty = rod auto-launches
}

func Load() *Config {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		slog.Error("failed to parse config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Auto-detect workers
	if cfg.NumWorkers == 0 {
		cfg.NumWorkers = runtime.NumCPU() * 2
	}

	return cfg
}
