package stashdb

import "github.com/mononen/stasharr/internal/config"

// Client is a GraphQL client for the StashDB API.
type Client struct {
	// TODO: add fields
}

// New creates a new StashDB client.
func New(cfg *config.Config) *Client {
	return &Client{}
}
