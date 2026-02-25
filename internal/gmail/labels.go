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

// --- list_labels ---

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

// --- get_label ---

type getLabelInput struct {
	Account string `json:"account" jsonschema:"Account name"`
	LabelID string `json:"label_id" jsonschema:"Label ID (from list_labels)"`
}

func registerGetLabel(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "get_label",
		Description: "Get details of a Gmail label including unread and total message/thread counts. Use list_labels to discover label IDs.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input getLabelInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		label, err := svc.Users.Labels.Get("me", input.LabelID).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting label: %w", err)
		}

		text := fmt.Sprintf("Label ID: %s\nName: %s\nType: %s\nMessages total: %d\nMessages unread: %d\nThreads total: %d\nThreads unread: %d\nLabel list visibility: %s\nMessage list visibility: %s",
			label.Id, label.Name, label.Type,
			label.MessagesTotal, label.MessagesUnread,
			label.ThreadsTotal, label.ThreadsUnread,
			label.LabelListVisibility, label.MessageListVisibility)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, nil, nil
	})
}

// --- create_label ---

type createLabelInput struct {
	Account               string `json:"account" jsonschema:"Account name"`
	Name                  string `json:"name" jsonschema:"Label name (use '/' for nested labels, e.g. 'Projects/Work')"`
	LabelListVisibility   string `json:"label_list_visibility,omitempty" jsonschema:"Visibility in label list: labelShow, labelShowIfUnread, or labelHide (default: labelShow)"`
	MessageListVisibility string `json:"message_list_visibility,omitempty" jsonschema:"Visibility in message list: show or hide (default: show)"`
}

func registerCreateLabel(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "create_label",
		Description: "Create a custom Gmail label for organizing email. Use '/' in the name for nested labels (e.g. 'Projects/Work').",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: server.BoolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input createLabelInput) (*mcp.CallToolResult, any, error) {
		if input.Name == "" {
			return nil, nil, fmt.Errorf("name is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		label := &gmailapi.Label{
			Name: input.Name,
		}
		if input.LabelListVisibility != "" {
			label.LabelListVisibility = input.LabelListVisibility
		}
		if input.MessageListVisibility != "" {
			label.MessageListVisibility = input.MessageListVisibility
		}

		created, err := svc.Users.Labels.Create("me", label).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("creating label: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Label created.\n\nLabel ID: %s\nName: %s",
					created.Id, created.Name)},
			},
		}, nil, nil
	})
}

// --- delete_label ---

type deleteLabelInput struct {
	Account string `json:"account" jsonschema:"Account name"`
	LabelID string `json:"label_id" jsonschema:"Label ID to delete (from list_labels). System labels cannot be deleted."`
}

func registerDeleteLabel(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "delete_label",
		Description: "Delete a custom Gmail label. System labels (INBOX, SENT, etc.) cannot be deleted. Messages with this label are not deleted, only the label is removed.",
		Annotations: &mcp.ToolAnnotations{},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input deleteLabelInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		if err := svc.Users.Labels.Delete("me", input.LabelID).Do(); err != nil {
			return nil, nil, fmt.Errorf("deleting label: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Label %s deleted.", input.LabelID)},
			},
		}, nil, nil
	})
}

// TODO: Planned label tools (from api-coverage.md):
// - update_label (Labels.Update / Labels.Patch)
