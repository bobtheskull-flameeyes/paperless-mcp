package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Config holds all configuration for the MCP server.
type Config struct {
	PaperlessURL   string `json:"paperless_url"`
	PaperlessToken string `json:"paperless_token"`
	ListenAddr     string `json:"listen_addr"`
	MCPToken       string `json:"mcp_token"`
}

// LoadConfig reads configuration from a JSON file, then applies environment
// variable overrides for any field that has a corresponding env var set.
func LoadConfig(path string) (*Config, error) {
	cfg := &Config{
		ListenAddr: ":8035",
	}

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}
	if err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config %s: %w", path, err)
		}
	}

	// Environment variable overrides.
	if v := os.Getenv("PAPERLESS_URL"); v != "" {
		cfg.PaperlessURL = v
	}
	if v := os.Getenv("PAPERLESS_TOKEN"); v != "" {
		cfg.PaperlessToken = v
	}
	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("MCP_TOKEN"); v != "" {
		cfg.MCPToken = v
	}

	// Validate required fields.
	if cfg.PaperlessURL == "" {
		return nil, fmt.Errorf("paperless_url is required (set in config or PAPERLESS_URL)")
	}
	if cfg.PaperlessToken == "" {
		return nil, fmt.Errorf("paperless_token is required (set in config or PAPERLESS_TOKEN)")
	}

	// Normalise URL: strip trailing slashes.
	cfg.PaperlessURL = strings.TrimRight(cfg.PaperlessURL, "/")

	// Special mcp_token value: "paperless" means reuse the Paperless API token
	// for MCP endpoint authentication, so you can manage it from the Paperless UI.
	if strings.EqualFold(cfg.MCPToken, "paperless") {
		cfg.MCPToken = cfg.PaperlessToken
	}

	return cfg, nil
}
