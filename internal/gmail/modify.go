package gmail

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	gmailapi "google.golang.org/api/gmail/v1"
)

// --- gmail_modify ---

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

func registerModify(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
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
