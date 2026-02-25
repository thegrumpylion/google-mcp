package gmail

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	gmailapi "google.golang.org/api/gmail/v1"
)

// --- gmail_modify ---

type modifyInput struct {
	Account      string   `json:"account" jsonschema:"Account name to use"`
	MessageID    string   `json:"message_id,omitempty" jsonschema:"Gmail message ID to modify (for single message)"`
	MessageIDs   []string `json:"message_ids,omitempty" jsonschema:"Gmail message IDs to modify in batch (for multiple messages)"`
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
		Name: "modify",
		Description: `Modify labels on Gmail messages. Use this to archive, trash, star, or mark messages as read/unread.

Supports both single and batch operations:
  - Single: provide message_id for one message
  - Batch: provide message_ids array for multiple messages at once

Common operations:
  - Archive: remove_labels=["INBOX"]
  - Trash: add_labels=["TRASH"]
  - Mark read: remove_labels=["UNREAD"]
  - Mark unread: add_labels=["UNREAD"]
  - Star: add_labels=["STARRED"]
  - Unstar: remove_labels=["STARRED"]

Use list_labels to discover custom label IDs.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input modifyInput) (*mcp.CallToolResult, any, error) {
		if len(input.AddLabels) == 0 && len(input.RemoveLabels) == 0 {
			return nil, nil, fmt.Errorf("at least one of add_labels or remove_labels must be specified")
		}

		// Determine which mode: single or batch.
		hasSingle := input.MessageID != ""
		hasBatch := len(input.MessageIDs) > 0

		if !hasSingle && !hasBatch {
			return nil, nil, fmt.Errorf("either message_id or message_ids must be specified")
		}
		if hasSingle && hasBatch {
			return nil, nil, fmt.Errorf("specify either message_id or message_ids, not both")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		// Batch modify.
		if hasBatch {
			batchReq := &gmailapi.BatchModifyMessagesRequest{
				Ids:            input.MessageIDs,
				AddLabelIds:    input.AddLabels,
				RemoveLabelIds: input.RemoveLabels,
			}

			if err := svc.Users.Messages.BatchModify("me", batchReq).Do(); err != nil {
				return nil, nil, fmt.Errorf("batch modifying messages: %w", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Batch modified %d messages.", len(input.MessageIDs))},
				},
			}, nil, nil
		}

		// Single modify.
		modReq := &gmailapi.ModifyMessageRequest{
			AddLabelIds:    input.AddLabels,
			RemoveLabelIds: input.RemoveLabels,
		}

		msg, err := svc.Users.Messages.Modify("me", input.MessageID, modReq).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("modifying message: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Message %s modified. Current labels: %s",
					msg.Id, strings.Join(msg.LabelIds, ", "))},
			},
		}, nil, nil
	})
}
