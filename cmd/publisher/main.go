package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lukostrobl/fathom/internal/config"
	"github.com/lukostrobl/fathom/internal/db"
	applog "github.com/lukostrobl/fathom/internal/log"
	"github.com/lukostrobl/fathom/internal/metrics"
)

type Config struct {
	config.BasicConfig
	Database struct {
		URL string `koanf:"url"`
	} `koanf:"database"`
}

func (c Config) GetBasicConfig() config.BasicConfig { return c.BasicConfig }

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New(usageText())
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(os.Stdout, usageText())
		return nil
	case "rollup", "emit":
		// known — fall through
	default:
		return fmt.Errorf("unknown subcommand %q\n\n%s", cmd, usageText())
	}

	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "local"
	}
	cfg, err := config.ParseConfig[Config]("publisher", env)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	logger := applog.New(cfg.BasicConfig)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer pool.Close()

	switch cmd {
	case "rollup":
		fs := flag.NewFlagSet("rollup", flag.ExitOnError)
		ethPrices := fs.String("eth-prices", "data/eth-usd-weekly.json",
			"weekly ETH/USD reference price file, keyed by ISO Monday week-start (consumed by the gas rollup)")
		if err := fs.Parse(args); err != nil {
			return err
		}
		prices, err := metrics.LoadETHPrices(*ethPrices)
		if err != nil {
			return err
		}
		logger.Info("publisher: rebuilding metrics tables", "eth_prices", *ethPrices)
		if err := metrics.Rebuild(ctx, pool, prices); err != nil {
			return err
		}
		logger.Info("publisher: rollup complete")
		return nil
	case "emit":
		fs := flag.NewFlagSet("emit", flag.ExitOnError)
		outDir := fs.String("out", "dist", "directory to write JSON artifacts into")
		claimsPath := fs.String("claims", "data/claims.json", "curated claim ledger file")
		if err := fs.Parse(args); err != nil {
			return err
		}
		claims, err := metrics.LoadClaims(*claimsPath)
		if err != nil {
			return err
		}
		logger.Info("publisher: emitting artifacts", "out", *outDir, "claims", *claimsPath)
		if err := metrics.Emit(ctx, pool, *outDir, claims, time.Now); err != nil {
			return err
		}
		logger.Info("publisher: emit complete", "out", *outDir)
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q", cmd)
	}
}

func usageText() string {
	return `usage: publisher <subcommand> [flags]

subcommands:
  rollup --eth-prices FILE   recompute all metrics tables from payment_x402_v1
                             (default FILE=data/eth-usd-weekly.json)
  emit   --out DIR --claims FILE
                             write dashboard JSON artifacts
                             (defaults DIR=dist, FILE=data/claims.json)

run rollup after a backfill, then emit. config: config/publisher/{env}.toml + env (DATABASE__URL)
`
}
