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

	client := NewClient(cfg.PaperlessURL, cfg.PaperlessToken)
	client.CheckAPIVersion()
	mcpServer := NewMCPServer(client)
	httpHandler := NewHTTPServer(mcpServer, cfg.MCPToken, cfg.PaperlessURL)

	log.Printf("paperless-mcp %s listening on %s", serverVersion, cfg.ListenAddr)
	log.Printf("paperless-ngx: %s", cfg.PaperlessURL)
	if cfg.MCPToken != "" {
		log.Printf("bearer auth: enabled")
	} else {
		log.Printf("bearer auth: disabled (no mcp_token configured)")
	}

	log.Fatal(http.ListenAndServe(cfg.ListenAddr, httpHandler))
}
