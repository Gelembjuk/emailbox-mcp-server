package main

import (
	"context"
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
		"Email MCP Server",
		"1.0.0",
		server.WithLogging(),
	)

	// Register tools
	shared.RegisterTools(mcpServer, config)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start notification checker
	shared.StartEmailNotificationChecker(ctx, mcpServer, config)

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
		os.Exit(0)
	}()

	// Start STDIO server
	stdioServer := server.NewStdioServer(mcpServer)
	log.Println("Starting Email MCP Server (STDIO mode)")

	if err := stdioServer.Listen(ctx, os.Stdin, os.Stdout); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
