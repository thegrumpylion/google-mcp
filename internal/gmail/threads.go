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

// --- gmail_list_threads ---

type listThreadsInput struct {
	Account    string   `json:"account" jsonschema:"Account name or 'all' for all accounts"`
	Query      string   `json:"query,omitempty" jsonschema:"Gmail search query to filter threads (same syntax as Gmail search bar)"`
	MaxResults int64    `json:"max_results,omitempty" jsonschema:"Maximum number of results per account (default 10, max 500)"`
	LabelIDs   []string `json:"label_ids,omitempty" jsonschema:"Only return threads with all of these label IDs"`
}

func registerListThreads(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "list_threads",
		Description: "List Gmail threads. Supports query filtering with Gmail search syntax, label filtering, and multi-account search. Returns thread IDs, snippets, and message counts.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listThreadsInput) (*mcp.CallToolResult, any, error) {
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

			call := svc.Users.Threads.List("me").MaxResults(maxResults)
			if input.Query != "" {
				call = call.Q(input.Query)
			}
			if len(input.LabelIDs) > 0 {
				call = call.LabelIds(input.LabelIDs...)
			}

			resp, err := call.Do()
			if err != nil {
				if multiAccount {
					fmt.Fprintf(&sb, "=== Account: %s ===\nError listing threads: %v\n\n", account, err)
					continue
				}
				return nil, nil, fmt.Errorf("listing threads: %w", err)
			}

			if multiAccount {
				fmt.Fprintf(&sb, "=== Account: %s ===\n", account)
			}

			if len(resp.Threads) == 0 {
				sb.WriteString("No threads found.\n\n")
				continue
			}

			fmt.Fprintf(&sb, "Found %d threads (estimated total: %d):\n\n", len(resp.Threads), resp.ResultSizeEstimate)

			for _, thread := range resp.Threads {
				// Threads.List returns minimal info; fetch metadata for the first message.
				detail, err := svc.Users.Threads.Get("me", thread.Id).Format("metadata").MetadataHeaders("From", "Subject", "Date").Do()
				if err != nil {
					fmt.Fprintf(&sb, "- Thread ID: %s (error fetching details: %v)\n\n", thread.Id, err)
					continue
				}

				fmt.Fprintf(&sb, "- Thread ID: %s\n  Account: %s\n  Messages: %d\n  Snippet: %s\n",
					thread.Id, account, len(detail.Messages), thread.Snippet)

				// Show headers from the first message.
				if len(detail.Messages) > 0 && detail.Messages[0].Payload != nil {
					headers := make(map[string]string)
					for _, h := range detail.Messages[0].Payload.Headers {
						headers[h.Name] = h.Value
					}
					if from := headers["From"]; from != "" {
						fmt.Fprintf(&sb, "  From: %s\n", from)
					}
					if subj := headers["Subject"]; subj != "" {
						fmt.Fprintf(&sb, "  Subject: %s\n", subj)
					}
					if date := headers["Date"]; date != "" {
						fmt.Fprintf(&sb, "  Date: %s\n", date)
					}
				}
				sb.WriteString("\n")
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- gmail_read_thread ---

type readThreadInput struct {
	Account  string `json:"account" jsonschema:"Account name"`
	ThreadID string `json:"thread_id" jsonschema:"Gmail thread ID (from search or read results)"`
}

func registerReadThread(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "read_thread",
		Description: "Read all messages in a Gmail thread/conversation by thread ID. Returns each message with headers and body text in chronological order.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input readThreadInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		thread, err := svc.Users.Threads.Get("me", input.ThreadID).Format("full").Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting thread: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Thread ID: %s\nMessages: %d\n\n", thread.Id, len(thread.Messages))

		for i, msg := range thread.Messages {
			fmt.Fprintf(&sb, "--- Message %d/%d (Message ID: %s) ---\n", i+1, len(thread.Messages), msg.Id)

			// Write headers.
			if msg.Payload != nil {
				for _, h := range msg.Payload.Headers {
					switch h.Name {
					case "From", "To", "Cc", "Subject", "Date":
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
			}

			sb.WriteString("\n\n")
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- gmail_thread_modify ---

type threadModifyInput struct {
	Account      string   `json:"account" jsonschema:"Account name"`
	ThreadID     string   `json:"thread_id" jsonschema:"Gmail thread ID to modify"`
	AddLabels    []string `json:"add_labels,omitempty" jsonschema:"Label IDs to add (e.g. 'STARRED', 'IMPORTANT', 'TRASH', or custom label IDs from list_labels)"`
	RemoveLabels []string `json:"remove_labels,omitempty" jsonschema:"Label IDs to remove (e.g. 'UNREAD', 'INBOX', 'STARRED')"`
}

func registerThreadModify(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name: "modify_thread",
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
		},
		Description: `Modify labels on all messages in a Gmail thread. Use this to archive, trash, star, or mark entire conversations as read/unread.

Common operations:
  - Archive thread: remove_labels=["INBOX"]
  - Trash thread: add_labels=["TRASH"]
  - Mark thread read: remove_labels=["UNREAD"]
  - Star thread: add_labels=["STARRED"]

Use list_labels to discover custom label IDs.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input threadModifyInput) (*mcp.CallToolResult, any, error) {
		if len(input.AddLabels) == 0 && len(input.RemoveLabels) == 0 {
			return nil, nil, fmt.Errorf("at least one of add_labels or remove_labels must be specified")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		modReq := &gmailapi.ModifyThreadRequest{
			AddLabelIds:    input.AddLabels,
			RemoveLabelIds: input.RemoveLabels,
		}

		thread, err := svc.Users.Threads.Modify("me", input.ThreadID, modReq).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("modifying thread: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Thread %s modified (%d messages affected).",
					thread.Id, len(thread.Messages))},
			},
		}, nil, nil
	})
}

// --- gmail_trash_thread ---

type trashThreadInput struct {
	Account  string `json:"account" jsonschema:"Account name"`
	ThreadID string `json:"thread_id" jsonschema:"Gmail thread ID to trash"`
}

func registerTrashThread(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "trash_thread",
		Description: "Move a Gmail thread to the trash. The thread will be permanently deleted after 30 days. Use untrash_thread to restore.",
		Annotations: &mcp.ToolAnnotations{},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input trashThreadInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		thread, err := svc.Users.Threads.Trash("me", input.ThreadID).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("trashing thread: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Thread %s moved to trash.", thread.Id)},
			},
		}, nil, nil
	})
}

// --- gmail_untrash_thread ---

type untrashThreadInput struct {
	Account  string `json:"account" jsonschema:"Account name"`
	ThreadID string `json:"thread_id" jsonschema:"Gmail thread ID to restore from trash"`
}

func registerUntrashThread(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "untrash_thread",
		Description: "Restore a Gmail thread from the trash back to the inbox.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: server.BoolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input untrashThreadInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		thread, err := svc.Users.Threads.Untrash("me", input.ThreadID).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("untrashing thread: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Thread %s restored from trash.", thread.Id)},
			},
		}, nil, nil
	})
}

// TODO: Planned thread tools (from api-coverage.md):
// - delete_thread (Threads.Delete)
