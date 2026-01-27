package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gelembjuk/mcp_imap_smtp/shared"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Load configuration
	config, err := shared.LoadConfig("")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Validate IMAP and SMTP connections before starting
	log.Println("Validating email server connections...")
	if err := shared.ValidateConnections(config); err != nil {
		log.Fatalf("Connection validation failed: %v", err)
	}
	log.Println("Email server connections validated successfully")

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"Email MCP Server (HTTP Streaming)",
		"1.0.0",
		server.WithLogging(),
	)

	// Register tools
	shared.RegisterTools(mcpServer, config)

	// Create HTTP streaming server
	httpServer := server.NewStreamableHTTPServer(mcpServer)

	// Setup graceful shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start email notification checker
	// Note: StreamableHTTPServer doesn't support session hooks for client tracking,
	// so we run the checker continuously (same as email_mock)
	shared.StartEmailNotificationChecker(ctx, mcpServer, config)

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	addr := fmt.Sprintf("%s:%d", config.HTTP.Host, config.HTTP.Port)
	go func() {
		log.Printf("Starting Email MCP Server (HTTP Streaming) on %s", addr)
		log.Printf("MCP endpoint: http://%s/mcp", addr)
		if err := httpServer.Start(addr); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt
	<-sigChan
	log.Println("Received interrupt signal, shutting down...")

	// Graceful shutdown
	cancel() // Stop notification checker
	if err := httpServer.Shutdown(context.Background()); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
