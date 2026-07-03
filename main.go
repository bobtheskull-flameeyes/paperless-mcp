package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	configPath := flag.String("config", "config.json", "path to config file")
	addr := flag.String("addr", "", "listen address (overrides config)")
	flag.Parse()

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if *addr != "" {
		cfg.ListenAddr = *addr
	}

	// Create the base Paperless client. In passthrough mode without a
	// configured token, the base client has an empty token and is only
	// used as a template for per-request WithToken copies.
	baseClient := NewClient(cfg.PaperlessURL, cfg.PaperlessToken)

	// Check API version if we have a token to authenticate with.
	if cfg.PaperlessToken != "" {
		baseClient.CheckAPIVersion()
	}

	// Create the shared MCPServer for none/token modes. In passthrough mode
	// a per-request server is created instead, but we still need a base
	// client for WithToken.
	var baseMCP *MCPServer
	if cfg.MCPAuth != MCPAuthPassthrough {
		baseMCP = NewMCPServer(baseClient)
	}

	httpHandler := NewHTTPServer(baseMCP, baseClient, cfg)

	log.Printf("paperless-mcp %s listening on %s", serverVersion, cfg.ListenAddr)
	log.Printf("paperless-ngx: %s", cfg.PaperlessURL)
	switch cfg.MCPAuth {
	case MCPAuthNone:
		log.Printf("mcp auth: none")
	case MCPAuthToken:
		log.Printf("mcp auth: token")
	case MCPAuthPassthrough:
		if cfg.PaperlessToken != "" {
			log.Printf("mcp auth: passthrough (server-side token available for health checks)")
		} else {
			log.Printf("mcp auth: passthrough (no server-side token)")
		}
	}

	log.Fatal(http.ListenAndServe(cfg.ListenAddr, httpHandler))
}
