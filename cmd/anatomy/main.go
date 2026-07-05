// Command anatomy serves the Anatomy dossier API and embedded frontend,
// and provides an offline rollup subcommand to rebuild entity tables.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lukostrobl/fathom/internal/anatomy"
	"github.com/lukostrobl/fathom/internal/config"
	"github.com/lukostrobl/fathom/internal/db"
	applog "github.com/lukostrobl/fathom/internal/log"
	anatomyweb "github.com/lukostrobl/fathom/web/anatomy"
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
	// Default subcommand is serve so `anatomy --addr :8090` stays valid.
	cmd := "serve"
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd = args[0]
		args = args[1:]
	}
	switch cmd {
	case "help":
		_, _ = fmt.Fprint(os.Stdout, usageText())
		return nil
	case "serve", "rollup":
		// known — fall through
	default:
		return fmt.Errorf("unknown subcommand %q\n\n%s", cmd, usageText())
	}

	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "local"
	}
	cfg, err := config.ParseConfig[Config]("anatomy", env)
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
		labelsPath := fs.String("labels", "data/entity-labels.json", "curated entity label file")
		if err := fs.Parse(args); err != nil {
			return err
		}
		labels, err := anatomy.LoadManualLabels(*labelsPath)
		if err != nil {
			return err
		}
		logger.Info("anatomy: rebuilding entity tables", "labels", *labelsPath)
		if err := anatomy.Rollup(ctx, pool, labels); err != nil {
			return err
		}
		logger.Info("anatomy: rollup complete")
		return nil
	default: // serve
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		addr := fs.String("addr", ":8090", "listen address")
		if err := fs.Parse(args); err != nil {
			return err
		}
		return serve(ctx, cfg, pool, logger, *addr)
	}
}

func serve(ctx context.Context, cfg Config, pool *pgxpool.Pool, logger *slog.Logger, addr string) error {
	pgEntity := anatomy.NewPgEntity(pool)
	srv := anatomy.NewServer(
		anatomy.Providers{
			Dossier:     anatomy.NewPgDossier(pool),
			Meta:        anatomy.NewPgMeta(pool),
			Entity:      pgEntity,
			Neighbors:   pgEntity,
			Activity:    pgEntity,
			Lists:       pgEntity,
			Leaderboard: anatomy.NewPgLeaderboard(pool),
		},
		anatomyweb.Assets(),
		logger,
	)
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv,
		ReadHeaderTimeout: 5 * time.Second,
	}
	// shutdown requires a fresh context after signal ctx is cancelled
	go func() { //nolint:gosec // G118: signal ctx is cancelled before shutdown
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
	}()
	logger.Info("anatomy: listening", "addr", addr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

func usageText() string {
	return `usage: anatomy [subcommand] [flags]

subcommands:
  serve  --addr :8090          serve the dossier API + embedded UI (default)
  rollup --labels FILE         rebuild the entity tables from payment_x402_v1
                               (default FILE=data/entity-labels.json)

config: config/anatomy/{env}.toml + env (DATABASE__URL)
`
}
