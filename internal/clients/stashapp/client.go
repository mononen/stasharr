package stashapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"
)

// Client is a GraphQL client for the StashApp API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NetworkError wraps a transport-level failure.
type NetworkError struct{ Err error }

func (e *NetworkError) Error() string { return "stashapp: network error: " + e.Err.Error() }
func (e *NetworkError) Unwrap() error { return e.Err }

// StatusError is returned when the server responds with a non-2xx status.
type StatusError struct {
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("stashapp: unexpected status %d: %s", e.StatusCode, e.Body)
}

// ParseError is returned when the response body cannot be decoded.
type ParseError struct{ Err error }

func (e *ParseError) Error() string { return "stashapp: parse error: " + e.Err.Error() }
func (e *ParseError) Unwrap() error { return e.Err }

// New creates a new StashApp client.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// graphqlRequest sends a GraphQL request and returns the raw response bytes.
func (c *Client) graphqlRequest(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
	payload := map[string]any{
		"query":     query,
		"variables": variables,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, &ParseError{err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/graphql", bytes.NewReader(body))
	if err != nil {
		return nil, &NetworkError{err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("ApiKey", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &NetworkError{err}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &NetworkError{err}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &StatusError{resp.StatusCode, string(respBody)}
	}
	return respBody, nil
}

// StashScene holds minimal scene metadata returned from the Stash instance.
type StashScene struct {
	Title      string
	StudioName string
}

// FindSceneByStashDBID looks up a scene by its StashDB ID in the Stash instance.
// Returns nil, nil when the scene is not found.
func (c *Client) FindSceneByStashDBID(ctx context.Context, stashdbSceneID string) (*StashScene, error) {
	const query = `query FindSceneByStashDBID($stash_id: String!, $endpoint: String!) {
		findScenes(scene_filter: { stash_id_endpoint: { stash_id: $stash_id, endpoint: $endpoint, modifier: EQUALS } }) {
			scenes {
				title
				studio { name }
			}
		}
	}`

	respBytes, err := c.graphqlRequest(ctx, query, map[string]any{
		"stash_id": stashdbSceneID,
		"endpoint": "https://stashdb.org/graphql",
	})
	if err != nil {
		return nil, err
	}

	var envelope struct {
		Data struct {
			FindScenes struct {
				Scenes []struct {
					Title  string `json:"title"`
					Studio *struct {
						Name string `json:"name"`
					} `json:"studio"`
				} `json:"scenes"`
			} `json:"findScenes"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBytes, &envelope); err != nil {
		return nil, &ParseError{err}
	}
	if len(envelope.Errors) > 0 {
		return nil, &ParseError{fmt.Errorf("%s", envelope.Errors[0].Message)}
	}
	scenes := envelope.Data.FindScenes.Scenes
	if len(scenes) == 0 {
		return nil, nil
	}
	s := scenes[0]
	result := &StashScene{Title: s.Title}
	if s.Studio != nil {
		result.StudioName = s.Studio.Name
	}
	return result, nil
}

// FindSceneByPath returns the Stash scene ID if a scene with this exact path exists, or "" if not found.
func (c *Client) FindSceneByPath(ctx context.Context, path string) (string, error) {
	const query = `query FindSceneByPath($path: String!) {
		findScenes(scene_filter: { path: { value: $path, modifier: EQUALS } }) {
			scenes { id }
		}
	}`

	respBytes, err := c.graphqlRequest(ctx, query, map[string]any{"path": path})
	if err != nil {
		return "", err
	}

	var envelope struct {
		Data struct {
			FindScenes struct {
				Scenes []struct {
					ID string `json:"id"`
				} `json:"scenes"`
			} `json:"findScenes"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBytes, &envelope); err != nil {
		return "", &ParseError{err}
	}
	if len(envelope.Errors) > 0 {
		return "", &ParseError{fmt.Errorf("%s", envelope.Errors[0].Message)}
	}
	if len(envelope.Data.FindScenes.Scenes) == 0 {
		return "", nil
	}
	return envelope.Data.FindScenes.Scenes[0].ID, nil
}

// UpdateSceneStashID attaches a StashDB stash_id to a scene in Stash.
func (c *Client) UpdateSceneStashID(ctx context.Context, sceneID, stashdbSceneID string) error {
	const mutation = `mutation SceneUpdate($input: SceneUpdateInput!) {
		sceneUpdate(input: $input) { id }
	}`

	_, err := c.graphqlRequest(ctx, mutation, map[string]any{
		"input": map[string]any{
			"id": sceneID,
			"stash_ids": []map[string]any{
				{
					"endpoint": "https://stashdb.org/graphql",
					"stash_id": stashdbSceneID,
				},
			},
		},
	})
	return err
}

// TriggerScan executes a minimal metadataScan on the parent directory of path.
// Generation tasks are disabled so Stash only registers the file — stasharr
// handles scraping and phash generation explicitly afterwards.
func (c *Client) TriggerScan(ctx context.Context, path string) error {
	const mutation = `mutation MetadataScan($input: ScanMetadataInput!) {
		metadataScan(input: $input)
	}`

	dir := filepath.Dir(path)
	_, err := c.graphqlRequest(ctx, mutation, map[string]any{
		"input": map[string]any{
			"paths":                     []string{dir},
			"scanGenerateCovers":        false,
			"scanGeneratePreviews":      false,
			"scanGenerateImagePreviews": false,
			"scanGenerateThumbnails":    false,
			"scanGeneratePhashes":       false,
			"scanGenerateSprites":       false,
			"scanGenerateClipPreviews":  false,
		},
	})
	return err
}

// ScrapedScene holds metadata returned from Stash's scrapeSingleScene mutation.
type ScrapedScene struct {
	Title      *string            `json:"title"`
	Date       *string            `json:"date"`
	Details    *string            `json:"details"`
	URL        *string            `json:"url"`
	Studio     *ScrapedStudio     `json:"studio"`
	Performers []ScrapedPerformer `json:"performers"`
	Tags       []ScrapedTag       `json:"tags"`
	StashIDs   []ScrapedStashID   `json:"stash_ids"`
}

// ScrapedStudio holds studio data from a scene scrape.
type ScrapedStudio struct {
	StoredID *string `json:"stored_id"`
	Name     string  `json:"name"`
}

// ScrapedPerformer holds performer data from a scene scrape.
type ScrapedPerformer struct {
	StoredID *string `json:"stored_id"`
	Name     string  `json:"name"`
}

// ScrapedTag holds tag data from a scene scrape.
type ScrapedTag struct {
	StoredID *string `json:"stored_id"`
	Name     string  `json:"name"`
}

// ScrapedStashID holds a stash_id entry from a scene scrape.
type ScrapedStashID struct {
	Endpoint string `json:"endpoint"`
	StashID  string `json:"stash_id"`
}

// ScrapeSingleScene fetches scene metadata from StashDB via Stash's built-in stash-box scraper.
// Returns nil if the scene was not found on StashDB.
func (c *Client) ScrapeSingleScene(ctx context.Context, stashdbSceneID string) (*ScrapedScene, error) {
	const mutation = `mutation ScrapeSingleScene($source: ScraperSourceInput!, $input: ScrapeSceneInput!) {
		scrapeSingleScene(source: $source, input: $input) {
			title date details url
			studio { stored_id name }
			performers { stored_id name }
			tags { stored_id name }
			stash_ids { endpoint stash_id }
		}
	}`

	respBytes, err := c.graphqlRequest(ctx, mutation, map[string]any{
		"source": map[string]any{
			"stash_box_endpoint": "https://stashdb.org/graphql",
		},
		"input": map[string]any{
			"stash_box_scene_ids": []string{stashdbSceneID},
		},
	})
	if err != nil {
		return nil, err
	}

	var envelope struct {
		Data struct {
			ScrapeSingleScene *ScrapedScene `json:"scrapeSingleScene"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBytes, &envelope); err != nil {
		return nil, &ParseError{err}
	}
	if len(envelope.Errors) > 0 {
		return nil, &ParseError{fmt.Errorf("%s", envelope.Errors[0].Message)}
	}
	return envelope.Data.ScrapeSingleScene, nil
}

// ApplySceneScrape applies scraped metadata to a scene via sceneUpdate.
// It maps stored performer/tag/studio IDs from the scraped result into the update input.
func (c *Client) ApplySceneScrape(ctx context.Context, sceneID string, scraped *ScrapedScene) error {
	const mutation = `mutation SceneUpdate($input: SceneUpdateInput!) {
		sceneUpdate(input: $input) { id }
	}`

	input := map[string]any{"id": sceneID}

	if scraped.Title != nil {
		input["title"] = *scraped.Title
	}
	if scraped.Date != nil {
		input["date"] = *scraped.Date
	}
	if scraped.Details != nil {
		input["details"] = *scraped.Details
	}
	if scraped.URL != nil {
		input["url"] = *scraped.URL
	}
	if scraped.Studio != nil && scraped.Studio.StoredID != nil {
		input["studio_id"] = *scraped.Studio.StoredID
	}

	var performerIDs []string
	for _, p := range scraped.Performers {
		if p.StoredID != nil {
			performerIDs = append(performerIDs, *p.StoredID)
		}
	}
	if len(performerIDs) > 0 {
		input["performer_ids"] = performerIDs
	}

	var tagIDs []string
	for _, t := range scraped.Tags {
		if t.StoredID != nil {
			tagIDs = append(tagIDs, *t.StoredID)
		}
	}
	if len(tagIDs) > 0 {
		input["tag_ids"] = tagIDs
	}

	if len(scraped.StashIDs) > 0 {
		var stashIDs []map[string]any
		for _, sid := range scraped.StashIDs {
			stashIDs = append(stashIDs, map[string]any{
				"endpoint": sid.Endpoint,
				"stash_id": sid.StashID,
			})
		}
		input["stash_ids"] = stashIDs
	}

	_, err := c.graphqlRequest(ctx, mutation, map[string]any{"input": input})
	return err
}

// GeneratePhash triggers phash generation for a single specific scene.
func (c *Client) GeneratePhash(ctx context.Context, sceneID string) error {
	const mutation = `mutation MetadataGenerate($input: GenerateMetadataInput!) {
		metadataGenerate(input: $input)
	}`

	_, err := c.graphqlRequest(ctx, mutation, map[string]any{
		"input": map[string]any{
			"scene_ids": []string{sceneID},
			"phashes":   true,
		},
	})
	return err
}

// Ping checks connectivity and returns the Stash version string.
func (c *Client) Ping(ctx context.Context) (string, error) {
	const query = `query { version { version } }`

	respBytes, err := c.graphqlRequest(ctx, query, nil)
	if err != nil {
		return "", err
	}

	var envelope struct {
		Data struct {
			Version struct {
				Version string `json:"version"`
			} `json:"version"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBytes, &envelope); err != nil {
		return "", &ParseError{err}
	}
	if len(envelope.Errors) > 0 {
		return "", &ParseError{fmt.Errorf("%s", envelope.Errors[0].Message)}
	}
	return envelope.Data.Version.Version, nil
}
