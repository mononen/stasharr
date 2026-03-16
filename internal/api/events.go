package api

import "github.com/gofiber/fiber/v2"

func handleGlobalEvents(app interface{}) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// TODO: implement SSE stream
		return c.SendStatus(fiber.StatusNotImplemented)
	}
}

func handleJobEvents(app interface{}) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// TODO: implement SSE stream
		return c.SendStatus(fiber.StatusNotImplemented)
	}
}
