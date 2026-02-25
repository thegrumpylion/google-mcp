// Package gmail provides MCP tools for interacting with the Gmail API.
package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"google.golang.org/api/gmail/v1"
)

// Scopes required by the Gmail tools.
// GmailModifyScope is a superset of GmailReadonlyScope and covers
// label changes, drafts, and message modifications.
var Scopes = []string{
	gmail.GmailModifyScope,
	gmail.GmailSendScope,
}

// RegisterTools registers all Gmail MCP tools on the given server.
func RegisterTools(server *mcp.Server, mgr *auth.Manager) {
	registerAccountsList(server, mgr)
	registerSearch(server, mgr)
	registerRead(server, mgr)
	registerReadThread(server, mgr)
	registerSend(server, mgr)
	registerListLabels(server, mgr)
	registerModify(server, mgr)
	registerGetAttachment(server, mgr)
	registerDraftCreate(server, mgr)
	registerDraftList(server, mgr)
	registerDraftSend(server, mgr)
}

func newService(ctx context.Context, mgr *auth.Manager, account string) (*gmail.Service, error) {
	opt, err := mgr.ClientOption(ctx, account, Scopes)
	if err != nil {
		return nil, err
	}
	return gmail.NewService(ctx, opt)
}

// --- accounts_list ---

func registerAccountsList(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "accounts_list",
		Description: "List all configured Google accounts. Use this to discover available account names.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
		accounts := mgr.ListAccounts()
		if len(accounts) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "No accounts configured. Run 'google-mcp auth add <name>' to add one."},
				},
			}, nil, nil
		}
		var sb strings.Builder
		sb.WriteString("Configured accounts:\n")
		for name, email := range accounts {
			if email != "" {
				fmt.Fprintf(&sb, "  - %s (%s)\n", name, email)
			} else {
				fmt.Fprintf(&sb, "  - %s\n", name)
			}
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- gmail_search ---

type searchInput struct {
	Account    string `json:"account" jsonschema:"Account name (e.g. 'personal', 'work') or 'all' to search all accounts"`
	Query      string `json:"query" jsonschema:"Gmail search query (same syntax as Gmail search bar)"`
	MaxResults int64  `json:"max_results,omitempty" jsonschema:"Maximum number of results per account (default 10, max 50)"`
}

func registerSearch(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gmail_search",
		Description: "Search Gmail messages using Gmail query syntax. Set account to 'all' to search across all accounts. Returns message IDs and snippets. Use gmail_read to get full message content.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, any, error) {
		accounts, err := mgr.ResolveAccounts(input.Account)
		if err != nil {
			return nil, nil, err
		}

		maxResults := input.MaxResults
		if maxResults <= 0 {
			maxResults = 10
		}
		if maxResults > 50 {
			maxResults = 50
		}

		var sb strings.Builder
		multiAccount := len(accounts) > 1

		for _, account := range accounts {
			svc, err := newService(ctx, mgr, account)
			if err != nil {
				if multiAccount {
					fmt.Fprintf(&sb, "=== Account: %s ===\nError: %v\n\n", account, err)
					continue
				}
				return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
			}

			resp, err := svc.Users.Messages.List("me").Q(input.Query).MaxResults(maxResults).Do()
			if err != nil {
				if multiAccount {
					fmt.Fprintf(&sb, "=== Account: %s ===\nError searching: %v\n\n", account, err)
					continue
				}
				return nil, nil, fmt.Errorf("searching messages: %w", err)
			}

			if multiAccount {
				fmt.Fprintf(&sb, "=== Account: %s ===\n", account)
			}

			if len(resp.Messages) == 0 {
				sb.WriteString("No messages found.\n\n")
				continue
			}

			fmt.Fprintf(&sb, "Found %d messages (estimated total: %d):\n\n", len(resp.Messages), resp.ResultSizeEstimate)

			for _, msg := range resp.Messages {
				detail, err := svc.Users.Messages.Get("me", msg.Id).Format("metadata").MetadataHeaders("From", "Subject", "Date").Do()
				if err != nil {
					fmt.Fprintf(&sb, "- ID: %s (error fetching details: %v)\n", msg.Id, err)
					continue
				}
				headers := make(map[string]string)
				if detail.Payload != nil {
					for _, h := range detail.Payload.Headers {
						headers[h.Name] = h.Value
					}
				}
				fmt.Fprintf(&sb, "- ID: %s\n  Account: %s\n  From: %s\n  Subject: %s\n  Date: %s\n  Snippet: %s\n\n",
					msg.Id, account, headers["From"], headers["Subject"], headers["Date"], detail.Snippet)
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- gmail_read ---

type readInput struct {
	Account   string `json:"account" jsonschema:"Account name to use"`
	MessageID string `json:"message_id" jsonschema:"Gmail message ID (from gmail_search results)"`
}

func registerRead(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gmail_read",
		Description: "Read the full content of a Gmail message by ID. Returns headers, body text, and attachment list. Use gmail_get_attachment to download attachments.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input readInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		msg, err := svc.Users.Messages.Get("me", input.MessageID).Format("full").Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting message: %w", err)
		}

		var sb strings.Builder

		// Write headers.
		fmt.Fprintf(&sb, "Thread ID: %s\n", msg.ThreadId)
		if msg.Payload != nil {
			for _, h := range msg.Payload.Headers {
				switch h.Name {
				case "From", "To", "Cc", "Bcc", "Subject", "Date", "Reply-To":
					fmt.Fprintf(&sb, "%s: %s\n", h.Name, h.Value)
				}
			}
		}
		sb.WriteString("\n")

		// Extract body text.
		body := extractBody(msg.Payload)
		if body != "" {
			sb.WriteString(body)
		} else {
			sb.WriteString("(no text content)")
		}

		// List attachments.
		attachments := listAttachments(msg.Payload)
		if len(attachments) > 0 {
			sb.WriteString("\n\nAttachments:\n")
			for _, a := range attachments {
				fmt.Fprintf(&sb, "  - %s (MIME: %s, Size: %d bytes, Attachment ID: %s)\n",
					a.filename, a.mimeType, a.size, a.attachmentID)
			}
			sb.WriteString("\nUse gmail_get_attachment with the message ID and attachment ID to download.")
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// extractBody recursively extracts text content from a message payload.
func extractBody(part *gmail.MessagePart) string {
	if part == nil {
		return ""
	}

	// Prefer text/plain, fall back to text/html.
	if part.MimeType == "text/plain" && part.Body != nil && part.Body.Data != "" {
		data, err := base64.URLEncoding.DecodeString(part.Body.Data)
		if err != nil {
			return "(error decoding body)"
		}
		return string(data)
	}

	// For multipart messages, recurse into parts.
	// Prefer text/plain over text/html.
	var htmlBody string
	for _, p := range part.Parts {
		if p.MimeType == "text/plain" {
			if body := extractBody(p); body != "" {
				return body
			}
		}
		if p.MimeType == "text/html" {
			htmlBody = extractBody(p)
		}
		// Recurse into nested multipart.
		if strings.HasPrefix(p.MimeType, "multipart/") {
			if body := extractBody(p); body != "" {
				return body
			}
		}
	}

	// Fall back to HTML if no plain text found.
	if htmlBody == "" && part.MimeType == "text/html" && part.Body != nil && part.Body.Data != "" {
		data, err := base64.URLEncoding.DecodeString(part.Body.Data)
		if err != nil {
			return "(error decoding body)"
		}
		return string(data)
	}

	return htmlBody
}

// --- gmail_send ---

type sendInput struct {
	Account string `json:"account" jsonschema:"Account name to send from"`
	To      string `json:"to" jsonschema:"Recipient email address"`
	Subject string `json:"subject" jsonschema:"Email subject line"`
	Body    string `json:"body" jsonschema:"Email body (plain text)"`
	Cc      string `json:"cc,omitempty" jsonschema:"CC recipients (comma-separated email addresses)"`
	Bcc     string `json:"bcc,omitempty" jsonschema:"BCC recipients (comma-separated email addresses)"`
	ReplyTo string `json:"reply_to,omitempty" jsonschema:"Message ID to reply to (sets In-Reply-To and References headers)"`
}

func registerSend(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gmail_send",
		Description: "Send an email via Gmail. Supports To, CC, BCC, and replying to existing messages.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input sendInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		// Build the RFC 2822 message.
		var raw strings.Builder
		fmt.Fprintf(&raw, "To: %s\r\n", input.To)
		if input.Cc != "" {
			fmt.Fprintf(&raw, "Cc: %s\r\n", input.Cc)
		}
		if input.Bcc != "" {
			fmt.Fprintf(&raw, "Bcc: %s\r\n", input.Bcc)
		}
		fmt.Fprintf(&raw, "Subject: %s\r\n", mime2047Encode(input.Subject))
		fmt.Fprintf(&raw, "Content-Type: text/plain; charset=\"UTF-8\"\r\n")
		if input.ReplyTo != "" {
			// Fetch the original message to get its Message-ID header.
			origMsg, err := svc.Users.Messages.Get("me", input.ReplyTo).Format("metadata").MetadataHeaders("Message-Id").Do()
			if err == nil && origMsg.Payload != nil {
				for _, h := range origMsg.Payload.Headers {
					if h.Name == "Message-Id" {
						fmt.Fprintf(&raw, "In-Reply-To: %s\r\n", h.Value)
						fmt.Fprintf(&raw, "References: %s\r\n", h.Value)
					}
				}
			}
		}
		raw.WriteString("\r\n")
		raw.WriteString(input.Body)

		msg := &gmail.Message{
			Raw:      base64.URLEncoding.EncodeToString([]byte(raw.String())),
			ThreadId: input.ReplyTo, // keeps it in the same thread if replying
		}

		sent, err := svc.Users.Messages.Send("me", msg).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("sending message: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Message sent successfully. ID: %s, Thread: %s", sent.Id, sent.ThreadId)},
			},
		}, nil, nil
	})
}

// mime2047Encode performs a simple RFC 2047 encoding for the Subject header.
func mime2047Encode(s string) string {
	// Check if encoding is needed (non-ASCII characters).
	for _, r := range s {
		if r > 127 {
			return fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(s)))
		}
	}
	return s
}

// --- gmail_list_labels ---

type listLabelsInput struct {
	Account string `json:"account" jsonschema:"Account name (e.g. 'personal', 'work') or 'all' to list labels from all accounts"`
}

func registerListLabels(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gmail_list_labels",
		Description: "List all Gmail labels for an account. Set account to 'all' to list labels from all accounts. Useful for filtering searches.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listLabelsInput) (*mcp.CallToolResult, any, error) {
		accounts, err := mgr.ResolveAccounts(input.Account)
		if err != nil {
			return nil, nil, err
		}

		var sb strings.Builder
		multiAccount := len(accounts) > 1

		for _, account := range accounts {
			svc, err := newService(ctx, mgr, account)
			if err != nil {
				if multiAccount {
					fmt.Fprintf(&sb, "=== Account: %s ===\nError: %v\n\n", account, err)
					continue
				}
				return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
			}

			resp, err := svc.Users.Labels.List("me").Do()
			if err != nil {
				if multiAccount {
					fmt.Fprintf(&sb, "=== Account: %s ===\nError listing labels: %v\n\n", account, err)
					continue
				}
				return nil, nil, fmt.Errorf("listing labels: %w", err)
			}

			if multiAccount {
				fmt.Fprintf(&sb, "=== Account: %s ===\n", account)
			}
			sb.WriteString("Gmail labels:\n")
			for _, label := range resp.Labels {
				fmt.Fprintf(&sb, "  - %s (ID: %s, type: %s)\n", label.Name, label.Id, label.Type)
			}
			sb.WriteString("\n")
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// AccountScopes returns the scopes used by Gmail tools.
func AccountScopes() []string {
	return Scopes
}
