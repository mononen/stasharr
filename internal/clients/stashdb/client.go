package stashdb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

const endpoint = "https://stashdb.org/graphql"

// Client is a GraphQL client for the StashDB API.
type Client struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
	limiter    *rate.Limiter
}

// Performer is a scene performer as returned by StashDB.
type Performer struct {
	Name           string  `json:"name"`
	Slug           string  `json:"slug"`
	Disambiguation *string `json:"disambiguation,omitempty"`
	Gender         string  `json:"gender,omitempty"`
}

// Scene is a StashDB scene with all metadata fields populated.
// The worker layer maps this to the DB model type when persisting.
type Scene struct {
	ID              string
	Title           string
	Date            string // "YYYY-MM-DD"
	DurationSeconds int
	StudioName      string
	StudioSlug      string
	Performers      []Performer
	Tags            []string
	RawResponse     []byte
}

// NetworkError wraps a transport-level failure.
type NetworkError struct{ Err error }

func (e *NetworkError) Error() string { return "stashdb: network error: " + e.Err.Error() }
func (e *NetworkError) Unwrap() error { return e.Err }

// StatusError is returned when the server responds with a non-2xx status.
type StatusError struct {
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("stashdb: unexpected status %d: %s", e.StatusCode, e.Body)
}

// ParseError is returned when the response body cannot be decoded.
type ParseError struct{ Err error }

func (e *ParseError) Error() string { return "stashdb: parse error: " + e.Err.Error() }
func (e *ParseError) Unwrap() error { return e.Err }

// New creates a new StashDB client targeting https://stashdb.org/graphql.
// limiter controls the request rate; pass nil to allow unlimited requests.
func New(apiKey string, limiter *rate.Limiter) *Client {
	return NewWithEndpoint(apiKey, limiter, endpoint)
}

// NewWithEndpoint creates a StashDB client with a custom endpoint URL.
// Intended for testing; production code should use New.
func NewWithEndpoint(apiKey string, limiter *rate.Limiter, endpointURL string) *Client {
	return &Client{
		endpoint:   endpointURL,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		limiter:    limiter,
	}
}

// graphqlRequest sends a GraphQL query and returns the raw response bytes.
func (c *Client) graphqlRequest(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, &NetworkError{err}
		}
	}

	payload := map[string]any{
		"query":     query,
		"variables": variables,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, &ParseError{err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
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

// stashdbSceneFields is the common scene fragment used in all queries.
const stashdbSceneFields = `
	id
	title
	date
	duration
	studio { name }
	performers { performer { name disambiguation gender } }
	tags { name }
	urls { url type }
`

// rawScene is the JSON shape StashDB returns for a scene.
type rawScene struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Date     string `json:"date"`
	Duration int    `json:"duration"`
	Studio   *struct {
		Name string `json:"name"`
	} `json:"studio"`
	Performers []struct {
		Performer struct {
			Name           string  `json:"name"`
			Disambiguation *string `json:"disambiguation"`
			Gender         string  `json:"gender"`
		} `json:"performer"`
	} `json:"performers"`
	Tags []struct {
		Name string `json:"name"`
	} `json:"tags"`
}

// mapRawScene converts a rawScene and its original JSON bytes to a Scene.
func mapRawScene(raw rawScene, rawJSON []byte) Scene {
	s := Scene{
		ID:          raw.ID,
		Title:       raw.Title,
		Date:        raw.Date,
		DurationSeconds: raw.Duration,
		RawResponse: rawJSON,
	}
	if raw.Studio != nil {
		s.StudioName = raw.Studio.Name
	}
	for _, p := range raw.Performers {
		s.Performers = append(s.Performers, Performer{
			Name:           p.Performer.Name,
			Disambiguation: p.Performer.Disambiguation,
			Gender:         p.Performer.Gender,
		})
	}
	for _, t := range raw.Tags {
		s.Tags = append(s.Tags, t.Name)
	}
	return s
}

// FindScene fetches a single scene by StashDB ID.
func (c *Client) FindScene(ctx context.Context, id string) (*Scene, error) {
	const query = `query FindScene($id: ID!) {
		findScene(id: $id) {` + stashdbSceneFields + `}
	}`

	respBytes, err := c.graphqlRequest(ctx, query, map[string]any{"id": id})
	if err != nil {
		return nil, err
	}

	var envelope struct {
		Data struct {
			FindScene *rawScene `json:"findScene"`
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
	if envelope.Data.FindScene == nil {
		return nil, &StatusError{404, "scene not found"}
	}

	// Capture just the scene object bytes for RawResponse.
	sceneBytes, _ := json.Marshal(envelope.Data.FindScene)
	scene := mapRawScene(*envelope.Data.FindScene, sceneBytes)
	return &scene, nil
}

// FindPerformerScenes fetches all scenes for a performer, handling pagination.
func (c *Client) FindPerformerScenes(ctx context.Context, performerID string) ([]Scene, error) {
	const query = `query QueryPerformerScenes($input: SceneQueryInput!) {
		queryScenes(input: $input) {
			count
			scenes {` + stashdbSceneFields + `}
		}
	}`

	const perPage = 25
	var all []Scene

	for page := 1; ; page++ {
		respBytes, err := c.graphqlRequest(ctx, query, map[string]any{
			"input": map[string]any{
				"performers": map[string]any{
					"value":    []string{performerID},
					"modifier": "INCLUDES",
				},
				"page":     page,
				"per_page": perPage,
			},
		})
		if err != nil {
			return nil, err
		}

		scenes, total, err := parseQueryScenesResponse(respBytes)
		if err != nil {
			return nil, err
		}
		all = append(all, scenes...)
		if len(all) >= total || len(scenes) == 0 {
			break
		}
	}
	return all, nil
}

// FindStudioScenes fetches all scenes for a studio, handling pagination.
func (c *Client) FindStudioScenes(ctx context.Context, studioID string) ([]Scene, error) {
	const query = `query QueryStudioScenes($input: SceneQueryInput!) {
		queryScenes(input: $input) {
			count
			scenes {` + stashdbSceneFields + `}
		}
	}`

	const perPage = 25
	var all []Scene

	for page := 1; ; page++ {
		respBytes, err := c.graphqlRequest(ctx, query, map[string]any{
			"input": map[string]any{
				"studios": map[string]any{
					"value":    []string{studioID},
					"modifier": "INCLUDES",
				},
				"page":     page,
				"per_page": perPage,
			},
		})
		if err != nil {
			return nil, err
		}

		scenes, total, err := parseQueryScenesResponse(respBytes)
		if err != nil {
			return nil, err
		}
		all = append(all, scenes...)
		if len(all) >= total || len(scenes) == 0 {
			break
		}
	}
	return all, nil
}

// parseQueryScenesResponse decodes a queryScenes GraphQL response.
func parseQueryScenesResponse(respBytes []byte) ([]Scene, int, error) {
	var envelope struct {
		Data struct {
			QueryScenes struct {
				Count  int        `json:"count"`
				Scenes []rawScene `json:"scenes"`
			} `json:"queryScenes"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBytes, &envelope); err != nil {
		return nil, 0, &ParseError{err}
	}
	if len(envelope.Errors) > 0 {
		return nil, 0, &ParseError{fmt.Errorf("%s", envelope.Errors[0].Message)}
	}

	raw := envelope.Data.QueryScenes
	scenes := make([]Scene, 0, len(raw.Scenes))
	for _, r := range raw.Scenes {
		sceneBytes, _ := json.Marshal(r)
		scenes = append(scenes, mapRawScene(r, sceneBytes))
	}
	return scenes, raw.Count, nil
}

// FindPerformerName returns the performer's display name, or "" if not found.
func (c *Client) FindPerformerName(ctx context.Context, id string) (string, error) {
	const query = `query FindPerformer($id: ID!) {
		findPerformer(id: $id) { name }
	}`

	respBytes, err := c.graphqlRequest(ctx, query, map[string]any{"id": id})
	if err != nil {
		return "", err
	}

	var envelope struct {
		Data struct {
			FindPerformer *struct {
				Name string `json:"name"`
			} `json:"findPerformer"`
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
	if envelope.Data.FindPerformer == nil {
		return "", nil
	}
	return envelope.Data.FindPerformer.Name, nil
}

// FindStudioName returns the studio's display name, or "" if not found.
func (c *Client) FindStudioName(ctx context.Context, id string) (string, error) {
	const query = `query FindStudio($id: ID!) {
		findStudio(id: $id) { name }
	}`

	respBytes, err := c.graphqlRequest(ctx, query, map[string]any{"id": id})
	if err != nil {
		return "", err
	}

	var envelope struct {
		Data struct {
			FindStudio *struct {
				Name string `json:"name"`
			} `json:"findStudio"`
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
	if envelope.Data.FindStudio == nil {
		return "", nil
	}
	return envelope.Data.FindStudio.Name, nil
}

// BatchPerPage is the number of scenes fetched per page for batch operations.
const BatchPerPage = 20

// FindPerformerScenesPage fetches a single page of scenes for a performer.
// tagIDs optionally filters to scenes that include at least one of the given tag IDs.
// Returns (scenes, totalCount, error). Uses per_page=20 for clean batch alignment.
func (c *Client) FindPerformerScenesPage(ctx context.Context, performerID string, page int, tagIDs []string) ([]Scene, int, error) {
	const query = `query QueryPerformerScenes($input: SceneQueryInput!) {
		queryScenes(input: $input) {
			count
			scenes {` + stashdbSceneFields + `}
		}
	}`

	input := map[string]any{
		"performers": map[string]any{
			"value":    []string{performerID},
			"modifier": "INCLUDES",
		},
		"page":     page,
		"per_page": BatchPerPage,
	}
	if len(tagIDs) > 0 {
		input["tags"] = map[string]any{
			"value":    tagIDs,
			"modifier": "INCLUDES",
		}
	}

	respBytes, err := c.graphqlRequest(ctx, query, map[string]any{"input": input})
	if err != nil {
		return nil, 0, err
	}
	return parseQueryScenesResponse(respBytes)
}

// FindStudioScenesPage fetches a single page of scenes for a studio.
// tagIDs optionally filters to scenes that include at least one of the given tag IDs.
// Returns (scenes, totalCount, error). Uses per_page=20 for clean batch alignment.
func (c *Client) FindStudioScenesPage(ctx context.Context, studioID string, page int, tagIDs []string) ([]Scene, int, error) {
	const query = `query QueryStudioScenes($input: SceneQueryInput!) {
		queryScenes(input: $input) {
			count
			scenes {` + stashdbSceneFields + `}
		}
	}`

	input := map[string]any{
		"studios": map[string]any{
			"value":    []string{studioID},
			"modifier": "INCLUDES",
		},
		"page":     page,
		"per_page": BatchPerPage,
	}
	if len(tagIDs) > 0 {
		input["tags"] = map[string]any{
			"value":    tagIDs,
			"modifier": "INCLUDES",
		}
	}

	respBytes, err := c.graphqlRequest(ctx, query, map[string]any{"input": input})
	if err != nil {
		return nil, 0, err
	}
	return parseQueryScenesResponse(respBytes)
}

// Ping sends a minimal introspection query to verify the API key is valid.
func (c *Client) Ping(ctx context.Context) error {
	if c.apiKey == "" {
		return fmt.Errorf("stashdb: api key is missing")
	}
	respBytes, err := c.graphqlRequest(ctx, `{ __typename }`, nil)
	if err != nil {
		return err
	}
	var envelope struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBytes, &envelope); err != nil {
		return &ParseError{err}
	}
	if len(envelope.Errors) > 0 {
		return &ParseError{fmt.Errorf("%s", envelope.Errors[0].Message)}
	}
	return nil
}
