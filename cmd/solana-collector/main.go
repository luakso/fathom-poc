package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/lukostrobl/fathom/internal/config"
	"github.com/lukostrobl/fathom/internal/db"
	applog "github.com/lukostrobl/fathom/internal/log"
)

type Config struct {
	config.BasicConfig
	Database struct {
		URL string `koanf:"url"`
	} `koanf:"database"`
}

func (c Config) GetBasicConfig() config.BasicConfig { return c.BasicConfig }

func main() {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "local"
	}

	cfg, err := config.ParseConfig[Config]("solana-collector", env)
	if err != nil {
		slog.Error("parse config", "err", err)
		os.Exit(1)
	}

	logger := applog.New(cfg.BasicConfig)

	ctx := context.Background()
	pool, err := db.Open(ctx, cfg.Database.URL)
	if err != nil {
		logger.Error("open db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	logger.Info("solana-collector ready")
}
