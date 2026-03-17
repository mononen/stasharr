package sabnzbd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"
)

// Client is an HTTP client for the SABnzbd API.
type Client struct {
	baseURL    string
	apiKey     string
	category   string
	httpClient *http.Client
}

// QueueItem represents an active download job in the SABnzbd queue.
type QueueItem struct {
	NzoID      string `json:"nzo_id"`
	Filename   string `json:"filename"`
	Status     string `json:"status"`
	Percentage string `json:"percentage"`
	MB         string `json:"mb"`
	MBLeft     string `json:"mbleft"`
}

// HistoryItem represents a completed or failed download in SABnzbd history.
type HistoryItem struct {
	NzoID       string `json:"nzo_id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Size        string `json:"size"`
	StoragePath string `json:"storage"`
}

// NetworkError wraps a transport-level failure.
type NetworkError struct{ Err error }

func (e *NetworkError) Error() string { return "sabnzbd: network error: " + e.Err.Error() }
func (e *NetworkError) Unwrap() error { return e.Err }

// StatusError is returned when the server responds with a non-2xx status.
type StatusError struct {
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("sabnzbd: unexpected status %d: %s", e.StatusCode, e.Body)
}

// ParseError is returned when the response body cannot be decoded.
type ParseError struct{ Err error }

func (e *ParseError) Error() string { return "sabnzbd: parse error: " + e.Err.Error() }
func (e *ParseError) Unwrap() error { return e.Err }

// APIError is returned when SABnzbd reports an application-level error.
type APIError struct{ Message string }

func (e *APIError) Error() string { return "sabnzbd: api error: " + e.Message }

// New creates a new SABnzbd client.
func New(baseURL, apiKey, category string) *Client {
	return &Client{
		baseURL:  baseURL,
		apiKey:   apiKey,
		category: category,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SubmitNZB POSTs nzbBytes as multipart/form-data to SABnzbd and returns the nzo_id.
func (c *Client) SubmitNZB(ctx context.Context, nzbBytes []byte, name string) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	part, err := mw.CreateFormFile("nzbfile", name+".nzb")
	if err != nil {
		return "", &ParseError{err}
	}
	if _, err := part.Write(nzbBytes); err != nil {
		return "", &ParseError{err}
	}
	if err := mw.Close(); err != nil {
		return "", &ParseError{err}
	}

	u := c.buildURL("addfile", url.Values{
		"cat":     {c.category},
		"nzbname": {name},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, &buf)
	if err != nil {
		return "", &NetworkError{err}
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	body, err := c.do(req)
	if err != nil {
		return "", err
	}

	var resp struct {
		Status bool     `json:"status"`
		NzoIDs []string `json:"nzo_ids"`
		Error  string   `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", &ParseError{err}
	}
	if !resp.Status {
		return "", &APIError{resp.Error}
	}
	if len(resp.NzoIDs) == 0 {
		return "", &APIError{"no nzo_id returned"}
	}
	return resp.NzoIDs[0], nil
}

// SubmitNZBURL adds a download by URL to SABnzbd (mode=addurl) and returns the nzo_id.
// Use this when the download URL points directly to an indexer rather than through Prowlarr.
func (c *Client) SubmitNZBURL(ctx context.Context, downloadURL string, name string) (string, error) {
	u := c.buildURL("addurl", url.Values{
		"name":    {downloadURL},
		"cat":     {c.category},
		"nzbname": {name},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", &NetworkError{err}
	}

	body, err := c.do(req)
	if err != nil {
		return "", err
	}

	var resp struct {
		Status bool     `json:"status"`
		NzoIDs []string `json:"nzo_ids"`
		Error  string   `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", &ParseError{err}
	}
	if !resp.Status {
		return "", &APIError{resp.Error}
	}
	if len(resp.NzoIDs) == 0 {
		return "", &APIError{"no nzo_id returned"}
	}
	return resp.NzoIDs[0], nil
}

// GetQueue returns active jobs from the SABnzbd queue.
func (c *Client) GetQueue(ctx context.Context) ([]QueueItem, error) {
	u := c.buildURL("queue", url.Values{"output": {"json"}})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, &NetworkError{err}
	}

	body, err := c.do(req)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Queue struct {
			Slots []QueueItem `json:"slots"`
		} `json:"queue"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, &ParseError{err}
	}
	return resp.Queue.Slots, nil
}

// GetHistory returns completed and failed jobs from SABnzbd history.
func (c *Client) GetHistory(ctx context.Context) ([]HistoryItem, error) {
	u := c.buildURL("history", url.Values{"output": {"json"}})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, &NetworkError{err}
	}

	body, err := c.do(req)
	if err != nil {
		return nil, err
	}

	var resp struct {
		History struct {
			Slots []HistoryItem `json:"slots"`
		} `json:"history"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, &ParseError{err}
	}
	return resp.History.Slots, nil
}

// DeleteJob removes a job from the SABnzbd queue by nzo_id.
func (c *Client) DeleteJob(ctx context.Context, nzoID string) error {
	u := c.buildURL("queue", url.Values{
		"name":  {"delete"},
		"value": {nzoID},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return &NetworkError{err}
	}

	_, err = c.do(req)
	return err
}

// Ping checks connectivity and returns a status message.
func (c *Client) Ping(ctx context.Context) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("sabnzbd: base URL is missing")
	}
	u := c.buildURL("version", url.Values{"output": {"json"}})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", &NetworkError{err}
	}

	body, err := c.do(req)
	if err != nil {
		return "", err
	}

	var resp struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", &ParseError{err}
	}
	return "ok: SABnzbd " + resp.Version, nil
}

// CheckAPIKey verifies that the configured API key is accepted by SABnzbd.
// Unlike Ping, this calls mode=queue which requires authentication.
func (c *Client) CheckAPIKey(ctx context.Context) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("sabnzbd: base URL is missing")
	}
	u := c.buildURL("queue", url.Values{"output": {"json"}, "limit": {"0"}})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", &NetworkError{err}
	}

	body, err := c.do(req)
	if err != nil {
		return "", err
	}

	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", &ParseError{err}
	}
	if resp.Error != "" {
		return "", &APIError{resp.Error}
	}
	return "ok: API key valid", nil
}

// buildURL constructs the SABnzbd API URL for a given mode and extra params.
func (c *Client) buildURL(mode string, extra url.Values) string {
	u, _ := url.Parse(c.baseURL + "/api")
	q := u.Query()
	q.Set("mode", mode)
	q.Set("apikey", c.apiKey)
	for k, vals := range extra {
		for _, v := range vals {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
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
