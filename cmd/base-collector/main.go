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
	Base BaseConfig `koanf:"base"`
}

// BaseConfig holds chain + endpoint configuration. Defaults are applied at use
// site (rpc.go / backfill.go) when a field is its zero value.
type BaseConfig struct {
	RPCURL               string `koanf:"rpc_url"`
	HyperSyncURL         string `koanf:"hypersync_url"`
	HyperSyncBearerToken string `koanf:"hypersync_bearer_token"`

	// Live tuning knobs (zero = default)
	ConfirmationDepth uint64 `koanf:"confirmation_depth"`
	PollIntervalMs    int    `koanf:"poll_interval_ms"`
	BlockBatchSize    uint64 `koanf:"block_batch_size"`
	Concurrency       int64  `koanf:"concurrency"`

	// Backfill tuning knob
	BatchCommitSize int `koanf:"batch_commit_size"`
}

func (c Config) GetBasicConfig() config.BasicConfig { return c.BasicConfig }

func main() {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "local"
	}

	cfg, err := config.ParseConfig[Config]("base-collector", env)
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

	logger.Info("base-collector ready")
}
