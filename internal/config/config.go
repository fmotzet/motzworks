// Package config loads and validates motzworks configuration from a YAML
// file with environment-variable overrides.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config is the top-level application configuration.
type Config struct {
	Log      LogConfig      `yaml:"log"`
	Database DatabaseConfig `yaml:"database"`
	Server   ServerConfig   `yaml:"server"`
	Vault    VaultConfig    `yaml:"vault"`
	Scan     ScanConfig     `yaml:"scan"`
}

// LogConfig controls structured logging output.
type LogConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // text or json
}

// DatabaseConfig describes the PostgreSQL connection.
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Name     string `yaml:"name"`
	SSLMode  string `yaml:"sslmode"`
}

// DSN builds a libpq-style connection string.
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode)
}

// ServerConfig configures the HTTP API/dashboard server.
type ServerConfig struct {
	Addr string `yaml:"addr"`
}

// VaultConfig configures the credential vault.
type VaultConfig struct {
	// KeyEnv names the environment variable holding the base64-encoded key.
	KeyEnv string `yaml:"key_env"`
}

// ScanConfig holds scan engine defaults.
type ScanConfig struct {
	Concurrency int `yaml:"concurrency"`
}

// Default returns a Config populated with sensible defaults.
func Default() Config {
	return Config{
		Log: LogConfig{Level: "info", Format: "text"},
		Database: DatabaseConfig{
			Host: "localhost", Port: 5432,
			User: "motzworks", Name: "motzworks", SSLMode: "disable",
		},
		Server: ServerConfig{Addr: ":8080"},
		Vault:  VaultConfig{KeyEnv: "MOTZWORKS_VAULT_KEY"},
		Scan:   ScanConfig{Concurrency: 64},
	}
}

// Load reads the YAML file at path (if non-empty), applies environment
// overrides, and validates the result. A non-empty path that cannot be read
// is an error.
func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("read config %s: %w", path, err)
		}
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse config %s: %w", path, err)
		}
	}
	cfg.applyEnv()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// applyEnv overlays MOTZWORKS_* environment variables onto the config.
// Secrets (notably the DB password) are best supplied this way rather than
// committed to a file.
func (c *Config) applyEnv() {
	if v := os.Getenv("MOTZWORKS_DB_HOST"); v != "" {
		c.Database.Host = v
	}
	if v := os.Getenv("MOTZWORKS_DB_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			c.Database.Port = p
		}
	}
	if v := os.Getenv("MOTZWORKS_DB_USER"); v != "" {
		c.Database.User = v
	}
	if v := os.Getenv("MOTZWORKS_DB_PASSWORD"); v != "" {
		c.Database.Password = v
	}
	if v := os.Getenv("MOTZWORKS_DB_NAME"); v != "" {
		c.Database.Name = v
	}
	if v := os.Getenv("MOTZWORKS_DB_SSLMODE"); v != "" {
		c.Database.SSLMode = v
	}
	if v := os.Getenv("MOTZWORKS_LOG_LEVEL"); v != "" {
		c.Log.Level = v
	}
}

// Validate checks required fields.
func (c Config) Validate() error {
	if c.Database.Host == "" {
		return errors.New("database.host is required")
	}
	if c.Database.Port == 0 {
		return errors.New("database.port is required")
	}
	if c.Scan.Concurrency < 1 {
		return errors.New("scan.concurrency must be >= 1")
	}
	return nil
}
