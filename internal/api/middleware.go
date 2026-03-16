package api

import "github.com/gofiber/fiber/v2"

// AuthMiddleware validates the X-Api-Key header.
func AuthMiddleware(secretKey string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// TODO: implement
		return c.Next()
	}
}

// RegisterRoutes sets up all API routes.
func RegisterRoutes(app *fiber.App, appCtx interface{}) {
	v1 := app.Group("/api/v1")

	// Health (unauthenticated)
	v1.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// All other routes require auth
	// TODO: add AuthMiddleware

	// Jobs
	v1.Get("/jobs", handleListJobs(appCtx))
	v1.Post("/jobs", handleCreateJob(appCtx))
	v1.Get("/jobs/:id", handleGetJob(appCtx))
	v1.Post("/jobs/:id/approve", handleApproveJob(appCtx))
	v1.Post("/jobs/:id/retry", handleRetryJob(appCtx))
	v1.Delete("/jobs/:id", handleDeleteJob(appCtx))

	// Job SSE
	v1.Get("/jobs/:id/events", handleJobEvents(appCtx))

	// Batches
	v1.Get("/batches", handleListBatches(appCtx))
	v1.Get("/batches/:id", handleGetBatch(appCtx))
	v1.Post("/batches/:id/confirm", handleConfirmBatch(appCtx))

	// Review
	v1.Get("/review", handleListJobs(appCtx)) // same handler, filtered

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

	// Aliases
	v1.Get("/aliases", handleListAliases(appCtx))
	v1.Post("/aliases", handleCreateAlias(appCtx))
	v1.Delete("/aliases/:id", handleDeleteAlias(appCtx))

	// Status
	v1.Get("/status", func(c *fiber.Ctx) error {
		// TODO: implement
		return c.SendStatus(fiber.StatusNotImplemented)
	})

	// Global SSE
	v1.Get("/events", handleGlobalEvents(appCtx))
}
