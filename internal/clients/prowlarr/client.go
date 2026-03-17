package prowlarr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client is an HTTP client for the Prowlarr API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
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

// searchItem mirrors the relevant fields of a Prowlarr /api/v1/search response item.
type searchItem struct {
	Title       string    `json:"title"`
	Size        int64     `json:"size"`
	PublishDate time.Time `json:"publishDate"`
	Indexer     string    `json:"indexer"`
	DownloadURL string    `json:"downloadUrl"`
	GUID        string    `json:"guid"`
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
