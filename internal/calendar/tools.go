// Package calendar provides MCP tools for interacting with the Google Calendar API.
package calendar

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
	"google.golang.org/api/calendar/v3"
	driveapi "google.golang.org/api/drive/v3"
)

// Scopes required by the Calendar tools.
// CalendarScope is the full-access scope, required for ACL operations and calendar CRUD.
// DriveScope is required for resolving Drive file metadata when attaching files to events.
var Scopes = []string{
	calendar.CalendarScope,
	driveapi.DriveScope,
}

// RegisterTools registers all Calendar MCP tools on the given server.
func RegisterTools(srv *server.Server, mgr *auth.Manager) {
	server.RegisterAccountsListTool(srv, mgr)
	server.RegisterLocalFSTools(srv)
	// calendars.go
	registerListCalendars(srv, mgr)
	registerGetCalendar(srv, mgr)
	registerCreateCalendar(srv, mgr)
	registerUpdateCalendar(srv, mgr)
	registerDeleteCalendar(srv, mgr)
	registerGetCalendarListEntry(srv, mgr)
	registerSubscribeCalendar(srv, mgr)
	registerUnsubscribeCalendar(srv, mgr)
	registerUpdateCalendarListEntry(srv, mgr)
	// events.go
	registerListEvents(srv, mgr)
	registerGetEvent(srv, mgr)
	registerCreateEvent(srv, mgr)
	registerUpdateEvent(srv, mgr)
	registerDeleteEvent(srv, mgr)
	registerRespondEvent(srv, mgr)
	registerQuickAddEvent(srv, mgr)
	registerListEventInstances(srv, mgr)
	registerMoveEvent(srv, mgr)
	// freebusy.go
	registerQueryFreeBusy(srv, mgr)
	// acl.go
	registerShareCalendar(srv, mgr)
	registerListCalendarSharing(srv, mgr)
	registerGetACLRule(srv, mgr)
	registerUpdateACLRule(srv, mgr)
	registerDeleteACLRule(srv, mgr)
	// colors.go
	registerGetColors(srv, mgr)
}

func newService(ctx context.Context, mgr *auth.Manager, account string) (*calendar.Service, error) {
	opt, err := mgr.ClientOption(ctx, account, Scopes)
	if err != nil {
		return nil, err
	}
	return calendar.NewService(ctx, opt)
}

// isDateOnly checks if a time string is a date-only format (YYYY-MM-DD).
func isDateOnly(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// formatEvent formats an event for brief display.
func formatEvent(event *calendar.Event, account string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "- %s\n", event.Summary)
	fmt.Fprintf(&sb, "  Event ID: %s\n", event.Id)
	fmt.Fprintf(&sb, "  Account: %s\n", account)

	if event.Start != nil {
		if event.Start.DateTime != "" {
			fmt.Fprintf(&sb, "  Start: %s\n", event.Start.DateTime)
		} else if event.Start.Date != "" {
			fmt.Fprintf(&sb, "  Start: %s (all day)\n", event.Start.Date)
		}
	}
	if event.End != nil {
		if event.End.DateTime != "" {
			fmt.Fprintf(&sb, "  End: %s\n", event.End.DateTime)
		} else if event.End.Date != "" {
			fmt.Fprintf(&sb, "  End: %s\n", event.End.Date)
		}
	}

	if event.Location != "" {
		fmt.Fprintf(&sb, "  Location: %s\n", event.Location)
	}
	if event.Status != "" {
		fmt.Fprintf(&sb, "  Status: %s\n", event.Status)
	}

	return sb.String()
}

// formatEventDetailed formats an event with full details.
func formatEventDetailed(event *calendar.Event) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Event: %s\n", event.Summary)
	fmt.Fprintf(&sb, "Event ID: %s\n", event.Id)

	if event.Start != nil {
		if event.Start.DateTime != "" {
			fmt.Fprintf(&sb, "Start: %s\n", event.Start.DateTime)
		} else if event.Start.Date != "" {
			fmt.Fprintf(&sb, "Start: %s (all day)\n", event.Start.Date)
		}
	}
	if event.End != nil {
		if event.End.DateTime != "" {
			fmt.Fprintf(&sb, "End: %s\n", event.End.DateTime)
		} else if event.End.Date != "" {
			fmt.Fprintf(&sb, "End: %s\n", event.End.Date)
		}
	}

	if event.Location != "" {
		fmt.Fprintf(&sb, "Location: %s\n", event.Location)
	}
	if event.Description != "" {
		fmt.Fprintf(&sb, "Description: %s\n", event.Description)
	}
	if event.Status != "" {
		fmt.Fprintf(&sb, "Status: %s\n", event.Status)
	}
	if event.HtmlLink != "" {
		fmt.Fprintf(&sb, "Link: %s\n", event.HtmlLink)
	}
	if event.Creator != nil {
		fmt.Fprintf(&sb, "Creator: %s\n", event.Creator.Email)
	}
	if event.Organizer != nil {
		fmt.Fprintf(&sb, "Organizer: %s\n", event.Organizer.Email)
	}
	if len(event.Attendees) > 0 {
		sb.WriteString("Attendees:\n")
		for _, a := range event.Attendees {
			name := a.DisplayName
			if name == "" {
				name = a.Email
			}
			fmt.Fprintf(&sb, "  - %s (%s)\n", name, a.ResponseStatus)
		}
	}
	if len(event.Recurrence) > 0 {
		fmt.Fprintf(&sb, "Recurrence: %s\n", strings.Join(event.Recurrence, "; "))
	}
	if len(event.Attachments) > 0 {
		sb.WriteString("Attachments:\n")
		for _, att := range event.Attachments {
			title := att.Title
			if title == "" {
				title = att.FileUrl
			}
			fmt.Fprintf(&sb, "  - %s", title)
			if att.MimeType != "" {
				fmt.Fprintf(&sb, " (%s)", att.MimeType)
			}
			if att.FileUrl != "" {
				fmt.Fprintf(&sb, "\n    URL: %s", att.FileUrl)
			}
			sb.WriteString("\n")
		}
	}
	if event.ConferenceData != nil && len(event.ConferenceData.EntryPoints) > 0 {
		sb.WriteString("Conference:\n")
		for _, ep := range event.ConferenceData.EntryPoints {
			fmt.Fprintf(&sb, "  - %s: %s\n", ep.EntryPointType, ep.Uri)
		}
	}

	return sb.String()
}

// AccountScopes returns the scopes used by Calendar tools.
func AccountScopes() []string {
	return Scopes
}
