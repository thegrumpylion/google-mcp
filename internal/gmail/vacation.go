package gmail

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
)

// --- gmail_get_vacation ---

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
