package gmail

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
	gmailapi "google.golang.org/api/gmail/v1"
)

// --- get_vacation ---

type getVacationInput struct {
	Account string `json:"account" jsonschema:"Account name"`
}

func registerGetVacation(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "get_vacation",
		Description: "Get the Gmail vacation/out-of-office auto-reply settings for an account.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input getVacationInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		settings, err := svc.Users.Settings.GetVacation("me").Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting vacation settings: %w", err)
		}

		enabled := "disabled"
		if settings.EnableAutoReply {
			enabled = "enabled"
		}

		text := fmt.Sprintf("Auto-reply: %s\nSubject: %s\nBody:\n%s\nRestrict to contacts: %v\nRestrict to domain: %v",
			enabled, settings.ResponseSubject, settings.ResponseBodyPlainText,
			settings.RestrictToContacts, settings.RestrictToDomain)

		if settings.StartTime > 0 {
			text += fmt.Sprintf("\nStart: %s", time.UnixMilli(settings.StartTime).UTC().Format(time.RFC3339))
		}
		if settings.EndTime > 0 {
			text += fmt.Sprintf("\nEnd: %s", time.UnixMilli(settings.EndTime).UTC().Format(time.RFC3339))
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, nil, nil
	})
}

// --- gmail_update_vacation ---

type updateVacationInput struct {
	Account            string `json:"account" jsonschema:"Account name"`
	EnableAutoReply    *bool  `json:"enable_auto_reply,omitempty" jsonschema:"Enable or disable the auto-reply"`
	ResponseSubject    string `json:"response_subject,omitempty" jsonschema:"Subject line for auto-reply (empty to keep current)"`
	ResponseBody       string `json:"response_body,omitempty" jsonschema:"Plain text body for auto-reply (empty to keep current)"`
	StartTime          string `json:"start_time,omitempty" jsonschema:"Start date/time in RFC3339 format (e.g. 2026-03-01T00:00:00Z)"`
	EndTime            string `json:"end_time,omitempty" jsonschema:"End date/time in RFC3339 format (e.g. 2026-03-15T00:00:00Z)"`
	RestrictToContacts *bool  `json:"restrict_to_contacts,omitempty" jsonschema:"Only send auto-reply to contacts"`
	RestrictToDomain   *bool  `json:"restrict_to_domain,omitempty" jsonschema:"Only send auto-reply to same domain"`
}

func registerUpdateVacation(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "update_vacation",
		Description: "Update Gmail vacation/out-of-office auto-reply settings. Set enable_auto_reply to true/false to toggle. Provide response_subject and response_body for the auto-reply message.",
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input updateVacationInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		// Fetch current settings to merge with updates.
		current, err := svc.Users.Settings.GetVacation("me").Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting current vacation settings: %w", err)
		}

		if input.EnableAutoReply != nil {
			current.EnableAutoReply = *input.EnableAutoReply
		}
		if input.ResponseSubject != "" {
			current.ResponseSubject = input.ResponseSubject
		}
		if input.ResponseBody != "" {
			current.ResponseBodyPlainText = input.ResponseBody
		}
		if input.RestrictToContacts != nil {
			current.RestrictToContacts = *input.RestrictToContacts
		}
		if input.RestrictToDomain != nil {
			current.RestrictToDomain = *input.RestrictToDomain
		}
		if input.StartTime != "" {
			t, err := time.Parse(time.RFC3339, input.StartTime)
			if err != nil {
				return nil, nil, fmt.Errorf("parsing start_time: %w", err)
			}
			current.StartTime = t.UnixMilli()
		}
		if input.EndTime != "" {
			t, err := time.Parse(time.RFC3339, input.EndTime)
			if err != nil {
				return nil, nil, fmt.Errorf("parsing end_time: %w", err)
			}
			current.EndTime = t.UnixMilli()
		}

		updated, err := svc.Users.Settings.UpdateVacation("me", current).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("updating vacation settings: %w", err)
		}

		enabled := "disabled"
		if updated.EnableAutoReply {
			enabled = "enabled"
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Vacation auto-reply updated.\n\nAuto-reply: %s\nSubject: %s",
					enabled, updated.ResponseSubject)},
			},
		}, nil, nil
	})
}

// --- list_filters ---

type listFiltersInput struct {
	Account string `json:"account" jsonschema:"Account name"`
}

func registerListFilters(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "list_filters",
		Description: "List all Gmail filters (inbox rules) for an account. Shows matching criteria and actions for each filter.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listFiltersInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		resp, err := svc.Users.Settings.Filters.List("me").Do()
		if err != nil {
			return nil, nil, fmt.Errorf("listing filters: %w", err)
		}

		var sb strings.Builder
		if len(resp.Filter) == 0 {
			sb.WriteString("No filters found.")
		} else {
			fmt.Fprintf(&sb, "Found %d filters:\n\n", len(resp.Filter))
			for _, f := range resp.Filter {
				fmt.Fprintf(&sb, "- Filter ID: %s\n", f.Id)
				if f.Criteria != nil {
					sb.WriteString("  Criteria:\n")
					if f.Criteria.From != "" {
						fmt.Fprintf(&sb, "    From: %s\n", f.Criteria.From)
					}
					if f.Criteria.To != "" {
						fmt.Fprintf(&sb, "    To: %s\n", f.Criteria.To)
					}
					if f.Criteria.Subject != "" {
						fmt.Fprintf(&sb, "    Subject: %s\n", f.Criteria.Subject)
					}
					if f.Criteria.Query != "" {
						fmt.Fprintf(&sb, "    Query: %s\n", f.Criteria.Query)
					}
					if f.Criteria.NegatedQuery != "" {
						fmt.Fprintf(&sb, "    Negated query: %s\n", f.Criteria.NegatedQuery)
					}
					if f.Criteria.HasAttachment {
						sb.WriteString("    Has attachment: true\n")
					}
					if f.Criteria.ExcludeChats {
						sb.WriteString("    Exclude chats: true\n")
					}
					if f.Criteria.Size > 0 {
						fmt.Fprintf(&sb, "    Size %s: %d bytes\n", f.Criteria.SizeComparison, f.Criteria.Size)
					}
				}
				if f.Action != nil {
					sb.WriteString("  Actions:\n")
					if len(f.Action.AddLabelIds) > 0 {
						fmt.Fprintf(&sb, "    Add labels: %s\n", strings.Join(f.Action.AddLabelIds, ", "))
					}
					if len(f.Action.RemoveLabelIds) > 0 {
						fmt.Fprintf(&sb, "    Remove labels: %s\n", strings.Join(f.Action.RemoveLabelIds, ", "))
					}
					if f.Action.Forward != "" {
						fmt.Fprintf(&sb, "    Forward to: %s\n", f.Action.Forward)
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

// --- create_filter ---

type createFilterInput struct {
	Account       string   `json:"account" jsonschema:"Account name"`
	From          string   `json:"from,omitempty" jsonschema:"Match sender email or name"`
	To            string   `json:"to,omitempty" jsonschema:"Match recipient email or name"`
	Subject       string   `json:"subject,omitempty" jsonschema:"Match subject (case-insensitive)"`
	Query         string   `json:"query,omitempty" jsonschema:"Match using Gmail search query syntax"`
	NegatedQuery  string   `json:"negated_query,omitempty" jsonschema:"Exclude messages matching this query"`
	HasAttachment *bool    `json:"has_attachment,omitempty" jsonschema:"Match messages with attachments"`
	AddLabels     []string `json:"add_labels,omitempty" jsonschema:"Label IDs to add to matching messages"`
	RemoveLabels  []string `json:"remove_labels,omitempty" jsonschema:"Label IDs to remove from matching messages"`
	Forward       string   `json:"forward,omitempty" jsonschema:"Email address to forward matching messages to"`
}

func registerCreateFilter(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name: "create_filter",
		Description: `Create a Gmail filter (inbox rule). Filters automatically apply actions to incoming messages that match the criteria.

At least one criteria field must be set. Common patterns:
  - Auto-label: from="notifications@github.com", add_labels=["Label_123"]
  - Auto-archive: from="noreply@example.com", remove_labels=["INBOX"]
  - Auto-star: query="is:important", add_labels=["STARRED"]

Use list_labels to discover label IDs.`,
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: server.BoolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input createFilterInput) (*mcp.CallToolResult, any, error) {
		// Validate that at least one criteria is set.
		hasCriteria := input.From != "" || input.To != "" || input.Subject != "" ||
			input.Query != "" || input.NegatedQuery != "" || input.HasAttachment != nil
		if !hasCriteria {
			return nil, nil, fmt.Errorf("at least one criteria field is required (from, to, subject, query, negated_query, or has_attachment)")
		}

		// Validate that at least one action is set.
		hasAction := len(input.AddLabels) > 0 || len(input.RemoveLabels) > 0 || input.Forward != ""
		if !hasAction {
			return nil, nil, fmt.Errorf("at least one action is required (add_labels, remove_labels, or forward)")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		filter := &gmailapi.Filter{
			Criteria: &gmailapi.FilterCriteria{
				From:         input.From,
				To:           input.To,
				Subject:      input.Subject,
				Query:        input.Query,
				NegatedQuery: input.NegatedQuery,
			},
			Action: &gmailapi.FilterAction{
				AddLabelIds:    input.AddLabels,
				RemoveLabelIds: input.RemoveLabels,
				Forward:        input.Forward,
			},
		}
		if input.HasAttachment != nil {
			filter.Criteria.HasAttachment = *input.HasAttachment
		}

		created, err := svc.Users.Settings.Filters.Create("me", filter).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("creating filter: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Filter created.\n\nFilter ID: %s", created.Id)},
			},
		}, nil, nil
	})
}

// --- delete_filter ---

type deleteFilterInput struct {
	Account  string `json:"account" jsonschema:"Account name"`
	FilterID string `json:"filter_id" jsonschema:"Filter ID to delete (from list_filters)"`
}

func registerDeleteFilter(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "delete_filter",
		Description: "Delete a Gmail filter (inbox rule) by ID. Use list_filters to discover filter IDs.",
		Annotations: &mcp.ToolAnnotations{},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input deleteFilterInput) (*mcp.CallToolResult, any, error) {
		if input.FilterID == "" {
			return nil, nil, fmt.Errorf("filter_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		if err := svc.Users.Settings.Filters.Delete("me", input.FilterID).Do(); err != nil {
			return nil, nil, fmt.Errorf("deleting filter: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Filter %s deleted.", input.FilterID)},
			},
		}, nil, nil
	})
}

// --- list_send_as ---

type listSendAsInput struct {
	Account string `json:"account" jsonschema:"Account name"`
}

func registerListSendAs(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "list_send_as",
		Description: "List send-as aliases for a Gmail account. Shows all email addresses the account can send from, including the primary address and any configured aliases.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listSendAsInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		resp, err := svc.Users.Settings.SendAs.List("me").Do()
		if err != nil {
			return nil, nil, fmt.Errorf("listing send-as aliases: %w", err)
		}

		var sb strings.Builder
		if len(resp.SendAs) == 0 {
			sb.WriteString("No send-as aliases found.")
		} else {
			fmt.Fprintf(&sb, "Found %d send-as aliases:\n\n", len(resp.SendAs))
			for _, sa := range resp.SendAs {
				fmt.Fprintf(&sb, "- %s\n", sa.SendAsEmail)
				if sa.DisplayName != "" {
					fmt.Fprintf(&sb, "  Display name: %s\n", sa.DisplayName)
				}
				if sa.ReplyToAddress != "" {
					fmt.Fprintf(&sb, "  Reply-to: %s\n", sa.ReplyToAddress)
				}
				if sa.IsPrimary {
					sb.WriteString("  (Primary)\n")
				}
				if sa.IsDefault {
					sb.WriteString("  (Default)\n")
				}
				if sa.VerificationStatus != "" {
					fmt.Fprintf(&sb, "  Verification: %s\n", sa.VerificationStatus)
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
