package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// NewHTTPServer creates the HTTP handler for the MCP server.
// In passthrough mode, baseMCP may be nil (no paperless_token configured);
// a per-request MCPServer is created from the caller's bearer token.
func NewHTTPServer(baseMCP *MCPServer, baseClient *Client, cfg *Config) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		// CORS preflight.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST, OPTIONS")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		// Resolve the MCPServer for this request based on auth mode.
		mcp, ok := resolveMCP(w, r, baseMCP, baseClient, cfg)
		if !ok {
			return // resolveMCP already wrote the error response.
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		log.Printf("http: POST /mcp from %s (%d bytes)", r.RemoteAddr, len(body))

		respData, statusCode := mcp.HandleMessage(body)

		if respData == nil {
			w.WriteHeader(statusCode)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(respData)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		resp := map[string]string{
			"status":        "ok",
			"paperless_url": cfg.PaperlessURL,
			"mcp_auth":      cfg.MCPAuth,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Route to mux or 404.
		if r.URL.Path == "/mcp" || r.URL.Path == "/health" {
			mux.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
}

// resolveMCP authenticates the request and returns the MCPServer to use.
// Returns false (and writes an HTTP error) if authentication fails.
func resolveMCP(w http.ResponseWriter, r *http.Request, baseMCP *MCPServer, baseClient *Client, cfg *Config) (*MCPServer, bool) {
	switch cfg.MCPAuth {
	case MCPAuthNone:
		return baseMCP, true

	case MCPAuthToken:
		bearer := extractBearer(r)
		if bearer != cfg.MCPToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return nil, false
		}
		return baseMCP, true

	case MCPAuthPassthrough:
		bearer := extractBearer(r)
		if bearer == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return nil, false
		}
		// Create a per-request client and MCPServer with the caller's token.
		reqClient := baseClient.WithToken(bearer)
		return NewMCPServer(reqClient), true

	default:
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return nil, false
	}
}

// extractBearer returns the bearer token from the Authorization header, or "".
func extractBearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}

// FormatJSON pretty-prints JSON for tool responses.
func FormatJSON(data json.RawMessage) string {
	var buf []byte
	buf, err := json.MarshalIndent(json.RawMessage(data), "", "  ")
	if err != nil {
		return string(data)
	}
	return string(buf)
}

// ErrorResult creates an MCP tool error result.
func ErrorResult(msg string, args ...interface{}) *ToolCallResult {
	return &ToolCallResult{
		Content: []ContentBlock{{
			Type: "text",
			Text: fmt.Sprintf(msg, args...),
		}},
		IsError: true,
	}
}

// TextResult creates an MCP tool success result with a text content block.
func TextResult(text string) *ToolCallResult {
	return &ToolCallResult{
		Content: []ContentBlock{{
			Type: "text",
			Text: text,
		}},
	}
}

// JSONResult creates a success result from raw JSON, pretty-printed.
func JSONResult(data json.RawMessage) *ToolCallResult {
	return TextResult(FormatJSON(data))
}
