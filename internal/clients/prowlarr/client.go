package prowlarr

import "github.com/mononen/stasharr/internal/config"

// Client is an HTTP client for the Prowlarr API.
type Client struct {
	// TODO: add fields
}

// New creates a new Prowlarr client.
func New(cfg *config.Config) *Client {
	return &Client{}
}
