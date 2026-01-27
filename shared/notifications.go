package shared

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

// notificationBroadcaster interface for servers that support broadcasting to all clients
type notificationBroadcaster interface {
	SendNotificationToAllClients(method string, params map[string]any)
}

// EmailNotificationChecker checks for new emails and sends notifications
type EmailNotificationChecker struct {
	config      *Config
	imapClient  *IMAPClient
	lastUID     uint32
	mu          sync.Mutex
	broadcaster notificationBroadcaster

	// For starting/stopping the checker
	running    atomic.Bool
	cancelFunc context.CancelFunc
	cancelMu   sync.Mutex
	ctx        context.Context // parent context
}

// NewEmailNotificationChecker creates a new notification checker
func NewEmailNotificationChecker(config *Config, mcpServer *server.MCPServer) *EmailNotificationChecker {
	checker := &EmailNotificationChecker{
		config:     config,
		imapClient: NewIMAPClient(config),
	}

	// Check if server supports broadcasting to all clients
	if broadcaster, ok := interface{}(mcpServer).(notificationBroadcaster); ok {
		checker.broadcaster = broadcaster
	}

	return checker
}

// SetContext sets the parent context for the checker
func (c *EmailNotificationChecker) SetContext(ctx context.Context) {
	c.ctx = ctx
}

// Start begins the background email checking goroutine
func (c *EmailNotificationChecker) Start() {
	if c.broadcaster == nil {
		log.Println("Warning: Server does not support SendNotificationToAllClients, notifications disabled")
		return
	}

	if c.running.Load() {
		return // Already running
	}

	c.cancelMu.Lock()
	defer c.cancelMu.Unlock()

	if c.running.Load() {
		return // Double-check after acquiring lock
	}

	// Get initial latest UID
	lastUID, err := c.imapClient.GetLatestUID()
	if err != nil {
		log.Printf("Warning: Could not get initial email UID: %v", err)
		lastUID = 0
	}
	c.mu.Lock()
	c.lastUID = lastUID
	c.mu.Unlock()

	// Create cancellable context
	parentCtx := c.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(parentCtx)
	c.cancelFunc = cancel

	interval := time.Duration(c.config.Notifications.CheckIntervalSeconds) * time.Second
	c.running.Store(true)

	go func() {
		log.Printf("Email notification checker started (interval: %v)", interval)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer c.running.Store(false)

		for {
			select {
			case <-ctx.Done():
				log.Println("Email notification checker stopped")
				return
			case <-ticker.C:
				c.checkForNewEmails()
			}
		}
	}()
}

// Stop stops the background email checking goroutine
func (c *EmailNotificationChecker) Stop() {
	c.cancelMu.Lock()
	defer c.cancelMu.Unlock()

	if c.cancelFunc != nil {
		c.cancelFunc()
		c.cancelFunc = nil
	}
}

// IsRunning returns whether the checker is currently running
func (c *EmailNotificationChecker) IsRunning() bool {
	return c.running.Load()
}

// checkForNewEmails checks for new emails and sends notifications
func (c *EmailNotificationChecker) checkForNewEmails() {
	c.mu.Lock()
	currentLastUID := c.lastUID
	c.mu.Unlock()

	log.Printf("Checking for new emails (since UID: %d)...", currentLastUID)

	newEmails, err := c.imapClient.GetEmailsSinceUID(currentLastUID)
	if err != nil {
		log.Printf("Error checking for new emails: %v", err)
		return
	}

	log.Printf("Check complete: found %d new email(s)", len(newEmails))

	for _, email := range newEmails {
		// Update lastUID
		var emailUID uint32
		fmt.Sscanf(email.ID, "%d", &emailUID)

		c.mu.Lock()
		if emailUID > c.lastUID {
			c.lastUID = emailUID
		}
		c.mu.Unlock()

		// Get email preview
		detail, err := c.imapClient.GetEmailContents(email.ID)
		preview := ""
		if err == nil {
			preview = GetEmailPreview(detail.Body, 100)
		}

		log.Printf("New email received: ID=%s From=%s Subject=%s", email.ID, email.From, email.Subject)

		// Broadcast notification to all connected clients
		c.broadcaster.SendNotificationToAllClients("new_email", map[string]any{
			"title":       "New Email Received. From: " + email.From + ", Subject: " + email.Subject,
			"email_id":    email.ID,
			"from":        email.From,
			"subject":     email.Subject,
			"received_at": email.Date.Format(time.RFC3339),
			"preview":     preview,
		})
	}
}

// ClientTracker tracks connected clients and manages the notification checker
type ClientTracker struct {
	clientCount atomic.Int32
	checker     *EmailNotificationChecker
	mu          sync.Mutex
}

// NewClientTracker creates a new client tracker
func NewClientTracker(checker *EmailNotificationChecker) *ClientTracker {
	return &ClientTracker{
		checker: checker,
	}
}

// SetChecker sets the notification checker (allows deferred initialization)
func (t *ClientTracker) SetChecker(checker *EmailNotificationChecker) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.checker = checker
}

// OnClientConnected is called when a client connects
func (t *ClientTracker) OnClientConnected() {
	count := t.clientCount.Add(1)
	log.Printf("Client connected (total: %d)", count)

	t.mu.Lock()
	defer t.mu.Unlock()

	// Start checker when first client connects
	if count == 1 && t.checker != nil {
		t.checker.Start()
	}
}

// OnClientDisconnected is called when a client disconnects
func (t *ClientTracker) OnClientDisconnected() {
	count := t.clientCount.Add(-1)
	log.Printf("Client disconnected (total: %d)", count)

	t.mu.Lock()
	defer t.mu.Unlock()

	// Stop checker when last client disconnects
	if count == 0 && t.checker != nil {
		t.checker.Stop()
	}
}

// GetClientCount returns the current number of connected clients
func (t *ClientTracker) GetClientCount() int {
	return int(t.clientCount.Load())
}

// CreateSessionHooks creates hooks for tracking client sessions
func CreateSessionHooks(tracker *ClientTracker) *server.Hooks {
	hooks := &server.Hooks{}

	hooks.AddOnRegisterSession(func(ctx context.Context, session server.ClientSession) {
		tracker.OnClientConnected()
	})

	hooks.AddOnUnregisterSession(func(ctx context.Context, session server.ClientSession) {
		tracker.OnClientDisconnected()
	})

	return hooks
}

// StartEmailNotificationChecker creates and starts the notification checker
// For STDIO mode - starts immediately (single client always connected)
func StartEmailNotificationChecker(ctx context.Context, mcpServer *server.MCPServer, config *Config) *EmailNotificationChecker {
	checker := NewEmailNotificationChecker(config, mcpServer)
	checker.SetContext(ctx)
	checker.Start() // Start immediately for STDIO
	return checker
}

// SetupHTTPNotificationChecker sets up the notification checker for HTTP mode
// Returns the checker and tracker - checker is NOT started, it starts when first client connects
func SetupHTTPNotificationChecker(ctx context.Context, mcpServer *server.MCPServer, config *Config) (*EmailNotificationChecker, *ClientTracker) {
	checker := NewEmailNotificationChecker(config, mcpServer)
	checker.SetContext(ctx)
	tracker := NewClientTracker(checker)
	return checker, tracker
}
