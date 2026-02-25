package calendar

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
	"google.golang.org/api/calendar/v3"
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

// --- create_calendar ---

type createCalendarInput struct {
	Account     string `json:"account" jsonschema:"Account name"`
	Summary     string `json:"summary" jsonschema:"Calendar name/title"`
	Description string `json:"description,omitempty" jsonschema:"Calendar description"`
	TimeZone    string `json:"time_zone,omitempty" jsonschema:"IANA timezone (e.g. 'America/New_York'). Defaults to account timezone."`
	Location    string `json:"location,omitempty" jsonschema:"Geographic location as free-form text"`
}

func registerCreateCalendar(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "create_calendar",
		Description: "Create a new Google Calendar with a given name, description, and timezone.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: server.BoolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input createCalendarInput) (*mcp.CallToolResult, any, error) {
		if input.Summary == "" {
			return nil, nil, fmt.Errorf("summary is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		cal := &calendar.Calendar{
			Summary:     input.Summary,
			Description: input.Description,
			TimeZone:    input.TimeZone,
			Location:    input.Location,
		}

		created, err := svc.Calendars.Insert(cal).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("creating calendar: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Calendar created.\n\n")
		fmt.Fprintf(&sb, "Calendar ID: %s\n", created.Id)
		fmt.Fprintf(&sb, "Name: %s\n", created.Summary)
		if created.Description != "" {
			fmt.Fprintf(&sb, "Description: %s\n", created.Description)
		}
		if created.TimeZone != "" {
			fmt.Fprintf(&sb, "Timezone: %s\n", created.TimeZone)
		}
		if created.Location != "" {
			fmt.Fprintf(&sb, "Location: %s\n", created.Location)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- delete_calendar ---

type deleteCalendarInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id" jsonschema:"Calendar ID to delete"`
}

func registerDeleteCalendar(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "delete_calendar",
		Description: "Delete a secondary calendar. The primary calendar cannot be deleted. This action is permanent.",
		Annotations: &mcp.ToolAnnotations{},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input deleteCalendarInput) (*mcp.CallToolResult, any, error) {
		if input.CalendarID == "" {
			return nil, nil, fmt.Errorf("calendar_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		if err := svc.Calendars.Delete(input.CalendarID).Do(); err != nil {
			return nil, nil, fmt.Errorf("deleting calendar: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Calendar %s deleted.", input.CalendarID)},
			},
		}, nil, nil
	})
}

// TODO: Planned calendar tools (from api-coverage.md):
// - get_calendar (Calendars.Get)
// - update_calendar (Calendars.Update / Calendars.Patch)
// - subscribe_calendar (CalendarList.Insert)
// - unsubscribe_calendar (CalendarList.Delete)
// - update_calendar_list_entry (CalendarList.Update / CalendarList.Patch)
