package shared

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	gomessage "github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
)

// Email represents basic email information
type Email struct {
	ID      string    `json:"id"`
	From    string    `json:"from"`
	Subject string    `json:"subject"`
	Date    time.Time `json:"date"`
	Read    bool      `json:"read"`
}

// AttachmentInfo represents metadata about an email attachment
type AttachmentInfo struct {
	Index       int    `json:"index"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        uint32 `json:"size"`
}

// EmailDetail represents full email content
type EmailDetail struct {
	ID          string           `json:"id"`
	From        string           `json:"from"`
	To          []string         `json:"to"`
	CC          []string         `json:"cc,omitempty"`
	Subject     string           `json:"subject"`
	Date        time.Time        `json:"date"`
	Body        string           `json:"body"`
	ContentType string           `json:"content_type"`
	Read        bool             `json:"read"`
	Attachments []AttachmentInfo `json:"attachments,omitempty"`
}

// IMAPClient handles IMAP operations
type IMAPClient struct {
	config *Config
}

// NewIMAPClient creates a new IMAP client
func NewIMAPClient(config *Config) *IMAPClient {
	return &IMAPClient{config: config}
}

// ValidateConnection tests the IMAP connection and credentials
func (c *IMAPClient) ValidateConnection() error {
	client, err := c.Connect()
	if err != nil {
		return err
	}
	client.Close()
	return nil
}

// Connect establishes a connection to the IMAP server
func (c *IMAPClient) Connect() (*imapclient.Client, error) {
	addr := fmt.Sprintf("%s:%d", c.config.IMAP.Server, c.config.IMAP.Port)

	var client *imapclient.Client
	var err error

	if c.config.IMAP.UseTLS {
		client, err = imapclient.DialTLS(addr, &imapclient.Options{
			TLSConfig: &tls.Config{
				ServerName: c.config.IMAP.Server,
			},
		})
	} else {
		client, err = imapclient.DialInsecure(addr, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to IMAP server: %w", err)
	}

	// Login
	if err := client.Login(c.config.IMAP.Username, c.config.IMAP.Password).Wait(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to login: %w", err)
	}

	return client, nil
}

// GetInbox retrieves emails from the inbox
func (c *IMAPClient) GetInbox(limit int, unreadOnly bool) ([]*Email, error) {
	client, err := c.Connect()
	if err != nil {
		return nil, err
	}
	defer client.Close()

	// Select INBOX
	mbox, err := client.Select("INBOX", nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("failed to select INBOX: %w", err)
	}

	if mbox.NumMessages == 0 {
		return []*Email{}, nil
	}

	// Calculate sequence range (get the latest emails)
	var seqSet imap.SeqSet
	start := uint32(1)
	if mbox.NumMessages > uint32(limit) {
		start = mbox.NumMessages - uint32(limit) + 1
	}
	seqSet.AddRange(start, mbox.NumMessages)

	// Fetch options
	fetchOptions := &imap.FetchOptions{
		Envelope: true,
		Flags:    true,
		UID:      true,
	}

	// Fetch messages
	messages, err := client.Fetch(seqSet, fetchOptions).Collect()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch messages: %w", err)
	}

	var emails []*Email
	for _, msg := range messages {
		isRead := false
		for _, flag := range msg.Flags {
			if flag == imap.FlagSeen {
				isRead = true
				break
			}
		}

		// Skip read emails if unreadOnly is true
		if unreadOnly && isRead {
			continue
		}

		from := ""
		if msg.Envelope != nil && len(msg.Envelope.From) > 0 {
			addr := msg.Envelope.From[0]
			if addr.Name != "" {
				from = fmt.Sprintf("%s <%s>", addr.Name, addr.Addr())
			} else {
				from = addr.Addr()
			}
		}

		subject := ""
		date := time.Time{}
		if msg.Envelope != nil {
			subject = msg.Envelope.Subject
			date = msg.Envelope.Date
		}

		emails = append(emails, &Email{
			ID:      strconv.FormatUint(uint64(msg.UID), 10),
			From:    from,
			Subject: subject,
			Date:    date,
			Read:    isRead,
		})
	}

	// Reverse to show newest first
	for i, j := 0, len(emails)-1; i < j; i, j = i+1, j-1 {
		emails[i], emails[j] = emails[j], emails[i]
	}

	return emails, nil
}

// GetEmailContents retrieves the full content of an email
func (c *IMAPClient) GetEmailContents(emailID string) (*EmailDetail, error) {
	uid, err := strconv.ParseUint(emailID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid email ID: %w", err)
	}

	client, err := c.Connect()
	if err != nil {
		return nil, err
	}
	defer client.Close()

	// Select INBOX
	_, err = client.Select("INBOX", nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("failed to select INBOX: %w", err)
	}

	// Create UID set
	var uidSet imap.UIDSet
	uidSet.AddNum(imap.UID(uid))

	// Fetch with body
	fetchOptions := &imap.FetchOptions{
		Envelope:    true,
		Flags:       true,
		UID:         true,
		BodySection: []*imap.FetchItemBodySection{{}},
	}

	messages, err := client.Fetch(uidSet, fetchOptions).Collect()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch message: %w", err)
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("email not found")
	}

	msg := messages[0]

	isRead := false
	for _, flag := range msg.Flags {
		if flag == imap.FlagSeen {
			isRead = true
			break
		}
	}

	from := ""
	var to, cc []string
	subject := ""
	date := time.Time{}

	if msg.Envelope != nil {
		if len(msg.Envelope.From) > 0 {
			addr := msg.Envelope.From[0]
			if addr.Name != "" {
				from = fmt.Sprintf("%s <%s>", addr.Name, addr.Addr())
			} else {
				from = addr.Addr()
			}
		}

		for _, addr := range msg.Envelope.To {
			to = append(to, addr.Addr())
		}

		for _, addr := range msg.Envelope.Cc {
			cc = append(cc, addr.Addr())
		}

		subject = msg.Envelope.Subject
		date = msg.Envelope.Date
	}

	// Get body content
	body := ""
	contentType := "text/plain"
	var attachments []AttachmentInfo

	var rawBody []byte
	if data := msg.FindBodySection(&imap.FetchItemBodySection{}); data != nil {
		rawBody = data
	} else if len(msg.BodySection) > 0 {
		rawBody = msg.BodySection[0].Bytes
	}

	if rawBody != nil {
		body, contentType, attachments = parseEmailBodyWithGoMessage(rawBody)
	}

	return &EmailDetail{
		ID:          emailID,
		From:        from,
		To:          to,
		CC:          cc,
		Subject:     subject,
		Date:        date,
		Body:        body,
		ContentType: contentType,
		Read:        isRead,
		Attachments: attachments,
	}, nil
}

// MarkAsRead marks an email as read
func (c *IMAPClient) MarkAsRead(emailID string) error {
	uid, err := strconv.ParseUint(emailID, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid email ID: %w", err)
	}

	client, err := c.Connect()
	if err != nil {
		return err
	}
	defer client.Close()

	// Select INBOX (not read-only)
	_, err = client.Select("INBOX", nil).Wait()
	if err != nil {
		return fmt.Errorf("failed to select INBOX: %w", err)
	}

	// Create UID set
	var uidSet imap.UIDSet
	uidSet.AddNum(imap.UID(uid))

	// Add \Seen flag
	storeFlags := &imap.StoreFlags{
		Op:     imap.StoreFlagsAdd,
		Flags:  []imap.Flag{imap.FlagSeen},
		Silent: true,
	}

	if err := client.Store(uidSet, storeFlags, nil).Close(); err != nil {
		return fmt.Errorf("failed to mark email as read: %w", err)
	}

	return nil
}

// GetLatestUID returns the highest UID in the inbox
func (c *IMAPClient) GetLatestUID() (uint32, error) {
	client, err := c.Connect()
	if err != nil {
		return 0, err
	}
	defer client.Close()

	// Select INBOX
	mbox, err := client.Select("INBOX", nil).Wait()
	if err != nil {
		return 0, fmt.Errorf("failed to select INBOX: %w", err)
	}

	if mbox.NumMessages == 0 {
		return 0, nil
	}

	// Fetch the last message to get its UID
	var seqSet imap.SeqSet
	seqSet.AddNum(mbox.NumMessages)

	fetchOptions := &imap.FetchOptions{
		UID: true,
	}

	messages, err := client.Fetch(seqSet, fetchOptions).Collect()
	if err != nil {
		return 0, fmt.Errorf("failed to fetch latest message: %w", err)
	}

	if len(messages) == 0 {
		return 0, nil
	}

	return uint32(messages[0].UID), nil
}

// GetEmailsSinceUID retrieves emails with UID greater than the given UID.
// Uses UIDNEXT from SELECT to quickly detect if new messages exist,
// then fetches them using UID FETCH with a range.
func (c *IMAPClient) GetEmailsSinceUID(sinceUID uint32) ([]*Email, error) {
	client, err := c.Connect()
	if err != nil {
		return nil, err
	}
	defer client.Close()

	// Select INBOX - check UIDNEXT to see if new messages exist
	mbox, err := client.Select("INBOX", nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("failed to select INBOX: %w", err)
	}

	// If UIDNEXT <= sinceUID+1, no new messages
	if uint32(mbox.UIDNext) <= sinceUID+1 {
		return []*Email{}, nil
	}

	// Fetch messages with UID from sinceUID+1 to * (0 means *)
	var uidSet imap.UIDSet
	uidSet.AddRange(imap.UID(sinceUID+1), imap.UID(0))

	fetchOptions := &imap.FetchOptions{
		Envelope: true,
		Flags:    true,
		UID:      true,
	}

	messages, err := client.Fetch(uidSet, fetchOptions).Collect()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch messages: %w", err)
	}

	var emails []*Email
	for _, msg := range messages {
		// Skip if UID is not actually greater than sinceUID
		if uint32(msg.UID) <= sinceUID {
			continue
		}

		isRead := false
		for _, flag := range msg.Flags {
			if flag == imap.FlagSeen {
				isRead = true
				break
			}
		}

		from := ""
		if msg.Envelope != nil && len(msg.Envelope.From) > 0 {
			addr := msg.Envelope.From[0]
			if addr.Name != "" {
				from = fmt.Sprintf("%s <%s>", addr.Name, addr.Addr())
			} else {
				from = addr.Addr()
			}
		}

		subject := ""
		date := time.Time{}
		if msg.Envelope != nil {
			subject = msg.Envelope.Subject
			date = msg.Envelope.Date
		}

		emails = append(emails, &Email{
			ID:      strconv.FormatUint(uint64(msg.UID), 10),
			From:    from,
			Subject: subject,
			Date:    date,
			Read:    isRead,
		})
	}

	return emails, nil
}

// parseEmailBodyWithGoMessage uses go-message to parse MIME email bodies
func parseEmailBodyWithGoMessage(data []byte) (body string, contentType string, attachments []AttachmentInfo) {
	contentType = "text/plain"

	entity, err := gomessage.Read(bytes.NewReader(data))
	if err != nil {
		// Fallback: return raw content
		return string(data), contentType, nil
	}

	if mr := entity.MultipartReader(); mr != nil {
		// Multipart message
		partIndex := 0
		var textBody, htmlBody string

		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			partIndex++

			partContentType, params, _ := part.Header.ContentType()
			disposition, dparams, _ := part.Header.ContentDisposition()

			if disposition == "attachment" || (disposition == "inline" && dparams["filename"] != "") {
				// Read to determine size
				partData, readErr := io.ReadAll(part.Body)
				size := uint32(0)
				if readErr == nil {
					size = uint32(len(partData))
				}

				filename := dparams["filename"]
				if filename == "" {
					filename = params["name"]
				}

				attachments = append(attachments, AttachmentInfo{
					Index:       partIndex,
					Filename:    filename,
					ContentType: partContentType,
					Size:        size,
				})
				continue
			}

			if strings.HasPrefix(partContentType, "text/plain") {
				b, readErr := io.ReadAll(part.Body)
				if readErr == nil {
					textBody = string(b)
				}
			} else if strings.HasPrefix(partContentType, "text/html") {
				b, readErr := io.ReadAll(part.Body)
				if readErr == nil {
					htmlBody = string(b)
				}
			} else if strings.HasPrefix(partContentType, "multipart/") {
				// Nested multipart — parse inner parts
				innerReader := mail.NewReader(part)
				for {
					innerPart, err := innerReader.NextPart()
					if err != nil {
						break
					}
					partIndex++

					innerCT, innerParams, _ := mime.ParseMediaType(innerPart.Header.Get("Content-Type"))
					innerDisp, innerDparams, _ := mime.ParseMediaType(innerPart.Header.Get("Content-Disposition"))

					if innerDisp == "attachment" || (innerDisp == "inline" && innerDparams["filename"] != "") {
						partData, readErr := io.ReadAll(innerPart.Body)
						size := uint32(0)
						if readErr == nil {
							size = uint32(len(partData))
						}
						filename := innerDparams["filename"]
						if filename == "" {
							filename = innerParams["name"]
						}
						attachments = append(attachments, AttachmentInfo{
							Index:       partIndex,
							Filename:    filename,
							ContentType: innerCT,
							Size:        size,
						})
						continue
					}

					if strings.HasPrefix(innerCT, "text/plain") && textBody == "" {
						b, readErr := io.ReadAll(innerPart.Body)
						if readErr == nil {
							textBody = string(b)
						}
					} else if strings.HasPrefix(innerCT, "text/html") && htmlBody == "" {
						b, readErr := io.ReadAll(innerPart.Body)
						if readErr == nil {
							htmlBody = string(b)
						}
					}
				}
			}
		}

		if textBody != "" {
			body = textBody
			contentType = "text/plain"
		} else if htmlBody != "" {
			body = htmlBody
			contentType = "text/html"
		}
	} else {
		// Single-part message
		ct, _, _ := entity.Header.ContentType()
		if ct != "" {
			contentType = ct
		}
		b, readErr := io.ReadAll(entity.Body)
		if readErr == nil {
			body = string(b)
		}
	}

	return body, contentType, attachments
}

// GetAttachment retrieves a specific attachment from an email by part index
func (c *IMAPClient) GetAttachment(emailID string, partIndex int) (filename, contentType string, data []byte, err error) {
	uid, err := strconv.ParseUint(emailID, 10, 32)
	if err != nil {
		return "", "", nil, fmt.Errorf("invalid email ID: %w", err)
	}

	client, err := c.Connect()
	if err != nil {
		return "", "", nil, err
	}
	defer client.Close()

	_, err = client.Select("INBOX", nil).Wait()
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to select INBOX: %w", err)
	}

	var uidSet imap.UIDSet
	uidSet.AddNum(imap.UID(uid))

	// Fetch full body to parse with go-message
	fetchOptions := &imap.FetchOptions{
		UID:         true,
		BodySection: []*imap.FetchItemBodySection{{}},
	}

	messages, err := client.Fetch(uidSet, fetchOptions).Collect()
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to fetch message: %w", err)
	}
	if len(messages) == 0 {
		return "", "", nil, fmt.Errorf("email not found")
	}

	msg := messages[0]
	var rawBody []byte
	if d := msg.FindBodySection(&imap.FetchItemBodySection{}); d != nil {
		rawBody = d
	} else if len(msg.BodySection) > 0 {
		rawBody = msg.BodySection[0].Bytes
	}
	if rawBody == nil {
		return "", "", nil, fmt.Errorf("no body data")
	}

	entity, err := gomessage.Read(bytes.NewReader(rawBody))
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to parse message: %w", err)
	}

	mr := entity.MultipartReader()
	if mr == nil {
		return "", "", nil, fmt.Errorf("message is not multipart, no attachments")
	}

	currentIndex := 0
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		currentIndex++

		if currentIndex == partIndex {
			ct, params, _ := part.Header.ContentType()
			_, dparams, _ := part.Header.ContentDisposition()
			fn := dparams["filename"]
			if fn == "" {
				fn = params["name"]
			}

			partData, readErr := io.ReadAll(part.Body)
			if readErr != nil {
				return "", "", nil, fmt.Errorf("failed to read attachment: %w", readErr)
			}
			return fn, ct, partData, nil
		}

		// Check nested multipart
		nestedCT, _, _ := part.Header.ContentType()
		if strings.HasPrefix(nestedCT, "multipart/") {
			innerReader := mail.NewReader(part)
			for {
				innerPart, err := innerReader.NextPart()
				if err != nil {
					break
				}
				currentIndex++
				if currentIndex == partIndex {
					ct, params, _ := mime.ParseMediaType(innerPart.Header.Get("Content-Type"))
					_, dparams, _ := mime.ParseMediaType(innerPart.Header.Get("Content-Disposition"))
					fn := dparams["filename"]
					if fn == "" {
						fn = params["name"]
					}
					partData, readErr := io.ReadAll(innerPart.Body)
					if readErr != nil {
						return "", "", nil, fmt.Errorf("failed to read attachment: %w", readErr)
					}
					return fn, ct, partData, nil
				}
			}
		}
	}

	return "", "", nil, fmt.Errorf("attachment at index %d not found", partIndex)
}

// GetEmailPreview returns a preview of an email body (first ~100 chars)
func GetEmailPreview(body string, maxLen int) string {
	body = strings.TrimSpace(body)
	// Remove excessive whitespace
	body = strings.Join(strings.Fields(body), " ")

	if len(body) <= maxLen {
		return body
	}
	return body[:maxLen] + "..."
}

