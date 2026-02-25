// Package calendar provides MCP tools for interacting with the Google Calendar API.
package calendar

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"google.golang.org/api/calendar/v3"
)

// Scopes required by the Calendar tools.
var Scopes = []string{
	calendar.CalendarReadonlyScope,
	calendar.CalendarEventsScope,
}

// RegisterTools registers all Calendar MCP tools on the given server.
func RegisterTools(server *mcp.Server, mgr *auth.Manager) {
	auth.RegisterAccountsListTool(server, mgr)
	registerListCalendars(server, mgr)
	registerListEvents(server, mgr)
	registerGetEvent(server, mgr)
	registerCreateEvent(server, mgr)
	registerUpdateEvent(server, mgr)
	registerDeleteEvent(server, mgr)
	registerRespondEvent(server, mgr)
}

func newService(ctx context.Context, mgr *auth.Manager, account string) (*calendar.Service, error) {
	opt, err := mgr.ClientOption(ctx, account, Scopes)
	if err != nil {
		return nil, err
	}
	return calendar.NewService(ctx, opt)
}

// --- calendar_list_calendars ---

type listCalendarsInput struct {
	Account string `json:"account" jsonschema:"Account name or 'all' for all accounts"`
}

func registerListCalendars(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
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

// --- calendar_list_events ---

type listEventsInput struct {
	Account    string `json:"account" jsonschema:"Account name or 'all' for all accounts"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
	TimeMin    string `json:"time_min,omitempty" jsonschema:"Start of time range in RFC3339 format (e.g. '2024-01-15T00:00:00Z'). Default: now"`
	TimeMax    string `json:"time_max,omitempty" jsonschema:"End of time range in RFC3339 format. Default: 7 days from now"`
	Query      string `json:"query,omitempty" jsonschema:"Free text search query"`
	MaxResults int64  `json:"max_results,omitempty" jsonschema:"Maximum number of events per account (default 20, max 100)"`
}

func registerListEvents(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_events",
		Description: "List events from a Google Calendar within a time range. Set account to 'all' to list events from all accounts. Defaults to upcoming events in the next 7 days.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listEventsInput) (*mcp.CallToolResult, any, error) {
		accounts, err := mgr.ResolveAccounts(input.Account)
		if err != nil {
			return nil, nil, err
		}

		calendarID := input.CalendarID
		if calendarID == "" {
			calendarID = "primary"
		}

		now := time.Now()
		timeMin := input.TimeMin
		if timeMin == "" {
			timeMin = now.Format(time.RFC3339)
		}
		timeMax := input.TimeMax
		if timeMax == "" {
			timeMax = now.Add(7 * 24 * time.Hour).Format(time.RFC3339)
		}

		maxResults := input.MaxResults
		if maxResults <= 0 {
			maxResults = 20
		}
		if maxResults > 100 {
			maxResults = 100
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

			call := svc.Events.List(calendarID).
				TimeMin(timeMin).
				TimeMax(timeMax).
				MaxResults(maxResults).
				SingleEvents(true).
				OrderBy("startTime")

			if input.Query != "" {
				call = call.Q(input.Query)
			}

			resp, err := call.Do()
			if err != nil {
				if multiAccount {
					fmt.Fprintf(&sb, "=== Account: %s ===\nError listing events: %v\n\n", account, err)
					continue
				}
				return nil, nil, fmt.Errorf("listing events: %w", err)
			}

			if multiAccount {
				fmt.Fprintf(&sb, "=== Account: %s ===\n", account)
			}

			if len(resp.Items) == 0 {
				sb.WriteString("No events found in the specified time range.\n\n")
				continue
			}

			fmt.Fprintf(&sb, "Found %d events:\n\n", len(resp.Items))
			for _, event := range resp.Items {
				sb.WriteString(formatEvent(event, account))
				sb.WriteString("\n")
			}
		}

		text := sb.String()
		if text == "" {
			text = "No events found in the specified time range."
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, nil, nil
	})
}

// --- calendar_get_event ---

type getEventInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
	EventID    string `json:"event_id" jsonschema:"Event ID to retrieve"`
}

func registerGetEvent(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_event",
		Description: "Get full details of a specific calendar event by ID.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input getEventInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		calendarID := input.CalendarID
		if calendarID == "" {
			calendarID = "primary"
		}

		event, err := svc.Events.Get(calendarID, input.EventID).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting event: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: formatEventDetailed(event)},
			},
		}, nil, nil
	})
}

// --- calendar_create_event ---

type createEventInput struct {
	Account     string   `json:"account" jsonschema:"Account name"`
	CalendarID  string   `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
	Summary     string   `json:"summary" jsonschema:"Event title"`
	Description string   `json:"description,omitempty" jsonschema:"Event description"`
	Location    string   `json:"location,omitempty" jsonschema:"Event location"`
	StartTime   string   `json:"start_time" jsonschema:"Event start time in RFC3339 format (e.g. '2024-01-15T09:00:00-05:00') or date for all-day events (e.g. '2024-01-15')"`
	EndTime     string   `json:"end_time" jsonschema:"Event end time in RFC3339 format or date for all-day events"`
	TimeZone    string   `json:"time_zone,omitempty" jsonschema:"IANA timezone (e.g. 'America/New_York'). Defaults to account calendar timezone."`
	Attendees   []string `json:"attendees,omitempty" jsonschema:"Email addresses of attendees"`
}

func registerCreateEvent(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "create_event",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: auth.BoolPtr(false),
		},
		Description: "Create a new event on a Google Calendar. Supports timed and all-day events, with optional attendees and location.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input createEventInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		calendarID := input.CalendarID
		if calendarID == "" {
			calendarID = "primary"
		}

		event := &calendar.Event{
			Summary:     input.Summary,
			Description: input.Description,
			Location:    input.Location,
		}

		// Determine if this is an all-day event (date only) or timed event.
		if isDateOnly(input.StartTime) {
			event.Start = &calendar.EventDateTime{Date: input.StartTime}
			event.End = &calendar.EventDateTime{Date: input.EndTime}
		} else {
			event.Start = &calendar.EventDateTime{
				DateTime: input.StartTime,
				TimeZone: input.TimeZone,
			}
			event.End = &calendar.EventDateTime{
				DateTime: input.EndTime,
				TimeZone: input.TimeZone,
			}
		}

		// Add attendees.
		for _, email := range input.Attendees {
			event.Attendees = append(event.Attendees, &calendar.EventAttendee{
				Email: email,
			})
		}

		created, err := svc.Events.Insert(calendarID, event).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("creating event: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Event created.\n\nEvent ID: %s\nLink: %s\n\n%s",
					created.Id, created.HtmlLink, formatEvent(created, input.Account))},
			},
		}, nil, nil
	})
}

// --- calendar_update_event ---

type updateEventInput struct {
	Account     string   `json:"account" jsonschema:"Account name"`
	CalendarID  string   `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
	EventID     string   `json:"event_id" jsonschema:"Event ID to update"`
	Summary     string   `json:"summary,omitempty" jsonschema:"New event title (leave empty to keep current)"`
	Description string   `json:"description,omitempty" jsonschema:"New event description (leave empty to keep current)"`
	Location    string   `json:"location,omitempty" jsonschema:"New event location (leave empty to keep current)"`
	StartTime   string   `json:"start_time,omitempty" jsonschema:"New start time in RFC3339 format or date for all-day events (leave empty to keep current)"`
	EndTime     string   `json:"end_time,omitempty" jsonschema:"New end time in RFC3339 format or date for all-day events (leave empty to keep current)"`
	TimeZone    string   `json:"time_zone,omitempty" jsonschema:"IANA timezone (e.g. 'America/New_York')"`
	Attendees   []string `json:"attendees,omitempty" jsonschema:"Replace attendee list with these email addresses. Omit to keep current attendees."`
}

func registerUpdateEvent(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "update_event",
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
		},
		Description: `Update an existing calendar event. Only specified fields are changed; omitted fields keep their current values.

To update attendees, provide the full list — it replaces the existing attendees.
To change times, provide both start_time and end_time.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input updateEventInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		calendarID := input.CalendarID
		if calendarID == "" {
			calendarID = "primary"
		}

		// Fetch the existing event so we can apply partial updates.
		existing, err := svc.Events.Get(calendarID, input.EventID).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting event: %w", err)
		}

		if input.Summary != "" {
			existing.Summary = input.Summary
		}
		if input.Description != "" {
			existing.Description = input.Description
		}
		if input.Location != "" {
			existing.Location = input.Location
		}

		// Update times if provided.
		if input.StartTime != "" {
			if isDateOnly(input.StartTime) {
				existing.Start = &calendar.EventDateTime{Date: input.StartTime}
			} else {
				existing.Start = &calendar.EventDateTime{
					DateTime: input.StartTime,
					TimeZone: input.TimeZone,
				}
			}
		}
		if input.EndTime != "" {
			if isDateOnly(input.EndTime) {
				existing.End = &calendar.EventDateTime{Date: input.EndTime}
			} else {
				existing.End = &calendar.EventDateTime{
					DateTime: input.EndTime,
					TimeZone: input.TimeZone,
				}
			}
		}

		// Replace attendees if provided.
		if input.Attendees != nil {
			existing.Attendees = nil
			for _, email := range input.Attendees {
				existing.Attendees = append(existing.Attendees, &calendar.EventAttendee{
					Email: email,
				})
			}
		}

		updated, err := svc.Events.Update(calendarID, input.EventID, existing).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("updating event: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Event updated.\n\nEvent ID: %s\nLink: %s\n\n%s",
					updated.Id, updated.HtmlLink, formatEvent(updated, input.Account))},
			},
		}, nil, nil
	})
}

// --- calendar_delete_event ---

type deleteEventInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
	EventID    string `json:"event_id" jsonschema:"Event ID to delete"`
}

func registerDeleteEvent(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_event",
		Annotations: &mcp.ToolAnnotations{},
		Description: "Delete a calendar event by ID. The event is kept in trash for 30 days before permanent removal.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input deleteEventInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		calendarID := input.CalendarID
		if calendarID == "" {
			calendarID = "primary"
		}

		if err := svc.Events.Delete(calendarID, input.EventID).Do(); err != nil {
			return nil, nil, fmt.Errorf("deleting event: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Event %s deleted.", input.EventID)},
			},
		}, nil, nil
	})
}

// --- calendar_respond_event ---

type respondEventInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
	EventID    string `json:"event_id" jsonschema:"Event ID to respond to"`
	Response   string `json:"response" jsonschema:"Response status: 'accepted', 'declined', or 'tentative'"`
}

func registerRespondEvent(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "respond_event",
		Description: `Respond to a calendar event invitation. Sets your attendance status.

Valid responses:
  - "accepted" — Accept the invitation
  - "declined" — Decline the invitation
  - "tentative" — Tentatively accept the invitation`,
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: auth.BoolPtr(false),
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input respondEventInput) (*mcp.CallToolResult, any, error) {
		// Validate response value.
		switch input.Response {
		case "accepted", "declined", "tentative":
		default:
			return nil, nil, fmt.Errorf("invalid response %q: must be 'accepted', 'declined', or 'tentative'", input.Response)
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		calendarID := input.CalendarID
		if calendarID == "" {
			calendarID = "primary"
		}

		// Fetch the event to find our attendee entry.
		event, err := svc.Events.Get(calendarID, input.EventID).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting event: %w", err)
		}

		// Find the attendee entry for the authenticated user (Self: true).
		found := false
		for _, a := range event.Attendees {
			if a.Self {
				a.ResponseStatus = input.Response
				found = true
				break
			}
		}

		if !found {
			return nil, nil, fmt.Errorf("you are not listed as an attendee of this event")
		}

		updated, err := svc.Events.Patch(calendarID, input.EventID, &calendar.Event{
			Attendees: event.Attendees,
		}).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("updating response: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Response updated to %q for event %q.\n\n%s",
					input.Response, updated.Summary, formatEvent(updated, input.Account))},
			},
		}, nil, nil
	})
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
