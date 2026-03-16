package models

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mononen/stasharr/internal/clients/prowlarr"
	"github.com/mononen/stasharr/internal/clients/sabnzbd"
	"github.com/mononen/stasharr/internal/clients/stashapp"
	"github.com/mononen/stasharr/internal/clients/stashdb"
	"github.com/mononen/stasharr/internal/config"
	"github.com/mononen/stasharr/internal/db/queries"
)

// App is the top-level dependency container passed through the application.
type App struct {
	DB         *pgxpool.Pool
	Config     *config.Config
	Prowlarr   *prowlarr.Client
	SABnzbd    *sabnzbd.Client
	StashApp   *stashapp.Client
	StashDB    *stashdb.Client
	Supervisor any // *worker.Supervisor
}

// Performer represents a scene performer entry stored in scenes.performers JSONB.
type Performer struct {
	Name           string  `json:"name"`
	Slug           string  `json:"slug"`
	Disambiguation *string `json:"disambiguation,omitempty"`
}

// Re-export sqlc-generated DB model types so callers import from internal/models.

type BatchJob = queries.BatchJob
type Config = queries.Config
type Download = queries.Download
type Job = queries.Job
type JobEvent = queries.JobEvent
type Scene = queries.Scene
type SchemaMigration = queries.SchemaMigration
type SearchResult = queries.SearchResult
type StashInstance = queries.StashInstance
type StudioAlias = queries.StudioAlias
