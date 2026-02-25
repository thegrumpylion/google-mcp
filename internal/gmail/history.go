package gmail

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
)

// --- list_history ---

type listHistoryInput struct {
	Account        string   `json:"account" jsonschema:"Account name"`
	StartHistoryID uint64   `json:"start_history_id" jsonschema:"Returns history records after this ID. Obtain from get_profile, read_message, or a previous list_history response."`
	HistoryTypes   []string `json:"history_types,omitempty" jsonschema:"Filter by history types: 'messageAdded', 'messageDeleted', 'labelAdded', 'labelRemoved'"`
	LabelID        string   `json:"label_id,omitempty" jsonschema:"Only return messages with this label ID"`
	MaxResults     int64    `json:"max_results,omitempty" jsonschema:"Maximum number of history records (default 100, max 500)"`
}

func registerListHistory(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name: "list_history",
		Description: `List mailbox changes since a given history ID. Use this to track what changed (messages added/deleted, labels added/removed) since a point in time.

Get the starting history ID from get_profile (historyId field) or from a previous list_history response.`,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listHistoryInput) (*mcp.CallToolResult, any, error) {
		if input.StartHistoryID == 0 {
			return nil, nil, fmt.Errorf("start_history_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		maxResults := input.MaxResults
		if maxResults <= 0 {
			maxResults = 100
		}
		if maxResults > 500 {
			maxResults = 500
		}

		call := svc.Users.History.List("me").
			StartHistoryId(input.StartHistoryID).
			MaxResults(maxResults)

		if len(input.HistoryTypes) > 0 {
			call = call.HistoryTypes(input.HistoryTypes...)
		}
		if input.LabelID != "" {
			call = call.LabelId(input.LabelID)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, nil, fmt.Errorf("listing history: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "History ID: %d\n", resp.HistoryId)

		if len(resp.History) == 0 {
			sb.WriteString("No changes since the specified history ID.\n")
		} else {
			fmt.Fprintf(&sb, "Found %d history records:\n\n", len(resp.History))
			for _, h := range resp.History {
				fmt.Fprintf(&sb, "Record ID: %d\n", h.Id)
				for _, ma := range h.MessagesAdded {
					fmt.Fprintf(&sb, "  + Message added: %s (labels: %s)\n",
						ma.Message.Id, strings.Join(ma.Message.LabelIds, ", "))
				}
				for _, md := range h.MessagesDeleted {
					fmt.Fprintf(&sb, "  - Message deleted: %s\n", md.Message.Id)
				}
				for _, la := range h.LabelsAdded {
					fmt.Fprintf(&sb, "  + Labels added to %s: %s\n",
						la.Message.Id, strings.Join(la.LabelIds, ", "))
				}
				for _, lr := range h.LabelsRemoved {
					fmt.Fprintf(&sb, "  - Labels removed from %s: %s\n",
						lr.Message.Id, strings.Join(lr.LabelIds, ", "))
				}
				sb.WriteString("\n")
			}
		}

		if resp.NextPageToken != "" {
			fmt.Fprintf(&sb, "Next page token: %s\n", resp.NextPageToken)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}
