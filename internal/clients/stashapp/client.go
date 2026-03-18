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

	// Check for GraphQL-level errors (HTTP 200 but errors in body).
	var errCheck struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &errCheck); err == nil && len(errCheck.Errors) > 0 {
		return nil, &ParseError{fmt.Errorf("%s", errCheck.Errors[0].Message)}
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

// TriggerScan executes a metadataScan on the parent directory of path.
func (c *Client) TriggerScan(ctx context.Context, path string) error {
	const mutation = `mutation MetadataScan($input: ScanMetadataInput!) {
		metadataScan(input: $input)
	}`

	dir := filepath.Dir(path)
	_, err := c.graphqlRequest(ctx, mutation, map[string]any{
		"input": map[string]any{
			"paths": []string{dir},
		},
	})
	return err
}

// RunIdentify triggers Stash's metadataIdentify task scoped to a single file path,
// using StashDB as the source. Unlike a manual scrape+apply, identify creates any
// missing performers, studios, and tags in the local Stash instance automatically.
func (c *Client) RunIdentify(ctx context.Context, path string) error {
	const mutation = `mutation MetadataIdentify($input: IdentifyMetadataInput!) {
		metadataIdentify(input: $input)
	}`

	_, err := c.graphqlRequest(ctx, mutation, map[string]any{
		"input": map[string]any{
			"sources": []map[string]any{
				{
					"source": map[string]any{
						"stash_box_endpoint": "https://stashdb.org/graphql",
					},
				},
			},
			"paths": []string{path},
		},
	})
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
