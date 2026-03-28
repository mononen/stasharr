package config

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config holds application configuration loaded from the database.
type Config struct {
	mu     sync.RWMutex
	values map[string]string
}

// LoadFromDB reads all config key-value pairs from the database.
func LoadFromDB(ctx context.Context, pool *pgxpool.Pool) (*Config, error) {
	rows, err := pool.Query(ctx, "SELECT key, value FROM config")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &Config{values: values}, nil
}

// Get returns a config value by key.
func (c *Config) Get(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.values[key]
}

// Set updates a config value.
func (c *Config) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = value
}
