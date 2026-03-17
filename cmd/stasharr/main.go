package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os/signal"
	"syscall"

	"github.com/mononen/stasharr/internal/api"
	"github.com/mononen/stasharr/internal/clients/prowlarr"
	"github.com/mononen/stasharr/internal/clients/sabnzbd"
	"github.com/mononen/stasharr/internal/clients/stashapp"
	"github.com/mononen/stasharr/internal/clients/stashdb"
	"github.com/mononen/stasharr/internal/config"
	"github.com/mononen/stasharr/internal/db/migrations"
	"github.com/mononen/stasharr/internal/models"
	"github.com/mononen/stasharr/internal/worker"
)

func main() {
	// Configure zerolog
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()

	// Load env config
	envCfg, err := config.LoadEnv()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load environment config")
	}

	// Connect to database
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, envCfg.DatabaseDSN)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatal().Err(err).Msg("database ping failed")
	}

	// Run migrations
	var migrationFS fs.FS = migrations.Files
	if p := os.Getenv("STASHARR_MIGRATIONS_PATH"); p != "" {
		migrationFS = os.DirFS(p)
	}
	if err := runMigrations(ctx, pool, migrationFS); err != nil {
		log.Warn().Err(err).Msg("migrations failed — continuing anyway")
	}

	// Load app config from DB
	appCfg, err := config.LoadFromDB(ctx, pool)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load app config")
	}

	// Populate in-memory config from DB (LoadFromDB is currently a stub).
	rows, rowErr := pool.Query(ctx, "SELECT key, value FROM config")
	if rowErr == nil {
		for rows.Next() {
			var key, value string
			if rows.Scan(&key, &value) == nil {
				appCfg.Set(key, value)
			}
		}
		rows.Close()
	}

	// Initialize clients
	prowlarrClient := prowlarr.New(appCfg.Get("prowlarr.url"), appCfg.Get("prowlarr.api_key"))
	sabnzbdClient := sabnzbd.New(appCfg.Get("sabnzbd.url"), appCfg.Get("sabnzbd.api_key"), appCfg.Get("sabnzbd.category"))
	stashappClient := stashapp.New(appCfg.Get("stashapp.url"), appCfg.Get("stashapp.api_key"))
	stashdbClient := stashdb.New(appCfg.Get("stashdb.api_key"), nil)

	// Build App container
	app := &models.App{
		DB:       pool,
		Config:   appCfg,
		Prowlarr: prowlarrClient,
		SABnzbd:  sabnzbdClient,
		StashApp: stashappClient,
		StashDB:  stashdbClient,
	}

	// Start worker supervisor
	supervisor := worker.NewSupervisor(app)
	app.Supervisor = supervisor
	go supervisor.Start(ctx)

	// Setup Fiber
	fiberApp := fiber.New(fiber.Config{
		AppName:      "Stasharr",
		ErrorHandler: api.ErrorHandler,
	})

	api.RegisterRoutes(fiberApp, app, envCfg.SecretKey, envCfg.DevMode)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		port := envCfg.Port
		if port == "" {
			port = "8080"
		}
		if err := fiberApp.Listen(":" + port); err != nil {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	<-quit
	log.Info().Msg("shutting down")
	supervisor.Stop()
	_ = fiberApp.Shutdown()
}

// runMigrations applies any unapplied *.up.sql files in migrationFS.
func runMigrations(ctx context.Context, pool *pgxpool.Pool, migrationFS fs.FS) error {
	// Create migrations tracking table if absent.
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// Discover migration files.
	entries, err := fs.ReadDir(migrationFS, ".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	type migration struct {
		version  int
		filename string
	}
	var migrations []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		var ver int
		if _, parseErr := fmt.Sscanf(e.Name(), "%d_", &ver); parseErr != nil {
			continue
		}
		migrations = append(migrations, migration{version: ver, filename: e.Name()})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })

	// Collect applied versions.
	applied := map[int]bool{}
	vrows, vErr := pool.Query(ctx, "SELECT version FROM schema_migrations")
	if vErr == nil {
		for vrows.Next() {
			var v int
			if vrows.Scan(&v) == nil {
				applied[v] = true
			}
		}
		vrows.Close()
	}

	// Apply missing migrations.
	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		sql, readErr := fs.ReadFile(migrationFS, m.filename)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", m.filename, readErr)
		}
		if _, execErr := pool.Exec(ctx, string(sql)); execErr != nil {
			return fmt.Errorf("apply %s: %w", m.filename, execErr)
		}
		if _, insertErr := pool.Exec(ctx,
			"INSERT INTO schema_migrations (version) VALUES ($1)",
			strconv.Itoa(m.version),
		); insertErr != nil {
			return fmt.Errorf("record migration %d: %w", m.version, insertErr)
		}
		log.Info().Str("file", m.filename).Msg("applied migration")
	}

	return nil
}
