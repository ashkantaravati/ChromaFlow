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
	ResultTTL   int    `env:"RESULT_TTL" envDefault:"3600"` // seconds (reserved for future expiry workers)
	ChromeWSURL string `env:"CHROME_WS_URL" envDefault:""`  // empty = rod auto-launches a reusable browser

	QueueBackend   string `env:"QUEUE_BACKEND" envDefault:"memory"` // memory or redis
	StorageBackend string `env:"STORAGE_BACKEND" envDefault:"memory"`
	RedisURL       string `env:"REDIS_URL" envDefault:"redis://localhost:6379/0"`
	RedisKeyPrefix string `env:"REDIS_KEY_PREFIX" envDefault:"chromaflow"`

	BlobBackend       string `env:"BLOB_BACKEND" envDefault:"memory"` // memory or s3
	S3Endpoint        string `env:"S3_ENDPOINT" envDefault:"localhost:9000"`
	S3AccessKeyID     string `env:"S3_ACCESS_KEY_ID" envDefault:"minioadmin"`
	S3SecretAccessKey string `env:"S3_SECRET_ACCESS_KEY" envDefault:"minioadmin"`
	S3Bucket          string `env:"S3_BUCKET" envDefault:"chromaflow-pdfs"`
	S3Region          string `env:"S3_REGION" envDefault:"us-east-1"`
	S3UseSSL          bool   `env:"S3_USE_SSL" envDefault:"false"`
}

func Load() *Config {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		slog.Error("failed to parse config", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if cfg.NumWorkers == 0 {
		cfg.NumWorkers = runtime.NumCPU() * 2
	}
	return cfg
}
