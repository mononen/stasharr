package config

import "os"

// EnvConfig holds values loaded from environment variables.
type EnvConfig struct {
	DatabaseDSN string
	SecretKey   string
	Port        string
	LogLevel    string
	DevMode     bool
}

// LoadEnv reads configuration from environment variables.
func LoadEnv() (*EnvConfig, error) {
	// TODO: implement
	return &EnvConfig{
		DatabaseDSN: os.Getenv("STASHARR_DB_DSN"),
		SecretKey:   os.Getenv("STASHARR_SECRET_KEY"),
		Port:        os.Getenv("STASHARR_LISTEN_PORT"),
		LogLevel:    os.Getenv("STASHARR_LOG_LEVEL"),
		DevMode:     os.Getenv("STASHARR_DEV") == "true",
	}, nil
}
