package shared

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds all configuration for the MCP email server
type Config struct {
	IMAP struct {
		Server   string `json:"server"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
		UseTLS   bool   `json:"use_tls"`
	} `json:"imap"`
	SMTP struct {
		Server     string `json:"server"`
		Port       int    `json:"port"`
		Username   string `json:"username"`
		Password   string `json:"password"`
		RequireTLS bool   `json:"require_tls"`
	} `json:"smtp"`
	MyEmail  string            `json:"my_email"`
	Contacts map[string]string `json:"contacts"`
	HTTP     struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	} `json:"http"`
	Notifications struct {
		CheckIntervalSeconds int `json:"check_interval_seconds"`
	} `json:"notifications"`
}

// LoadConfig loads configuration from a JSON file
// If path is empty, it looks for config.json in the same directory as the executable
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		// Get the directory of the executable
		execPath, err := os.Executable()
		if err != nil {
			// Fall back to current working directory
			path = "config.json"
		} else {
			path = filepath.Join(filepath.Dir(execPath), "config.json")
		}

		// If config doesn't exist next to executable, try current directory
		if _, err := os.Stat(path); os.IsNotExist(err) {
			path = "config.json"
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	if config.HTTP.Host == "" {
		config.HTTP.Host = "localhost"
	}
	if config.HTTP.Port == 0 {
		config.HTTP.Port = 8081
	}
	if config.Notifications.CheckIntervalSeconds == 0 {
		config.Notifications.CheckIntervalSeconds = 30
	}
	if config.IMAP.Port == 0 {
		config.IMAP.Port = 993
	}
	if config.SMTP.Port == 0 {
		config.SMTP.Port = 587
	}

	return &config, nil
}

// GetContactsDescription returns a formatted string of contacts for tool descriptions
func (c *Config) GetContactsDescription() string {
	if len(c.Contacts) == 0 {
		return "No contacts configured."
	}

	result := "Available contacts:\n"
	for name, email := range c.Contacts {
		result += fmt.Sprintf("  - %s: %s\n", name, email)
	}
	return result
}

// ResolveEmail resolves a contact name to an email address
// If the input is already an email address, it returns it unchanged
func (c *Config) ResolveEmail(nameOrEmail string) string {
	if email, ok := c.Contacts[nameOrEmail]; ok {
		return email
	}
	return nameOrEmail
}

// ValidateConnections tests both IMAP and SMTP connections
// Returns an error if either connection fails
func ValidateConnections(config *Config) error {
	// Test IMAP connection
	imapClient := NewIMAPClient(config)
	if err := imapClient.ValidateConnection(); err != nil {
		return fmt.Errorf("IMAP connection failed: %w", err)
	}

	// Test SMTP connection
	smtpClient := NewSMTPClient(config)
	if err := smtpClient.ValidateConnection(); err != nil {
		return fmt.Errorf("SMTP connection failed: %w", err)
	}

	return nil
}
