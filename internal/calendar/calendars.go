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

// --- get_calendar ---

type getCalendarInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
}

func registerGetCalendar(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "get_calendar",
		Description: "Get details of a specific calendar including name, description, timezone, and location.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input getCalendarInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		calendarID := input.CalendarID
		if calendarID == "" {
			calendarID = "primary"
		}

		cal, err := svc.Calendars.Get(calendarID).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting calendar: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Calendar ID: %s\n", cal.Id)
		fmt.Fprintf(&sb, "Name: %s\n", cal.Summary)
		if cal.Description != "" {
			fmt.Fprintf(&sb, "Description: %s\n", cal.Description)
		}
		if cal.TimeZone != "" {
			fmt.Fprintf(&sb, "Timezone: %s\n", cal.TimeZone)
		}
		if cal.Location != "" {
			fmt.Fprintf(&sb, "Location: %s\n", cal.Location)
		}
		if cal.Etag != "" {
			fmt.Fprintf(&sb, "ETag: %s\n", cal.Etag)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- update_calendar ---

type updateCalendarInput struct {
	Account     string `json:"account" jsonschema:"Account name"`
	CalendarID  string `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
	Summary     string `json:"summary,omitempty" jsonschema:"New calendar name (leave empty to keep current)"`
	Description string `json:"description,omitempty" jsonschema:"New calendar description (leave empty to keep current)"`
	TimeZone    string `json:"time_zone,omitempty" jsonschema:"New IANA timezone (leave empty to keep current)"`
	Location    string `json:"location,omitempty" jsonschema:"New location (leave empty to keep current)"`
}

func registerUpdateCalendar(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "update_calendar",
		Description: "Update a calendar's name, description, timezone, or location. Only specified fields are changed.",
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input updateCalendarInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		calendarID := input.CalendarID
		if calendarID == "" {
			calendarID = "primary"
		}

		// Fetch current calendar to merge updates.
		cal, err := svc.Calendars.Get(calendarID).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting calendar: %w", err)
		}

		if input.Summary != "" {
			cal.Summary = input.Summary
		}
		if input.Description != "" {
			cal.Description = input.Description
		}
		if input.TimeZone != "" {
			cal.TimeZone = input.TimeZone
		}
		if input.Location != "" {
			cal.Location = input.Location
		}

		updated, err := svc.Calendars.Update(calendarID, cal).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("updating calendar: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Calendar updated.\n\n")
		fmt.Fprintf(&sb, "Calendar ID: %s\n", updated.Id)
		fmt.Fprintf(&sb, "Name: %s\n", updated.Summary)
		if updated.Description != "" {
			fmt.Fprintf(&sb, "Description: %s\n", updated.Description)
		}
		if updated.TimeZone != "" {
			fmt.Fprintf(&sb, "Timezone: %s\n", updated.TimeZone)
		}
		if updated.Location != "" {
			fmt.Fprintf(&sb, "Location: %s\n", updated.Location)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- clear_calendar ---

type clearCalendarInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id" jsonschema:"Calendar ID to clear"`
}

func registerClearCalendar(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "clear_calendar",
		Description: "Remove all events from a calendar. The calendar itself is not deleted. This action is permanent and cannot be undone.",
		Annotations: &mcp.ToolAnnotations{},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input clearCalendarInput) (*mcp.CallToolResult, any, error) {
		if input.CalendarID == "" {
			return nil, nil, fmt.Errorf("calendar_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		if err := svc.Calendars.Clear(input.CalendarID).Do(); err != nil {
			return nil, nil, fmt.Errorf("clearing calendar: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("All events cleared from calendar %s.", input.CalendarID)},
			},
		}, nil, nil
	})
}

// --- get_calendar_list_entry ---

type getCalendarListEntryInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id" jsonschema:"Calendar ID to get details for"`
}

func registerGetCalendarListEntry(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "get_calendar_list_entry",
		Description: "Get detailed info about a specific calendar in the user's calendar list, including color, notifications, access role, and visibility settings.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input getCalendarListEntryInput) (*mcp.CallToolResult, any, error) {
		if input.CalendarID == "" {
			return nil, nil, fmt.Errorf("calendar_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		entry, err := svc.CalendarList.Get(input.CalendarID).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting calendar list entry: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Calendar ID: %s\n", entry.Id)
		fmt.Fprintf(&sb, "Name: %s\n", entry.Summary)
		if entry.SummaryOverride != "" {
			fmt.Fprintf(&sb, "Display name: %s\n", entry.SummaryOverride)
		}
		if entry.Description != "" {
			fmt.Fprintf(&sb, "Description: %s\n", entry.Description)
		}
		fmt.Fprintf(&sb, "Access role: %s\n", entry.AccessRole)
		if entry.TimeZone != "" {
			fmt.Fprintf(&sb, "Timezone: %s\n", entry.TimeZone)
		}
		if entry.Location != "" {
			fmt.Fprintf(&sb, "Location: %s\n", entry.Location)
		}
		if entry.BackgroundColor != "" {
			fmt.Fprintf(&sb, "Background color: %s\n", entry.BackgroundColor)
		}
		if entry.ForegroundColor != "" {
			fmt.Fprintf(&sb, "Foreground color: %s\n", entry.ForegroundColor)
		}
		if entry.ColorId != "" {
			fmt.Fprintf(&sb, "Color ID: %s\n", entry.ColorId)
		}
		if entry.Hidden {
			sb.WriteString("Hidden: true\n")
		}
		if entry.Selected {
			sb.WriteString("Selected: true\n")
		}
		if entry.Primary {
			sb.WriteString("Primary: true\n")
		}
		if entry.Deleted {
			sb.WriteString("Deleted: true\n")
		}
		if len(entry.DefaultReminders) > 0 {
			sb.WriteString("Default reminders:\n")
			for _, r := range entry.DefaultReminders {
				fmt.Fprintf(&sb, "  - %s: %d minutes\n", r.Method, r.Minutes)
			}
		}
		if len(entry.NotificationSettings.Notifications) > 0 {
			sb.WriteString("Notifications:\n")
			for _, n := range entry.NotificationSettings.Notifications {
				fmt.Fprintf(&sb, "  - %s: %s\n", n.Type, n.Method)
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- subscribe_calendar ---

type subscribeCalendarInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id" jsonschema:"Calendar ID to subscribe to (e.g. a public calendar or one shared with you)"`
}

func registerSubscribeCalendar(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "subscribe_calendar",
		Description: "Subscribe to an existing calendar by adding it to the user's calendar list. Use this for public calendars or calendars that have been shared with you.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: server.BoolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input subscribeCalendarInput) (*mcp.CallToolResult, any, error) {
		if input.CalendarID == "" {
			return nil, nil, fmt.Errorf("calendar_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		entry, err := svc.CalendarList.Insert(&calendar.CalendarListEntry{
			Id: input.CalendarID,
		}).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("subscribing to calendar: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Subscribed to calendar.\n\n")
		fmt.Fprintf(&sb, "Calendar ID: %s\n", entry.Id)
		fmt.Fprintf(&sb, "Name: %s\n", entry.Summary)
		fmt.Fprintf(&sb, "Access role: %s\n", entry.AccessRole)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- unsubscribe_calendar ---

type unsubscribeCalendarInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id" jsonschema:"Calendar ID to unsubscribe from"`
}

func registerUnsubscribeCalendar(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "unsubscribe_calendar",
		Description: "Unsubscribe from a calendar by removing it from the user's calendar list. The calendar itself is not deleted — only removed from your list.",
		Annotations: &mcp.ToolAnnotations{},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input unsubscribeCalendarInput) (*mcp.CallToolResult, any, error) {
		if input.CalendarID == "" {
			return nil, nil, fmt.Errorf("calendar_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		if err := svc.CalendarList.Delete(input.CalendarID).Do(); err != nil {
			return nil, nil, fmt.Errorf("unsubscribing from calendar: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Unsubscribed from calendar %s.", input.CalendarID)},
			},
		}, nil, nil
	})
}

// --- update_calendar_list_entry ---

type updateCalendarListEntryInput struct {
	Account         string `json:"account" jsonschema:"Account name"`
	CalendarID      string `json:"calendar_id" jsonschema:"Calendar ID to update"`
	SummaryOverride string `json:"summary_override,omitempty" jsonschema:"Custom display name for the calendar (leave empty to keep current)"`
	ColorID         string `json:"color_id,omitempty" jsonschema:"Color ID from get_colors (leave empty to keep current)"`
	Hidden          *bool  `json:"hidden,omitempty" jsonschema:"Hide this calendar in the list"`
	Selected        *bool  `json:"selected,omitempty" jsonschema:"Show this calendar's events in the UI"`
}

func registerUpdateCalendarListEntry(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "update_calendar_list_entry",
		Description: "Update a calendar's display settings in the user's calendar list: custom name, color, visibility, and selection state. Only specified fields are changed.",
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input updateCalendarListEntryInput) (*mcp.CallToolResult, any, error) {
		if input.CalendarID == "" {
			return nil, nil, fmt.Errorf("calendar_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		// Fetch current entry to merge updates.
		entry, err := svc.CalendarList.Get(input.CalendarID).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting calendar list entry: %w", err)
		}

		if input.SummaryOverride != "" {
			entry.SummaryOverride = input.SummaryOverride
		}
		if input.ColorID != "" {
			entry.ColorId = input.ColorID
		}
		if input.Hidden != nil {
			entry.Hidden = *input.Hidden
		}
		if input.Selected != nil {
			entry.Selected = *input.Selected
		}

		updated, err := svc.CalendarList.Update(input.CalendarID, entry).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("updating calendar list entry: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Calendar list entry updated.\n\n")
		fmt.Fprintf(&sb, "Calendar ID: %s\n", updated.Id)
		fmt.Fprintf(&sb, "Name: %s\n", updated.Summary)
		if updated.SummaryOverride != "" {
			fmt.Fprintf(&sb, "Display name: %s\n", updated.SummaryOverride)
		}
		if updated.ColorId != "" {
			fmt.Fprintf(&sb, "Color ID: %s\n", updated.ColorId)
		}
		if updated.BackgroundColor != "" {
			fmt.Fprintf(&sb, "Background color: %s\n", updated.BackgroundColor)
		}
		fmt.Fprintf(&sb, "Hidden: %v\n", updated.Hidden)
		fmt.Fprintf(&sb, "Selected: %v\n", updated.Selected)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}
