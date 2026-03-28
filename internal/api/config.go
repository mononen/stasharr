package api

import (
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/mononen/stasharr/internal/clients/myjdownloader"
	"github.com/mononen/stasharr/internal/clients/prowlarr"
	"github.com/mononen/stasharr/internal/clients/sabnzbd"
	"github.com/mononen/stasharr/internal/clients/stashapp"
	"github.com/mononen/stasharr/internal/clients/stashdb"
	"github.com/mononen/stasharr/internal/db/queries"
	"github.com/mononen/stasharr/internal/models"
)

// --- Config ---

func handleGetConfig(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		q := queries.New(app.DB)

		cfgs, err := q.GetAllConfig(ctx)
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to load config")
		}

		grouped := fiber.Map{}
		for _, cfg := range cfgs {
			parts := strings.SplitN(cfg.Key, ".", 2)
			prefix := parts[0]
			suffix := parts[0]
			if len(parts) == 2 {
				suffix = parts[1]
			}

			if _, ok := grouped[prefix]; !ok {
				grouped[prefix] = fiber.Map{}
			}
			group := grouped[prefix].(fiber.Map)

			value := cfg.Value
			if value != "" && shouldMaskKey(cfg.Key) {
				value = "***"
			}
			group[suffix] = value
		}

		return c.JSON(grouped)
	}
}

func handleUpdateConfig(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		q := queries.New(app.DB)

		var body map[string]string
		if err := c.BodyParser(&body); err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		}

		keys := make([]string, 0, len(body))
		values := make([]string, 0, len(body))
		for k, v := range body {
			// If the value is '***' and it's a sensitive key, use the existing value.
			if v == "***" && shouldMaskKey(k) {
				v = app.Config.Get(k)
			}
			keys = append(keys, k)
			values = append(values, v)
			app.Config.Set(k, v)
		}

		if len(keys) > 0 {
			if err := q.SetConfigValues(ctx, queries.SetConfigValuesParams{
				Keys:   keys,
				Values: values,
			}); err != nil {
				return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to update config")
			}
		}

		// Update in-memory clients with new config values.
		app.RefreshClients()

		// Return current config (re-read from DB for accuracy).
		cfgs, err := q.GetAllConfig(ctx)
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to reload config")
		}

		grouped := fiber.Map{}
		for _, cfg := range cfgs {
			parts := strings.SplitN(cfg.Key, ".", 2)
			prefix := parts[0]
			suffix := parts[0]
			if len(parts) == 2 {
				suffix = parts[1]
			}
			if _, ok := grouped[prefix]; !ok {
				grouped[prefix] = fiber.Map{}
			}
			group := grouped[prefix].(fiber.Map)
			value := cfg.Value
			if value != "" && shouldMaskKey(cfg.Key) {
				value = "***"
			}
			group[suffix] = value
		}

		return c.JSON(grouped)
	}
}

func handleTestService(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		service := c.Params("service")

		var body struct {
			URL        string `json:"url"`
			APIKey     string `json:"api_key"`
			Email      string `json:"email"`
			Password   string `json:"password"`
			DeviceName string `json:"device_name"`
		}
		_ = c.BodyParser(&body)

		switch service {
		case "prowlarr":
			client := app.Prowlarr
			if body.URL != "" {
				key := body.APIKey
				if key == "***" {
					key = app.Config.Get("prowlarr.api_key")
				}
				client = prowlarr.New(body.URL, key)
			}
			msg, err := client.Ping(ctx)
			if err != nil {
				return c.JSON(fiber.Map{"service": service, "ok": false, "message": err.Error()})
			}
			return c.JSON(fiber.Map{"service": service, "ok": true, "message": msg})

		case "sabnzbd":
			client := app.SABnzbd
			if body.URL != "" {
				key := body.APIKey
				if key == "***" {
					key = app.Config.Get("sabnzbd.api_key")
				}
				client = sabnzbd.New(body.URL, key, app.Config.Get("sabnzbd.category"))
			}
			msg, err := client.Ping(ctx)
			if err != nil {
				return c.JSON(fiber.Map{"service": service, "ok": false, "message": err.Error()})
			}
			return c.JSON(fiber.Map{"service": service, "ok": true, "message": msg})

		case "prowlarr-apikey":
		client := app.Prowlarr
		if body.URL != "" {
			key := body.APIKey
			if key == "***" {
				key = app.Config.Get("prowlarr.api_key")
			}
			client = prowlarr.New(body.URL, key)
		}
		msg, err := client.CheckAPIKey(ctx)
		if err != nil {
			return c.JSON(fiber.Map{"service": service, "ok": false, "message": err.Error()})
		}
		return c.JSON(fiber.Map{"service": service, "ok": true, "message": msg})

	case "sabnzbd-apikey":
		client := app.SABnzbd
		if body.URL != "" {
			key := body.APIKey
			if key == "***" {
				key = app.Config.Get("sabnzbd.api_key")
			}
			client = sabnzbd.New(body.URL, key, app.Config.Get("sabnzbd.category"))
		}
		msg, err := client.CheckAPIKey(ctx)
		if err != nil {
			return c.JSON(fiber.Map{"service": service, "ok": false, "message": err.Error()})
		}
		return c.JSON(fiber.Map{"service": service, "ok": true, "message": msg})

	case "stashdb":
			client := app.StashDB
			if body.APIKey != "" {
				key := body.APIKey
				if key == "***" {
					key = app.Config.Get("stashdb.api_key")
				}
				client = stashdb.New(key, nil)
			}
			err := client.Ping(ctx)
			if err != nil {
				return c.JSON(fiber.Map{"service": service, "ok": false, "message": err.Error()})
			}
			return c.JSON(fiber.Map{"service": service, "ok": true, "message": "StashDB API key valid"})

		case "myjdownloader":
			email := body.Email
			if email == "" {
				email = app.Config.Get("myjdownloader.email")
			}
			password := body.Password
			if password == "" || password == "***" {
				password = app.Config.Get("myjdownloader.password")
			}
			deviceName := body.DeviceName
			if deviceName == "" {
				deviceName = app.Config.Get("myjdownloader.device_name")
			}
			log.Printf("[myjdownloader] test: email=%q password_len=%d device=%q", email, len(password), deviceName)
			client := myjdownloader.New(email, password, deviceName)
			if err := client.Ping(c.Context()); err != nil {
				return c.JSON(fiber.Map{"service": service, "ok": false, "message": err.Error()})
			}
			return c.JSON(fiber.Map{"service": service, "ok": true, "message": "Connected to device: " + deviceName})

		default:
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST",
				"service must be one of: prowlarr, prowlarr-apikey, sabnzbd, sabnzbd-apikey, stashdb, myjdownloader")
		}
	}
}

// --- Stash Instances ---

func handleListStashInstances(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		q := queries.New(app.DB)

		instances, err := q.ListStashInstances(ctx)
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to list stash instances")
		}
		if instances == nil {
			instances = []queries.StashInstance{}
		}

		type instanceResp struct {
			ID        uuid.UUID   `json:"id"`
			Name      string      `json:"name"`
			URL       string      `json:"url"`
			APIKey    string      `json:"api_key"`
			IsDefault bool        `json:"is_default"`
			CreatedAt interface{} `json:"created_at"`
			UpdatedAt interface{} `json:"updated_at"`
		}

		rows := make([]instanceResp, 0, len(instances))
		for _, inst := range instances {
			maskedKey := ""
			if inst.ApiKey != "" {
				maskedKey = "***"
			}
			rows = append(rows, instanceResp{
				ID:        inst.ID,
				Name:      inst.Name,
				URL:       inst.Url,
				APIKey:    maskedKey,
				IsDefault: inst.IsDefault,
				CreatedAt: inst.CreatedAt,
				UpdatedAt: inst.UpdatedAt,
			})
		}

		return c.JSON(fiber.Map{"instances": rows})
	}
}

func handleCreateStashInstance(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		q := queries.New(app.DB)

		var body struct {
			Name      string `json:"name"`
			URL       string `json:"url"`
			APIKey    string `json:"api_key"`
			IsDefault bool   `json:"is_default"`
		}
		if err := c.BodyParser(&body); err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		}
		if body.Name == "" || body.URL == "" {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "name and url are required")
		}

		// Enforce single-default rule: clear existing default if this one is default.
		if body.IsDefault {
			existing, listErr := q.ListStashInstances(ctx)
			if listErr == nil {
				for _, inst := range existing {
					if inst.IsDefault {
						_ = q.SetDefaultStashInstance(ctx, uuid.Nil) // clear all
						break
					}
				}
			}
		}

		inst, err := q.CreateStashInstance(ctx, queries.CreateStashInstanceParams{
			Name:      body.Name,
			Url:       body.URL,
			ApiKey:    body.APIKey,
			IsDefault: body.IsDefault,
		})
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to create stash instance")
		}

		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"id":         inst.ID,
			"name":       inst.Name,
			"url":        inst.Url,
			"api_key":    "***",
			"is_default": inst.IsDefault,
			"created_at": inst.CreatedAt,
			"updated_at": inst.UpdatedAt,
		})
	}
}

func handleUpdateStashInstance(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		q := queries.New(app.DB)

		id, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid instance id")
		}

		// Fetch current to preserve existing api_key if not provided.
		current, err := q.GetStashInstance(ctx, id)
		if err != nil {
			return apiError(c, fiber.StatusNotFound, "NOT_FOUND", "stash instance not found")
		}

		var body struct {
			Name      string `json:"name"`
			URL       string `json:"url"`
			APIKey    string `json:"api_key"`
			IsDefault bool   `json:"is_default"`
		}
		if err := c.BodyParser(&body); err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		}

		apiKey := body.APIKey
		if apiKey == "***" {
			apiKey = current.ApiKey
		}
		name := body.Name
		if name == "" {
			name = current.Name
		}
		url := body.URL
		if url == "" {
			url = current.Url
		}

		// If promoting to default, update the default flag across all rows.
		if body.IsDefault {
			_ = q.SetDefaultStashInstance(ctx, id)
		}

		inst, err := q.UpdateStashInstance(ctx, queries.UpdateStashInstanceParams{
			ID:        id,
			Name:      name,
			Url:       url,
			ApiKey:    apiKey,
			IsDefault: body.IsDefault,
		})
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to update stash instance")
		}

		return c.JSON(fiber.Map{
			"id":         inst.ID,
			"name":       inst.Name,
			"url":        inst.Url,
			"api_key":    "***",
			"is_default": inst.IsDefault,
			"created_at": inst.CreatedAt,
			"updated_at": inst.UpdatedAt,
		})
	}
}

func handleDeleteStashInstance(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		q := queries.New(app.DB)

		id, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid instance id")
		}

		instances, err := q.ListStashInstances(ctx)
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to list instances")
		}

		if len(instances) <= 1 {
			return apiError(c, fiber.StatusConflict, "LAST_INSTANCE", "cannot delete the only stash instance")
		}

		// Find the target instance.
		var target *queries.StashInstance
		for i := range instances {
			if instances[i].ID == id {
				target = &instances[i]
				break
			}
		}
		if target == nil {
			return apiError(c, fiber.StatusNotFound, "NOT_FOUND", "stash instance not found")
		}

		// Reject deletion of default when no other exists to promote.
		if target.IsDefault {
			// Promote another instance automatically.
			for _, inst := range instances {
				if inst.ID != id {
					_ = q.SetDefaultStashInstance(ctx, inst.ID)
					break
				}
			}
		}

		if err := q.DeleteStashInstance(ctx, id); err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete stash instance")
		}

		return c.SendStatus(fiber.StatusNoContent)
	}
}

func handleTestStashInstance(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		q := queries.New(app.DB)

		id, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid instance id")
		}

		inst, err := q.GetStashInstance(ctx, id)
		if err != nil {
			return apiError(c, fiber.StatusNotFound, "NOT_FOUND", "stash instance not found")
		}

		client := stashapp.New(inst.Url, inst.ApiKey)
		version, pingErr := client.Ping(ctx)
		if pingErr != nil {
			return c.JSON(fiber.Map{
				"ok":      false,
				"message": pingErr.Error(),
			})
		}

		return c.JSON(fiber.Map{
			"ok":      true,
			"message": "Connected. Stash version: " + version,
		})
	}
}

// --- Studio Aliases ---

func handleListAliases(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		q := queries.New(app.DB)

		aliases, err := q.ListAliases(ctx)
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to list aliases")
		}
		if aliases == nil {
			aliases = []queries.StudioAlias{}
		}
		return c.JSON(fiber.Map{"aliases": aliases})
	}
}

func handleCreateAlias(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		q := queries.New(app.DB)

		var body struct {
			Canonical string `json:"canonical"`
			Alias     string `json:"alias"`
		}
		if err := c.BodyParser(&body); err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		}
		if body.Canonical == "" || body.Alias == "" {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "canonical and alias are required")
		}

		// Reject duplicate alias values.
		if _, err := q.GetAliasByAlias(ctx, body.Alias); err == nil {
			return apiError(c, fiber.StatusConflict, "DUPLICATE_ALIAS",
				"an alias with this value already exists")
		}

		alias, err := q.CreateAlias(ctx, queries.CreateAliasParams{
			Canonical: body.Canonical,
			Alias:     body.Alias,
		})
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to create alias")
		}

		return c.Status(fiber.StatusCreated).JSON(alias)
	}
}

func handleDeleteAlias(app *models.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()
		q := queries.New(app.DB)

		id, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid alias id")
		}

		if err := q.DeleteAlias(ctx, id); err != nil {
			return apiError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete alias")
		}

		return c.SendStatus(fiber.StatusNoContent)
	}
}
