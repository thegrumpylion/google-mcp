package calendar

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
)

// --- list_calendars ---

type listCalendarsInput struct {
	Account string `json:"account" jsonschema:"Account name or 'all' for all accounts"`
}

func registerListCalendars(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "list_calendars",
		Description: "List all calendars accessible by the account. Set account to 'all' to list calendars from all accounts. Returns calendar IDs and names.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listCalendarsInput) (*mcp.CallToolResult, any, error) {
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
				return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
			}

			resp, err := svc.CalendarList.List().Do()
			if err != nil {
				if multiAccount {
					fmt.Fprintf(&sb, "=== Account: %s ===\nError listing calendars: %v\n\n", account, err)
					continue
				}
				return nil, nil, fmt.Errorf("listing calendars: %w", err)
			}

			if multiAccount {
				fmt.Fprintf(&sb, "=== Account: %s ===\n", account)
			}

			fmt.Fprintf(&sb, "Found %d calendars:\n\n", len(resp.Items))
			for _, cal := range resp.Items {
				fmt.Fprintf(&sb, "- %s\n  Calendar ID: %s\n  Account: %s\n  Access: %s\n", cal.Summary, cal.Id, account, cal.AccessRole)
				if cal.Description != "" {
					fmt.Fprintf(&sb, "  Description: %s\n", cal.Description)
				}
				if cal.Primary {
					sb.WriteString("  (Primary)\n")
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

// TODO: Planned calendar tools (from api-coverage.md):
// - create_calendar (Calendars.Insert)
// - delete_calendar (Calendars.Delete)
// - get_calendar (Calendars.Get)
// - update_calendar (Calendars.Update / Calendars.Patch)
// - subscribe_calendar (CalendarList.Insert)
// - unsubscribe_calendar (CalendarList.Delete)
// - update_calendar_list_entry (CalendarList.Update / CalendarList.Patch)
