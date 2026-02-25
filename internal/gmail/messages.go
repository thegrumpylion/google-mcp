package gmail

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
	gmailapi "google.golang.org/api/gmail/v1"
)

// --- search_messages ---

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

// --- read_message ---

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

// --- send_message ---

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

		msg := &gmailapi.Message{
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

// --- modify_messages ---

type modifyInput struct {
	Account      string   `json:"account" jsonschema:"Account name"`
	MessageIDs   []string `json:"message_ids" jsonschema:"Gmail message IDs to modify (one or more)"`
	AddLabels    []string `json:"add_labels,omitempty" jsonschema:"Label IDs to add (e.g. 'STARRED', 'IMPORTANT', 'TRASH', or custom label IDs from list_labels)"`
	RemoveLabels []string `json:"remove_labels,omitempty" jsonschema:"Label IDs to remove (e.g. 'UNREAD', 'INBOX', 'STARRED')"`
}

// Common label operations as a reference:
//   Archive:     remove INBOX
//   Trash:       add TRASH
//   Mark read:   remove UNREAD
//   Mark unread: add UNREAD
//   Star:        add STARRED
//   Unstar:      remove STARRED

func registerModify(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name: "modify_messages",
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
		},
		Description: `Modify labels on one or more Gmail messages. Use this to archive, trash, star, or mark messages as read/unread.

Accepts one or more message IDs in message_ids. Uses Gmail batch API for efficiency.

Common operations:
  - Archive: remove_labels=["INBOX"]
  - Trash: add_labels=["TRASH"]
  - Mark read: remove_labels=["UNREAD"]
  - Mark unread: add_labels=["UNREAD"]
  - Star: add_labels=["STARRED"]
  - Unstar: remove_labels=["STARRED"]

Use list_labels to discover custom label IDs.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input modifyInput) (*mcp.CallToolResult, any, error) {
		if len(input.MessageIDs) == 0 {
			return nil, nil, fmt.Errorf("message_ids must contain at least one message ID")
		}
		if len(input.AddLabels) == 0 && len(input.RemoveLabels) == 0 {
			return nil, nil, fmt.Errorf("at least one of add_labels or remove_labels must be specified")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		batchReq := &gmailapi.BatchModifyMessagesRequest{
			Ids:            input.MessageIDs,
			AddLabelIds:    input.AddLabels,
			RemoveLabelIds: input.RemoveLabels,
		}

		if err := svc.Users.Messages.BatchModify("me", batchReq).Do(); err != nil {
			return nil, nil, fmt.Errorf("modifying messages: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Modified %d messages.", len(input.MessageIDs))},
			},
		}, nil, nil
	})
}

// --- delete_message ---

type deleteMessageInput struct {
	Account   string `json:"account" jsonschema:"Account name"`
	MessageID string `json:"message_id" jsonschema:"Gmail message ID to permanently delete"`
}

func registerDeleteMessage(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "delete_message",
		Description: "Permanently delete a Gmail message. This action bypasses the trash and is irreversible. The message cannot be recovered.",
		Annotations: &mcp.ToolAnnotations{},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input deleteMessageInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		if err := svc.Users.Messages.Delete("me", input.MessageID).Do(); err != nil {
			return nil, nil, fmt.Errorf("deleting message: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Message %s permanently deleted.", input.MessageID)},
			},
		}, nil, nil
	})
}

// TODO: Planned message tools (from api-coverage.md):
// - trash_message (Messages.Trash)
// - untrash_message (Messages.Untrash)
// - batch_delete_messages (Messages.BatchDelete)
