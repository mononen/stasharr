package api

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/rs/zerolog/log"

	"github.com/mononen/stasharr/internal/models"
	"github.com/mononen/stasharr/internal/worker"
)

// AuthMiddleware validates the X-Api-Key header.
func AuthMiddleware(secretKey string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Method() == fiber.MethodOptions {
			return c.Next()
		}
		key := c.Get("X-Api-Key")
		if key == "" || key != secretKey {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "UNAUTHORIZED",
					"message": "Invalid or missing API key",
				},
			})
		}
		return c.Next()
	}
}

// AuthFromQuery validates the api_key query param (used for SSE since EventSource
// doesn't support custom headers).
func AuthFromQuery(secretKey string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Method() == fiber.MethodOptions {
			return c.Next()
		}
		key := c.Query("api_key")
		if key == "" || key != secretKey {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "UNAUTHORIZED",
					"message": "Invalid or missing API key",
				},
			})
		}
		return c.Next()
	}
}

// LoggingMiddleware logs structured request info via zerolog.
func LoggingMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		latency := time.Since(start)

		evt := log.Info().
			Str("method", c.Method()).
			Str("path", c.Path()).
			Int("status", c.Response().StatusCode()).
			Dur("latency_ms", latency)

		if jobID := c.Params("id"); jobID != "" {
			evt = evt.Str("job_id", jobID)
		}

		evt.Msg("http")
		return err
	}
}

// ErrorHandler is Fiber's global error handler. Maps errors to the error envelope shape.
func ErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}

	return c.Status(code).JSON(fiber.Map{
		"error": fiber.Map{
			"code":    httpCodeString(code),
			"message": err.Error(),
		},
	})
}

func httpCodeString(code int) string {
	switch code {
	case fiber.StatusBadRequest:
		return "BAD_REQUEST"
	case fiber.StatusUnauthorized:
		return "UNAUTHORIZED"
	case fiber.StatusForbidden:
		return "FORBIDDEN"
	case fiber.StatusNotFound:
		return "NOT_FOUND"
	case fiber.StatusConflict:
		return "CONFLICT"
	case fiber.StatusUnprocessableEntity:
		return "UNPROCESSABLE_ENTITY"
	default:
		return "INTERNAL_ERROR"
	}
}

// apiError returns a structured error envelope response.
func apiError(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(fiber.Map{
		"error": fiber.Map{
			"code":    code,
			"message": message,
		},
	})
}

// shouldMaskKey reports whether the config key's value should be masked in GET responses.
func shouldMaskKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "key") ||
		strings.Contains(lower, "password") ||
		strings.Contains(lower, "secret")
}

// RegisterRoutes sets up all API routes on the Fiber app.
func RegisterRoutes(app *fiber.App, appCtx *models.App, secretKey string, devMode bool) {
	// CORS
	if devMode {
		app.Use(cors.New(cors.Config{AllowOrigins: "*"}))
	} else {
		app.Use(cors.New(cors.Config{AllowOrigins: "http://stasharr-ui"}))
	}

	app.Use(LoggingMiddleware())

	v1 := app.Group("/api/v1")

	// Health — unauthenticated
	v1.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// Status — authenticated
	v1.Get("/status", AuthMiddleware(secretKey), handleStatus(appCtx))

	// SSE — auth via query param
	v1.Get("/events", AuthFromQuery(secretKey), handleGlobalEvents(appCtx))
	v1.Get("/jobs/:id/events", AuthFromQuery(secretKey), handleJobEvents(appCtx))

	// All remaining routes require header auth
	v1.Use(AuthMiddleware(secretKey))

	// Jobs
	v1.Post("/jobs", handleCreateJob(appCtx))
	v1.Get("/jobs/stats", handleJobStats(appCtx))
	v1.Get("/jobs", handleListJobs(appCtx))
	v1.Get("/jobs/:id/neighbors", handleGetJobNeighbors(appCtx))
	v1.Get("/jobs/:id", handleGetJob(appCtx))
	v1.Post("/jobs/:id/approve", handleApproveJob(appCtx))
	v1.Post("/jobs/:id/retry", handleRetryJob(appCtx))
	v1.Post("/jobs/:id/advance", handleAdvanceJob(appCtx))
	v1.Post("/jobs/:id/search", handleCustomSearch(appCtx))
	v1.Patch("/jobs/:id/status", handleSetJobStatus(appCtx))
	v1.Delete("/jobs/:id", handleDeleteJob(appCtx))

	// Review queue (same handler, forced status filter)
	v1.Get("/review", handleListJobs(appCtx))

	// Batches
	v1.Get("/batches", handleListBatches(appCtx))
	v1.Get("/batches/:id", handleGetBatch(appCtx))
	v1.Post("/batches/:id/approve", handleApproveBatch(appCtx))
	v1.Post("/batches/:id/deny", handleDenyBatch(appCtx))
	v1.Post("/batches/:id/next", handleNextBatch(appCtx))
	v1.Post("/batches/:id/auto-start", handleAutoStartBatch(appCtx))

	// Config
	v1.Get("/config", handleGetConfig(appCtx))
	v1.Put("/config", handleUpdateConfig(appCtx))
	v1.Post("/config/test/:service", handleTestService(appCtx))

	// Stash instances
	v1.Get("/stash-instances", handleListStashInstances(appCtx))
	v1.Post("/stash-instances", handleCreateStashInstance(appCtx))
	v1.Put("/stash-instances/:id", handleUpdateStashInstance(appCtx))
	v1.Delete("/stash-instances/:id", handleDeleteStashInstance(appCtx))
	v1.Post("/stash-instances/:id/test", handleTestStashInstance(appCtx))

	// Studio aliases
	v1.Get("/aliases", handleListAliases(appCtx))
	v1.Post("/aliases", handleCreateAlias(appCtx))
	v1.Delete("/aliases/:id", handleDeleteAlias(appCtx))
}

func handleStatus(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()

		var sup worker.SupervisorStatus
		if s, ok := app.Supervisor.(*worker.Supervisor); ok {
			sup = s.Status()
		}

		workers := fiber.Map{}
		for _, w := range sup.Workers {
			workers[w.Name] = fiber.Map{
				"running":   w.Running,
				"pool_size": w.PoolSize,
			}
		}

		// DB ping
		dbOK := app.DB.Ping(ctx) == nil

		// Service pings (best-effort)
		_, prowlarrErr := app.Prowlarr.Ping(ctx)
		_, sabnzbdErr := app.SABnzbd.Ping(ctx)
		_, stashErr := app.StashApp.Ping(ctx)

		return c.JSON(fiber.Map{
			"workers":  workers,
			"database": fiber.Map{"ok": dbOK},
			"prowlarr": fiber.Map{"ok": prowlarrErr == nil},
			"sabnzbd":  fiber.Map{"ok": sabnzbdErr == nil},
			"stash":    fiber.Map{"ok": stashErr == nil},
		})
	}
}
