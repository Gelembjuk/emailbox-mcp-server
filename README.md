# MCP IMAP/SMTP Email Server

An MCP (Model Context Protocol) server that provides real email communication capabilities using IMAP for reading and SMTP for sending emails.

## Features

- **Send emails** via SMTP with support for text, markdown, and HTML formats
- **Read inbox** via IMAP with filtering options
- **Get full email contents** including attachments
- **New email notifications** via background polling
- **Dual transport**: STDIO mode for local MCP clients, HTTP streaming for network deployments
- **Contact list** support via configuration

## Quick Start

1. Copy `config.json.example` to `config.json` and fill in your IMAP/SMTP credentials.
2. Build and run:

```bash
# STDIO server
cd stdio-server && go build -o mcp-email-stdio && ./mcp-email-stdio

# HTTP server
cd http-server && go build -o mcp-email-http && ./mcp-email-http
```

The `config.json` file must be in the same directory as the binary.

## MCP Tools

| Tool | Description |
|------|-------------|
| `send_email` | Send an email (to, subject, body, optional cc/bcc) |
| `get_inbox` | List inbox emails (optional limit, unread filter) |
| `get_email_contents` | Get full content of a specific email by ID |
| `get_attachment` | Download an email attachment as base64 by email ID and attachment index |

## Requirements

- Go 1.23+
- An email account with IMAP/SMTP access

## License

This project is licensed under the CC BY-NC 4.0 License. See [LICENSE](LICENSE) for details.
