// SPDX-FileCopyrightText: 2026 paperless-mcp contributors
// SPDX-License-Identifier: 0BSD

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// MCPAuth mode determines how the MCP endpoint authenticates callers.
const (
	// MCPAuthNone disables authentication on the MCP endpoint.
	MCPAuthNone = "none"
	// MCPAuthToken requires a dedicated bearer token (mcp_token).
	MCPAuthToken = "token"
	// MCPAuthPassthrough forwards the caller's bearer token to Paperless-ngx,
	// so callers authenticate with their own Paperless API token.
	MCPAuthPassthrough = "passthrough"
)

// Config holds all configuration for the MCP server.
type Config struct {
	PaperlessURL   string `json:"paperless_url"`
	PaperlessToken string `json:"paperless_token"`
	ListenAddr     string `json:"listen_addr"`
	MCPAuth        string `json:"mcp_auth"`
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
	if v := os.Getenv("MCP_AUTH"); v != "" {
		cfg.MCPAuth = v
	}
	if v := os.Getenv("MCP_TOKEN"); v != "" {
		cfg.MCPToken = v
	}

	// Validate required fields.
	if cfg.PaperlessURL == "" {
		return nil, fmt.Errorf("paperless_url is required (set in config or PAPERLESS_URL)")
	}

	// Default mcp_auth from legacy mcp_token presence for backward compatibility.
	if cfg.MCPAuth == "" {
		if cfg.MCPToken != "" {
			cfg.MCPAuth = MCPAuthToken
		} else {
			cfg.MCPAuth = MCPAuthNone
		}
	}

	// Validate mcp_auth and dependent fields.
	switch cfg.MCPAuth {
	case MCPAuthNone:
		if cfg.PaperlessToken == "" {
			return nil, fmt.Errorf("paperless_token is required (set in config or PAPERLESS_TOKEN)")
		}
	case MCPAuthToken:
		if cfg.PaperlessToken == "" {
			return nil, fmt.Errorf("paperless_token is required (set in config or PAPERLESS_TOKEN)")
		}
		if cfg.MCPToken == "" {
			return nil, fmt.Errorf("mcp_auth is %q but mcp_token is not set", MCPAuthToken)
		}
	case MCPAuthPassthrough:
		// paperless_token is optional: if set, used for startup checks;
		// if not, callers provide their own token per request.
	default:
		return nil, fmt.Errorf("mcp_auth must be %q, %q, or %q (got %q)",
			MCPAuthNone, MCPAuthToken, MCPAuthPassthrough, cfg.MCPAuth)
	}

	// Normalise URL: strip trailing slashes.
	cfg.PaperlessURL = strings.TrimRight(cfg.PaperlessURL, "/")

	return cfg, nil
}
