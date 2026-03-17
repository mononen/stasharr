package prowlarr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Client is an HTTP client for the Prowlarr API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	logDir     string // if non-empty, search requests/responses are written here
}

// Result represents a single search result from Prowlarr /api/v1/search.
// Fields mirror matcher.ProwlarrResult — the worker layer converts between the two.
type Result struct {
	Title       string
	SizeBytes   int64
	PublishDate *time.Time
	IndexerName string
	DownloadURL string
	NzbID       string
	InfoURL     string
}

// NetworkError wraps a transport-level failure.
type NetworkError struct{ Err error }

func (e *NetworkError) Error() string { return "prowlarr: network error: " + e.Err.Error() }
func (e *NetworkError) Unwrap() error { return e.Err }

// StatusError is returned when the server responds with a non-2xx status.
type StatusError struct {
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("prowlarr: unexpected status %d: %s", e.StatusCode, e.Body)
}

// ParseError is returned when the response body cannot be decoded.
type ParseError struct{ Err error }

func (e *ParseError) Error() string { return "prowlarr: parse error: " + e.Err.Error() }
func (e *ParseError) Unwrap() error { return e.Err }

// New creates a new Prowlarr client.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// WithLogDir returns a copy of the client that writes search request/response
// JSON logs to dir. The directory is created if it does not exist.
func (c *Client) WithLogDir(dir string) *Client {
	cp := *c
	cp.logDir = dir
	return &cp
}

// writeSearchLog writes a JSON file capturing the query and raw response body.
// Errors are silently ignored — logging must not affect normal operation.
func (c *Client) writeSearchLog(query string, rawResponse []byte, searchErr error) {
	if c.logDir == "" {
		return
	}
	if err := os.MkdirAll(c.logDir, 0o755); err != nil {
		return
	}

	ts := time.Now().UTC().Format("2006-01-02T15-04-05.000Z")
	// Sanitise query for use in filename.
	safe := strings.NewReplacer(" ", "_", "/", "-", "\\", "-", ":", "-").Replace(query)
	if len(safe) > 80 {
		safe = safe[:80]
	}
	filename := filepath.Join(c.logDir, fmt.Sprintf("prowlarr_%s_%s.json", ts, safe))

	type logEntry struct {
		Timestamp string          `json:"timestamp"`
		Query     string          `json:"query"`
		Error     string          `json:"error,omitempty"`
		Response  json.RawMessage `json:"response,omitempty"`
	}
	entry := logEntry{
		Timestamp: ts,
		Query:     query,
	}
	if searchErr != nil {
		entry.Error = searchErr.Error()
	} else if len(rawResponse) > 0 {
		entry.Response = rawResponse
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filename, data, 0o644)
}

// searchItem mirrors the relevant fields of a Prowlarr /api/v1/search response item.
type searchItem struct {
	Title       string    `json:"title"`
	Size        int64     `json:"size"`
	PublishDate time.Time `json:"publishDate"`
	Indexer     string    `json:"indexer"`
	DownloadURL string    `json:"downloadUrl"`
	GUID        string    `json:"guid"`
	InfoURL     string    `json:"infoUrl"`
}

// Search calls GET /api/v1/search and returns the mapped results.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	u, err := url.Parse(c.baseURL + "/api/v1/search")
	if err != nil {
		return nil, &NetworkError{err}
	}
	q := u.Query()
	q.Set("query", query)
	q.Set("type", "search")
	q.Set("limit", strconv.Itoa(limit))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &NetworkError{err}
	}
	req.Header.Set("X-Api-Key", c.apiKey)

	body, err := c.do(req)
	c.writeSearchLog(query, body, err)
	if err != nil {
		return nil, err
	}

	var items []searchItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, &ParseError{err}
	}

	results := make([]Result, 0, len(items))
	for _, item := range items {
		var pt *time.Time
		if !item.PublishDate.IsZero() {
			t := item.PublishDate
			pt = &t
		}
		results = append(results, Result{
			Title:       item.Title,
			SizeBytes:   item.Size,
			PublishDate: pt,
			IndexerName: item.Indexer,
			DownloadURL: item.DownloadURL,
			NzbID:       item.GUID,
			InfoURL:     item.InfoURL,
		})
	}
	return results, nil
}

// FetchNZB fetches the raw NZB file bytes from downloadURL.
func (c *Client) FetchNZB(ctx context.Context, downloadURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, &NetworkError{err}
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	return c.do(req)
}

// Ping checks connectivity and returns a status message including indexer count.
func (c *Client) Ping(ctx context.Context) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("prowlarr: base URL is missing")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/indexer", nil)
	if err != nil {
		return "", &NetworkError{err}
	}
	req.Header.Set("X-Api-Key", c.apiKey)

	body, err := c.do(req)
	if err != nil {
		return "", err
	}

	var indexers []json.RawMessage
	if err := json.Unmarshal(body, &indexers); err != nil {
		return "", &ParseError{err}
	}
	return fmt.Sprintf("ok: %d indexer(s) configured", len(indexers)), nil
}

// do executes the request, reads the full body, and returns typed errors.
func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &NetworkError{err}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &NetworkError{err}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &StatusError{resp.StatusCode, string(body)}
	}
	return body, nil
}
