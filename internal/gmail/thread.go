package gmail

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
)

// --- gmail_read_thread ---

type readThreadInput struct {
	Account  string `json:"account" jsonschema:"Account name to use"`
	ThreadID string `json:"thread_id" jsonschema:"Gmail thread ID (from search or read results)"`
}

func registerReadThread(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
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
			fmt.Fprintf(&sb, "--- Message %d/%d (ID: %s) ---\n", i+1, len(thread.Messages), msg.Id)

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
