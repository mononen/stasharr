package stashapp

import "github.com/mononen/stasharr/internal/config"

// Client is a GraphQL client for the StashApp API.
type Client struct {
	// TODO: add fields
}

// New creates a new StashApp client.
func New(cfg *config.Config) *Client {
	return &Client{}
}
