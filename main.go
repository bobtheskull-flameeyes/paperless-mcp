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
	httpHandler := NewHTTPServer(mcpServer, cfg.EffectiveMCPToken(), cfg.PaperlessURL)

	log.Printf("paperless-mcp %s listening on %s", serverVersion, cfg.ListenAddr)
	log.Printf("paperless-ngx: %s", cfg.PaperlessURL)
	switch cfg.MCPAuth {
	case MCPAuthNone:
		log.Printf("mcp auth: none")
	case MCPAuthToken:
		log.Printf("mcp auth: token")
	case MCPAuthPassthrough:
		log.Printf("mcp auth: passthrough (using paperless token)")
	}

	log.Fatal(http.ListenAndServe(cfg.ListenAddr, httpHandler))
}
