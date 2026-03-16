package sabnzbd

import "github.com/mononen/stasharr/internal/config"

// Client is an HTTP client for the SABnzbd API.
type Client struct {
	// TODO: add fields
}

// New creates a new SABnzbd client.
func New(cfg *config.Config) *Client {
	return &Client{}
}
