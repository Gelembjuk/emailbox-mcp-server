package shared

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// SMTPClient handles SMTP operations
type SMTPClient struct {
	config *Config
}

// NewSMTPClient creates a new SMTP client
func NewSMTPClient(config *Config) *SMTPClient {
	return &SMTPClient{config: config}
}

// ValidateConnection tests the SMTP connection and credentials
func (c *SMTPClient) ValidateConnection() error {
	addr := fmt.Sprintf("%s:%d", c.config.SMTP.Server, c.config.SMTP.Port)

	conn, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	if c.config.SMTP.RequireTLS {
		tlsConfig := &tls.Config{
			ServerName: c.config.SMTP.Server,
		}
		if err := conn.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("failed to start TLS: %w", err)
		}
	}

	auth := smtp.PlainAuth("", c.config.SMTP.Username, c.config.SMTP.Password, c.config.SMTP.Server)
	if err := conn.Auth(auth); err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	return conn.Quit()
}

// SendEmail sends an email via SMTP
func (c *SMTPClient) SendEmail(to, subject, body, bodyFormat, cc, bcc string) error {
	// Resolve contact names to email addresses
	to = c.config.ResolveEmail(to)
	if cc != "" {
		cc = c.config.ResolveEmail(cc)
	}
	if bcc != "" {
		bcc = c.config.ResolveEmail(bcc)
	}

	// Convert body based on format
	contentType := "text/plain; charset=UTF-8"
	switch strings.ToLower(bodyFormat) {
	case "html":
		contentType = "text/html; charset=UTF-8"
	case "markdown":
		body = markdownToHTML(body)
		contentType = "text/html; charset=UTF-8"
	}

	// Build recipients list
	recipients := []string{to}
	if cc != "" {
		recipients = append(recipients, cc)
	}
	if bcc != "" {
		recipients = append(recipients, bcc)
	}

	// Build email headers and body
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", c.config.MyEmail))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	if cc != "" {
		msg.WriteString(fmt.Sprintf("Cc: %s\r\n", cc))
	}
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString(fmt.Sprintf("Content-Type: %s\r\n", contentType))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	// Connect and send
	addr := fmt.Sprintf("%s:%d", c.config.SMTP.Server, c.config.SMTP.Port)

	auth := smtp.PlainAuth("", c.config.SMTP.Username, c.config.SMTP.Password, c.config.SMTP.Server)

	if c.config.SMTP.RequireTLS {
		return c.sendWithTLS(addr, auth, c.config.MyEmail, recipients, []byte(msg.String()))
	}

	return smtp.SendMail(addr, auth, c.config.MyEmail, recipients, []byte(msg.String()))
}

// sendWithTLS sends email using STARTTLS
func (c *SMTPClient) sendWithTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	// Connect to the server
	conn, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	// Start TLS
	tlsConfig := &tls.Config{
		ServerName: c.config.SMTP.Server,
	}
	if err := conn.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("failed to start TLS: %w", err)
	}

	// Authenticate
	if err := conn.Auth(auth); err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	// Set sender
	if err := conn.Mail(from); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// Set recipients
	for _, recipient := range to {
		if err := conn.Rcpt(recipient); err != nil {
			return fmt.Errorf("failed to set recipient %s: %w", recipient, err)
		}
	}

	// Send the email body
	w, err := conn.Data()
	if err != nil {
		return fmt.Errorf("failed to get data writer: %w", err)
	}

	_, err = w.Write(msg)
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	err = w.Close()
	if err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	return conn.Quit()
}

// markdownToHTML converts markdown text to HTML
func markdownToHTML(md string) string {
	// Create markdown parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs
	p := parser.NewWithExtensions(extensions)

	// Create HTML renderer
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	// Convert
	doc := p.Parse([]byte(md))
	return string(markdown.Render(doc, renderer))
}
