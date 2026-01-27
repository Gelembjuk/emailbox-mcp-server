# MCP IMAP/SMTP Email Server

## Project Overview

This project implements an MCP (Model Context Protocol) server for real email communication using IMAP (for reading) and SMTP (for sending). Unlike the mock examples, this server connects to actual email servers.

## Architecture

### Dual Transport Support

The server supports two MCP transport modes (similar to the Go mock example):

1. **STDIO Server** - For local MCP client integration via stdin/stdout
2. **HTTP Streaming Server** - For distributed/network deployments with multiple clients

Both servers share common code through a `shared/` package.

### Directory Structure

```
mcp_imap_smtp/
├── shared/              # Shared library code
│   ├── config.go        # Configuration loading/management
│   ├── imap.go          # IMAP client operations
│   ├── smtp.go          # SMTP client operations
│   ├── handlers.go      # MCP tool handlers
│   ├── notifications.go # Email notification system
│   └── go.mod
├── stdio-server/        # STDIO MCP implementation
│   ├── main.go
│   └── go.mod
├── http-server/         # HTTP Streaming MCP implementation
│   ├── main.go
│   └── go.mod
├── config.json          # Configuration file (local to binary)
└── CLAUDE.md            # This file
```

## Configuration

**IMPORTANT**: Configuration is stored in a local JSON file (`config.json`) in the same folder as the binary, NOT via environment variables.

### Configuration File Structure

```json
{
  "imap": {
    "server": "imap.example.com",
    "port": 993,
    "username": "user@example.com",
    "password": "password",
    "use_tls": true
  },
  "smtp": {
    "server": "smtp.example.com",
    "port": 587,
    "username": "user@example.com",
    "password": "password",
    "require_tls": true
  },
  "my_email": "user@example.com",
  "contacts": {
    "John Doe": "john@example.com",
    "Jane Smith": "jane@example.com"
  },
  "http": {
    "host": "localhost",
    "port": 8081
  },
  "notifications": {
    "check_interval_seconds": 30
  }
}
```

### Configuration Fields

| Field | Description |
|-------|-------------|
| `imap.*` | IMAP server connection settings |
| `smtp.*` | SMTP server connection settings |
| `my_email` | User's email address (sender address) |
| `contacts` | Map of contact names to email addresses |
| `http.host` | HTTP server bind host (HTTP mode only) |
| `http.port` | HTTP server bind port (HTTP mode only) |
| `notifications.check_interval_seconds` | Interval for checking new emails |

## MCP Tools

### 1. `send_email`

Send an email via SMTP.

**Parameters:**
- `to` (string, required): Recipient email address
- `subject` (string, required): Email subject
- `body` (string, required): Email body content
- `body_format` (string, optional): "text", "markdown", or "html" (default: "text")
- `cc` (string, optional): CC recipient
- `bcc` (string, optional): BCC recipient

**Returns:** Success message or error

### 2. `get_inbox`

Retrieve emails from the inbox via IMAP.

**Parameters:**
- `limit` (integer, optional): Maximum number of emails to return (default: 20)
- `unread_only` (boolean, optional): Only return unread emails

**Returns:** JSON object with email list (id, from, subject, date, read status)

### 3. `get_email_contents`

Get the full content of a specific email.

**Parameters:**
- `email_id` (string, required): ID of the email to retrieve

**Returns:** Full email content including body, attachments info, headers

## Notification System

### Design

- **Single background thread** that periodically checks for new emails via IMAP
- NOT a separate thread per connection
- Broadcasts notifications to ALL connected clients
- For STDIO mode: single client receives notifications
- For HTTP mode: all connected clients receive notifications

### Notification Flow

1. Background goroutine starts when server initializes
2. Every `check_interval_seconds`, it queries IMAP for new emails
3. Tracks last seen email ID/UID to detect new arrivals
4. When new email detected, broadcasts `new_email` notification to all clients
5. Graceful shutdown via context cancellation

### Notification Payload

```json
{
  "method": "new_email",
  "params": {
    "email_id": "12345",
    "from": "sender@example.com",
    "subject": "Email subject",
    "received_at": "2024-01-15T10:30:00Z",
    "preview": "First 100 characters of body..."
  }
}
```

## Dependencies

- `github.com/mark3labs/mcp-go` - MCP protocol implementation
- `github.com/emersion/go-imap` - IMAP client library
- `github.com/emersion/go-smtp` or `net/smtp` - SMTP client
- Go 1.23+

## Build & Run

### STDIO Server

```bash
cd stdio-server
go build -o mcp-email-stdio
./mcp-email-stdio
```

### HTTP Server

```bash
cd http-server
go build -o mcp-email-http
./mcp-email-http
```

The config.json file must be in the same directory as the binary (or current working directory).

## Example Configurations

### Gmail

```json
{
  "imap": {
    "server": "imap.gmail.com",
    "port": 993,
    "username": "your-email@gmail.com",
    "password": "your-app-password",
    "use_tls": true
  },
  "smtp": {
    "server": "smtp.gmail.com",
    "port": 587,
    "username": "your-email@gmail.com",
    "password": "your-app-password",
    "require_tls": true
  }
}
```

### Outlook/Office 365

```json
{
  "imap": {
    "server": "outlook.office365.com",
    "port": 993,
    "username": "your-email@outlook.com",
    "password": "your-password",
    "use_tls": true
  },
  "smtp": {
    "server": "smtp.office365.com",
    "port": 587,
    "username": "your-email@outlook.com",
    "password": "your-password",
    "require_tls": true
  }
}
```

## Security Notes

- Store `config.json` with restricted file permissions (chmod 600)
- Use app-specific passwords when available (Gmail, etc.)
- Never commit `config.json` to version control
- Consider using encrypted config storage for production

## Reference Examples

This project is based on patterns from:
- `email_mock/` - Go MCP server examples (STDIO + HTTP) with notification broadcasting
- `email-mcp-server/` - Python MCP server with real SMTP and contact support
