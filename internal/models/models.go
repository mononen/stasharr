package models

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/mononen/stasharr/internal/clients/myjdownloader"
	"github.com/mononen/stasharr/internal/clients/prowlarr"
	"github.com/mononen/stasharr/internal/clients/sabnzbd"
	"github.com/mononen/stasharr/internal/clients/stashapp"
	"github.com/mononen/stasharr/internal/clients/stashdb"
	"github.com/mononen/stasharr/internal/config"
	"github.com/mononen/stasharr/internal/db/queries"
)

// App is the top-level dependency container passed through the application.
type App struct {
	DB              *pgxpool.Pool
	Config          *config.Config
	Prowlarr        *prowlarr.Client
	SABnzbd         *sabnzbd.Client
	StashApp        *stashapp.Client
	StashDB         *stashdb.Client
	MyJDownloader   *myjdownloader.Client
	Supervisor      any // *worker.Supervisor
	ProwlarrLogDir  string // if non-empty, search logs are written here (dev mode)
}

// RefreshClients re-initializes client instances from the current config.
func (a *App) RefreshClients() {
	log.Debug().Str("prowlarr_url", a.Config.Get("prowlarr.url")).Msg("refreshing prowlarr client")
	p := prowlarr.New(a.Config.Get("prowlarr.url"), a.Config.Get("prowlarr.api_key"))
	if a.ProwlarrLogDir != "" {
		p = p.WithLogDir(a.ProwlarrLogDir)
	}
	a.Prowlarr = p
	log.Debug().Str("sabnzbd_url", a.Config.Get("sabnzbd.url")).Msg("refreshing sabnzbd client")
	a.SABnzbd = sabnzbd.New(a.Config.Get("sabnzbd.url"), a.Config.Get("sabnzbd.api_key"), a.Config.Get("sabnzbd.category"))
	a.StashApp = stashapp.New(a.Config.Get("stashapp.url"), a.Config.Get("stashapp.api_key"))
	a.StashDB = stashdb.New(a.Config.Get("stashdb.api_key"), nil)
	a.MyJDownloader = myjdownloader.New(
		a.Config.Get("myjdownloader.email"),
		a.Config.Get("myjdownloader.password"),
		a.Config.Get("myjdownloader.device_name"),
	)
}

// Performer represents a scene performer entry stored in scenes.performers JSONB.
type Performer struct {
	Name           string  `json:"name"`
	Slug           string  `json:"slug"`
	Disambiguation *string `json:"disambiguation,omitempty"`
	Gender         string  `json:"gender,omitempty"`
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
