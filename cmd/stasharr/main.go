package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/mononen/stasharr/internal/api"
	"github.com/mononen/stasharr/internal/clients/prowlarr"
	"github.com/mononen/stasharr/internal/clients/sabnzbd"
	"github.com/mononen/stasharr/internal/clients/stashapp"
	"github.com/mononen/stasharr/internal/clients/stashdb"
	"github.com/mononen/stasharr/internal/config"
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

	// TODO: Run migrations

	// Load app config from DB
	appCfg, err := config.LoadFromDB(ctx, pool)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load app config")
	}

	// Initialize clients
	prowlarrClient := prowlarr.New(appCfg)
	sabnzbdClient := sabnzbd.New(appCfg)
	stashappClient := stashapp.New(appCfg)
	stashdbClient := stashdb.New(appCfg)

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
		AppName: "Stasharr",
	})
	api.RegisterRoutes(fiberApp, app)

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
