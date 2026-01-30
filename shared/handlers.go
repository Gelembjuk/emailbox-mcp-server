package shared

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterTools registers all MCP tools with the server
func RegisterTools(s *server.MCPServer, config *Config) {
	imapClient := NewIMAPClient(config)
	smtpClient := NewSMTPClient(config)

	// Build contacts description for tool help
	contactsInfo := config.GetContactsDescription()
	senderInfo := fmt.Sprintf("Your email address: %s", config.MyEmail)

	// Register send_email tool
	sendEmailTool := mcp.NewTool("send_email",
		mcp.WithDescription(fmt.Sprintf(`Send an email via SMTP.

%s

%s

You can use contact names instead of email addresses for the 'to', 'cc', and 'bcc' fields.`, senderInfo, contactsInfo)),
		mcp.WithString("to",
			mcp.Required(),
			mcp.Description("Recipient email address or contact name")),
		mcp.WithString("subject",
			mcp.Required(),
			mcp.Description("Email subject")),
		mcp.WithString("body",
			mcp.Required(),
			mcp.Description("Email body content")),
		mcp.WithString("body_format",
			mcp.Description("Body format: 'text' (default), 'markdown', or 'html'")),
		mcp.WithString("cc",
			mcp.Description("CC recipient email address or contact name")),
		mcp.WithString("bcc",
			mcp.Description("BCC recipient email address or contact name")),
	)

	s.AddTool(sendEmailTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		to := request.GetString("to", "")
		subject := request.GetString("subject", "")
		body := request.GetString("body", "")
		bodyFormat := request.GetString("body_format", "text")
		cc := request.GetString("cc", "")
		bcc := request.GetString("bcc", "")

		if to == "" || subject == "" || body == "" {
			return mcp.NewToolResultError("Missing required parameters: to, subject, and body are required"), nil
		}

		err := smtpClient.SendEmail(to, subject, body, bodyFormat, cc, bcc)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to send email: %v", err)), nil
		}

		resolvedTo := config.ResolveEmail(to)
		return mcp.NewToolResultText(fmt.Sprintf("Email sent successfully to %s", resolvedTo)), nil
	})

	// Register get_inbox tool
	getInboxTool := mcp.NewTool("get_inbox",
		mcp.WithDescription("Retrieve emails from the inbox via IMAP."),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of emails to return (default: 20)")),
		mcp.WithBoolean("unread_only",
			mcp.Description("Only return unread emails")),
	)

	s.AddTool(getInboxTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		limit := request.GetInt("limit", 20)
		unreadOnly := request.GetBool("unread_only", false)

		emails, err := imapClient.GetInbox(limit, unreadOnly)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get inbox: %v", err)), nil
		}

		result, err := json.MarshalIndent(map[string]interface{}{
			"emails": emails,
			"count":  len(emails),
		}, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to serialize response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Register get_email_contents tool
	getEmailContentsTool := mcp.NewTool("get_email_contents",
		mcp.WithDescription("Get the full content of a specific email."),
		mcp.WithString("email_id",
			mcp.Required(),
			mcp.Description("ID of the email to retrieve")),
	)

	s.AddTool(getEmailContentsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		emailID := request.GetString("email_id", "")
		if emailID == "" {
			return mcp.NewToolResultError("Missing required parameter: email_id"), nil
		}

		email, err := imapClient.GetEmailContents(emailID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get email: %v", err)), nil
		}

		result, err := json.MarshalIndent(email, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to serialize response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Register mark_email_read tool
	markEmailReadTool := mcp.NewTool("mark_email_read",
		mcp.WithDescription("Mark an email as read on the IMAP server."),
		mcp.WithString("email_id",
			mcp.Required(),
			mcp.Description("ID of the email to mark as read")),
	)

	s.AddTool(markEmailReadTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		emailID := request.GetString("email_id", "")
		if emailID == "" {
			return mcp.NewToolResultError("Missing required parameter: email_id"), nil
		}

		err := imapClient.MarkAsRead(emailID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to mark email as read: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Email %s marked as read", emailID)), nil
	})

	// Register get_attachment tool
	getAttachmentTool := mcp.NewTool("get_attachment",
		mcp.WithDescription("Get the content of an email attachment as a base64-encoded string."),
		mcp.WithString("email_id",
			mcp.Required(),
			mcp.Description("ID of the email containing the attachment")),
		mcp.WithNumber("attachment_index",
			mcp.Required(),
			mcp.Description("Index of the attachment (from get_email_contents attachments list)")),
	)

	s.AddTool(getAttachmentTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		emailID := request.GetString("email_id", "")
		attachmentIndex := request.GetInt("attachment_index", 0)

		if emailID == "" {
			return mcp.NewToolResultError("Missing required parameter: email_id"), nil
		}
		if attachmentIndex <= 0 {
			return mcp.NewToolResultError("Missing or invalid required parameter: attachment_index (must be >= 1)"), nil
		}

		filename, contentType, data, err := imapClient.GetAttachment(emailID, attachmentIndex)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get attachment: %v", err)), nil
		}

		encoded := base64.StdEncoding.EncodeToString(data)

		log.Printf("Returning attachment: filename=%s, content_type=%s, raw_size=%d, base64_size=%d",
			filename, contentType, len(data), len(encoded))

		if strings.HasPrefix(contentType, "image/") {
			return mcp.NewToolResultImage(filename, encoded, contentType), nil
		}

		return mcp.NewToolResultResource(filename, mcp.BlobResourceContents{
			URI:      "attachment://" + filename,
			MIMEType: contentType,
			Blob:     encoded,
		}), nil
	})
}
