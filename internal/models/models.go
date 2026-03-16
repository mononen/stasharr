package models

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mononen/stasharr/internal/clients/prowlarr"
	"github.com/mononen/stasharr/internal/clients/sabnzbd"
	"github.com/mononen/stasharr/internal/clients/stashapp"
	"github.com/mononen/stasharr/internal/clients/stashdb"
	"github.com/mononen/stasharr/internal/config"
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

// Job represents a pipeline job.
type Job struct {
	ID            string     `json:"id"`
	Type          string     `json:"type"`
	Status        string     `json:"status"`
	StashDBURL    string     `json:"stashdb_url"`
	StashDBID     *string    `json:"stashdb_id,omitempty"`
	ParentBatchID *string    `json:"parent_batch_id,omitempty"`
	ErrorMessage  *string    `json:"error_message,omitempty"`
	RetryCount    int        `json:"retry_count"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Scene holds resolved StashDB scene metadata.
type Scene struct {
	ID              string      `json:"id"`
	JobID           string      `json:"job_id"`
	StashDBSceneID  string      `json:"stashdb_scene_id"`
	Title           string      `json:"title"`
	StudioName      *string     `json:"studio_name,omitempty"`
	StudioSlug      *string     `json:"studio_slug,omitempty"`
	ReleaseDate     *time.Time  `json:"release_date,omitempty"`
	DurationSeconds *int        `json:"duration_seconds,omitempty"`
	Performers      []Performer `json:"performers"`
	Tags            []string    `json:"tags"`
	CreatedAt       time.Time   `json:"created_at"`
}

// Performer represents a scene performer.
type Performer struct {
	Name           string  `json:"name"`
	Slug           string  `json:"slug"`
	Disambiguation *string `json:"disambiguation,omitempty"`
}

// SearchResult represents an NZB candidate from Prowlarr.
type SearchResult struct {
	ID              string         `json:"id"`
	JobID           string         `json:"job_id"`
	IndexerName     string         `json:"indexer_name"`
	ReleaseTitle    string         `json:"release_title"`
	SizeBytes       *int64         `json:"size_bytes,omitempty"`
	PublishDate     *time.Time     `json:"publish_date,omitempty"`
	DownloadURL     *string        `json:"download_url,omitempty"`
	NzbID           *string        `json:"nzb_id,omitempty"`
	ConfidenceScore int            `json:"confidence_score"`
	ScoreBreakdown  map[string]any `json:"score_breakdown"`
	IsSelected      bool           `json:"is_selected"`
	SelectedBy      *string        `json:"selected_by,omitempty"`
	SelectedAt      *time.Time     `json:"selected_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
}

// Download tracks a SABnzbd download.
type Download struct {
	ID           string     `json:"id"`
	JobID        string     `json:"job_id"`
	SABnzbdNzoID string     `json:"sabnzbd_nzo_id"`
	Status       string     `json:"status"`
	Filename     *string    `json:"filename,omitempty"`
	SourcePath   *string    `json:"source_path,omitempty"`
	FinalPath    *string    `json:"final_path,omitempty"`
	SizeBytes    *int64     `json:"size_bytes,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

// BatchJob tracks performer/studio batch submissions.
type BatchJob struct {
	ID              string     `json:"id"`
	JobID           string     `json:"job_id"`
	Type            string     `json:"type"`
	StashDBEntityID string     `json:"stashdb_entity_id"`
	EntityName      *string    `json:"entity_name,omitempty"`
	TotalSceneCount *int       `json:"total_scene_count,omitempty"`
	EnqueuedCount   int        `json:"enqueued_count"`
	PendingCount    int        `json:"pending_count"`
	DuplicateCount  int        `json:"duplicate_count"`
	Confirmed       bool       `json:"confirmed"`
	ConfirmedAt     *time.Time `json:"confirmed_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// JobEvent is an append-only event log entry.
type JobEvent struct {
	ID        int64          `json:"id"`
	JobID     string         `json:"job_id"`
	EventType string         `json:"event_type"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}

// StashInstance is a user-configured StashApp connection.
type StashInstance struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	APIKey    string    `json:"api_key"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// StudioAlias maps variant studio names to canonical names.
type StudioAlias struct {
	ID        string    `json:"id"`
	Canonical string    `json:"canonical"`
	Alias     string    `json:"alias"`
	CreatedAt time.Time `json:"created_at"`
}
