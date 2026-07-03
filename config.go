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
	// MCPAuthPassthrough accepts the Paperless API token as the MCP bearer
	// token, so you can manage one token through the Paperless UI.
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

// EffectiveMCPToken returns the bearer token the MCP endpoint should check,
// or "" if authentication is disabled.
func (c *Config) EffectiveMCPToken() string {
	switch c.MCPAuth {
	case MCPAuthToken:
		return c.MCPToken
	case MCPAuthPassthrough:
		return c.PaperlessToken
	default:
		return ""
	}
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
	if cfg.PaperlessToken == "" {
		return nil, fmt.Errorf("paperless_token is required (set in config or PAPERLESS_TOKEN)")
	}

	// Normalise URL: strip trailing slashes.
	cfg.PaperlessURL = strings.TrimRight(cfg.PaperlessURL, "/")

	// Default mcp_auth from legacy mcp_token presence for backward compatibility.
	if cfg.MCPAuth == "" {
		if cfg.MCPToken != "" {
			cfg.MCPAuth = MCPAuthToken
		} else {
			cfg.MCPAuth = MCPAuthNone
		}
	}

	// Validate mcp_auth.
	switch cfg.MCPAuth {
	case MCPAuthNone:
		// OK.
	case MCPAuthToken:
		if cfg.MCPToken == "" {
			return nil, fmt.Errorf("mcp_auth is %q but mcp_token is not set", MCPAuthToken)
		}
	case MCPAuthPassthrough:
		// OK — uses PaperlessToken, which is already validated above.
	default:
		return nil, fmt.Errorf("mcp_auth must be %q, %q, or %q (got %q)",
			MCPAuthNone, MCPAuthToken, MCPAuthPassthrough, cfg.MCPAuth)
	}

	return cfg, nil
}
