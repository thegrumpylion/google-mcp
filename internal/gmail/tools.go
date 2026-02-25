// Package gmail provides MCP tools for interacting with the Gmail API.
package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
	"google.golang.org/api/gmail/v1"
)

// Scopes required by the Gmail tools.
// MailGoogleComScope grants full access to the mailbox including permanent
// deletion, settings, send, and all read/write operations. It is a superset
// of GmailModifyScope, GmailSendScope, and GmailSettingsBasicScope.
var Scopes = []string{
	gmail.MailGoogleComScope,
}

// RegisterTools registers all Gmail MCP tools on the given server.
func RegisterTools(srv *server.Server, mgr *auth.Manager) {
	server.RegisterAccountsListTool(srv, mgr)
	registerGetProfile(srv, mgr)
	registerSearch(srv, mgr)
	registerRead(srv, mgr)
	registerListThreads(srv, mgr)
	registerReadThread(srv, mgr)
	registerThreadModify(srv, mgr)
	registerTrashThread(srv, mgr)
	registerUntrashThread(srv, mgr)
	registerSend(srv, mgr)
	registerListLabels(srv, mgr)
	registerGetLabel(srv, mgr)
	registerCreateLabel(srv, mgr)
	registerDeleteLabel(srv, mgr)
	registerModify(srv, mgr)
	registerDeleteMessage(srv, mgr)
	registerGetAttachment(srv, mgr)
	registerGetVacation(srv, mgr)
	registerUpdateVacation(srv, mgr)
	registerDraftCreate(srv, mgr)
	registerDraftList(srv, mgr)
	registerDraftGet(srv, mgr)
	registerDraftUpdate(srv, mgr)
	registerDraftDelete(srv, mgr)
	registerDraftSend(srv, mgr)
}

func newService(ctx context.Context, mgr *auth.Manager, account string) (*gmail.Service, error) {
	opt, err := mgr.ClientOption(ctx, account, Scopes)
	if err != nil {
		return nil, err
	}
	return gmail.NewService(ctx, opt)
}

// --- gmail_search ---

type searchInput struct {
	Account    string `json:"account" jsonschema:"Account name or 'all' for all accounts"`
	Query      string `json:"query" jsonschema:"Gmail search query (same syntax as Gmail search bar)"`
	MaxResults int64  `json:"max_results,omitempty" jsonschema:"Maximum number of results per account (default 10, max 500)"`
}

func registerSearch(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "search_messages",
		Description: "Search Gmail messages using Gmail query syntax. Set account to 'all' to search across all accounts. Returns message IDs and snippets. Use read to get full message content.",
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
		if maxResults > 500 {
			maxResults = 500
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
					fmt.Fprintf(&sb, "- Message ID: %s (error fetching details: %v)\n", msg.Id, err)
					continue
				}
				headers := make(map[string]string)
				if detail.Payload != nil {
					for _, h := range detail.Payload.Headers {
						headers[h.Name] = h.Value
					}
				}
				fmt.Fprintf(&sb, "- Message ID: %s\n  Account: %s\n  From: %s\n  Subject: %s\n  Date: %s\n  Snippet: %s\n\n",
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
	Account   string `json:"account" jsonschema:"Account name"`
	MessageID string `json:"message_id" jsonschema:"Gmail message ID (from search results)"`
}

func registerRead(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "read_message",
		Description: "Read the full content of a Gmail message by ID. Returns headers, body text, and attachment list. Use get_attachment to download attachments.",
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
			sb.WriteString("\nUse get_attachment with the message ID and attachment ID to download.")
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
	Account string `json:"account" jsonschema:"Account name"`
	composeInput
	ReplyToMessageID string `json:"reply_to_message_id,omitempty" jsonschema:"Message ID to reply to (sets In-Reply-To and References headers, keeps thread)"`
}

func registerSend(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "send_message",
		Description: "Send an email via Gmail. Supports To, CC, BCC, and replying to existing messages.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: server.BoolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input sendInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		result, err := buildMessage(svc, input.composeInput, input.ReplyToMessageID)
		if err != nil {
			return nil, nil, err
		}

		msg := &gmail.Message{
			Raw:      result.Raw,
			ThreadId: result.ThreadID,
		}

		sent, err := svc.Users.Messages.Send("me", msg).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("sending message: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Message sent.\n\nMessage ID: %s\nThread ID: %s", sent.Id, sent.ThreadId)},
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
	Account string `json:"account" jsonschema:"Account name or 'all' for all accounts"`
}

func registerListLabels(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "list_labels",
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
				fmt.Fprintf(&sb, "  - %s (Label ID: %s, type: %s)\n", label.Name, label.Id, label.Type)
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
