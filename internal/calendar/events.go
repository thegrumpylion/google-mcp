package calendar

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/bridge"
	"github.com/thegrumpylion/google-mcp/internal/server"
	"google.golang.org/api/calendar/v3"
)

// calendarDriveAttachment references a Google Drive file to attach to an event.
// Only metadata (title, mimeType, webViewLink) is resolved — no file bytes are downloaded.
type calendarDriveAttachment struct {
	DriveAccount string `json:"drive_account" jsonschema:"Drive account name"`
	FileID       string `json:"file_id" jsonschema:"Google Drive file ID to attach"`
}

// resolveDriveAttachmentsForEvent resolves Drive file metadata and returns
// Calendar EventAttachment objects. No file content is downloaded.
func resolveDriveAttachmentsForEvent(ctx context.Context, mgr *auth.Manager, attachments []calendarDriveAttachment) ([]*calendar.EventAttachment, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	result := make([]*calendar.EventAttachment, 0, len(attachments))
	for i, da := range attachments {
		if da.DriveAccount == "" {
			return nil, fmt.Errorf("drive_attachments[%d]: drive_account is required", i)
		}
		if da.FileID == "" {
			return nil, fmt.Errorf("drive_attachments[%d]: file_id is required", i)
		}
		meta, err := bridge.GetDriveFileMetadata(ctx, mgr, bridge.GetDriveFileMetadataParams{
			DriveAccount: da.DriveAccount,
			FileID:       da.FileID,
		})
		if err != nil {
			return nil, fmt.Errorf("drive_attachments[%d] (%s): %w", i, da.FileID, err)
		}
		result = append(result, &calendar.EventAttachment{
			FileId:   meta.FileID,
			FileUrl:  meta.WebViewLink,
			MimeType: meta.MIMEType,
			Title:    meta.FileName,
		})
	}
	return result, nil
}

// --- list_events ---

type listEventsInput struct {
	Account    string `json:"account" jsonschema:"Account name or 'all' for all accounts"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
	TimeMin    string `json:"time_min,omitempty" jsonschema:"Start of time range in RFC3339 format (e.g. '2024-01-15T00:00:00Z'). Default: now"`
	TimeMax    string `json:"time_max,omitempty" jsonschema:"End of time range in RFC3339 format. Default: 7 days from now"`
	Query      string `json:"query,omitempty" jsonschema:"Free text search query"`
	MaxResults int64  `json:"max_results,omitempty" jsonschema:"Maximum number of events per account (default 20, max 100)"`
}

func registerListEvents(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
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

// --- get_event ---

type getEventInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
	EventID    string `json:"event_id" jsonschema:"Event ID to retrieve"`
}

func registerGetEvent(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
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

// --- create_event ---

type createEventInput struct {
	Account          string                    `json:"account" jsonschema:"Account name"`
	CalendarID       string                    `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
	Summary          string                    `json:"summary" jsonschema:"Event title"`
	Description      string                    `json:"description,omitempty" jsonschema:"Event description"`
	Location         string                    `json:"location,omitempty" jsonschema:"Event location"`
	StartTime        string                    `json:"start_time" jsonschema:"Event start time in RFC3339 format (e.g. '2024-01-15T09:00:00-05:00') or date for all-day events (e.g. '2024-01-15')"`
	EndTime          string                    `json:"end_time" jsonschema:"Event end time in RFC3339 format or date for all-day events"`
	TimeZone         string                    `json:"time_zone,omitempty" jsonschema:"IANA timezone (e.g. 'America/New_York'). Defaults to account calendar timezone."`
	Attendees        []string                  `json:"attendees,omitempty" jsonschema:"Email addresses of attendees"`
	DriveAttachments []calendarDriveAttachment `json:"drive_attachments,omitempty" jsonschema:"Google Drive files to attach to the event (metadata only, no file download)"`
}

func registerCreateEvent(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name: "create_event",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: server.BoolPtr(false),
		},
		Description: "Create a new event on a Google Calendar. Supports timed and all-day events, with optional attendees, location, and Google Drive file attachments.",
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

		// Resolve Drive attachments.
		if len(input.DriveAttachments) > 0 {
			attachments, err := resolveDriveAttachmentsForEvent(ctx, mgr, input.DriveAttachments)
			if err != nil {
				return nil, nil, err
			}
			event.Attachments = attachments
		}

		call := svc.Events.Insert(calendarID, event)
		if len(event.Attachments) > 0 {
			call = call.SupportsAttachments(true)
		}
		created, err := call.Do()
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

// --- update_event ---

type updateEventInput struct {
	Account          string                    `json:"account" jsonschema:"Account name"`
	CalendarID       string                    `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
	EventID          string                    `json:"event_id" jsonschema:"Event ID to update"`
	Summary          string                    `json:"summary,omitempty" jsonschema:"New event title (leave empty to keep current)"`
	Description      string                    `json:"description,omitempty" jsonschema:"New event description (leave empty to keep current)"`
	Location         string                    `json:"location,omitempty" jsonschema:"New event location (leave empty to keep current)"`
	StartTime        string                    `json:"start_time,omitempty" jsonschema:"New start time in RFC3339 format or date for all-day events (leave empty to keep current)"`
	EndTime          string                    `json:"end_time,omitempty" jsonschema:"New end time in RFC3339 format or date for all-day events (leave empty to keep current)"`
	TimeZone         string                    `json:"time_zone,omitempty" jsonschema:"IANA timezone (e.g. 'America/New_York')"`
	Attendees        []string                  `json:"attendees,omitempty" jsonschema:"Replace attendee list with these email addresses. Omit to keep current attendees."`
	DriveAttachments []calendarDriveAttachment `json:"drive_attachments,omitempty" jsonschema:"Google Drive files to attach to the event (adds to existing attachments). Metadata only, no file download."`
}

func registerUpdateEvent(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name: "update_event",
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
		},
		Description: `Update an existing calendar event. Only specified fields are changed; omitted fields keep their current values.

To update attendees, provide the full list — it replaces the existing attendees.
To change times, provide both start_time and end_time.
To add Drive file attachments, provide drive_attachments — they are appended to any existing attachments.`,
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

		// Add Drive attachments (appended to any existing attachments).
		if len(input.DriveAttachments) > 0 {
			attachments, err := resolveDriveAttachmentsForEvent(ctx, mgr, input.DriveAttachments)
			if err != nil {
				return nil, nil, err
			}
			existing.Attachments = append(existing.Attachments, attachments...)
		}

		call := svc.Events.Update(calendarID, input.EventID, existing)
		if len(existing.Attachments) > 0 {
			call = call.SupportsAttachments(true)
		}
		updated, err := call.Do()
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

// --- delete_event ---

type deleteEventInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
	EventID    string `json:"event_id" jsonschema:"Event ID to delete"`
}

func registerDeleteEvent(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
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

// --- respond_event ---

type respondEventInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
	EventID    string `json:"event_id" jsonschema:"Event ID to respond to"`
	Response   string `json:"response" jsonschema:"Response status: 'accepted', 'declined', or 'tentative'"`
}

func registerRespondEvent(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name: "respond_event",
		Description: `Respond to a calendar event invitation. Sets your attendance status.

Valid responses:
  - "accepted" — Accept the invitation
  - "declined" — Decline the invitation
  - "tentative" — Tentatively accept the invitation`,
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: server.BoolPtr(false),
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

// --- quick_add_event ---

type quickAddEventInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
	Text       string `json:"text" jsonschema:"Natural language event description (e.g. 'Lunch with Bob tomorrow at noon')"`
}

func registerQuickAddEvent(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name: "quick_add_event",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: server.BoolPtr(false),
		},
		Description: "Create a calendar event from a natural language description (e.g. \"Lunch with Bob tomorrow at noon\"). Google parses the text to extract event details.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input quickAddEventInput) (*mcp.CallToolResult, any, error) {
		if input.Text == "" {
			return nil, nil, fmt.Errorf("text is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		calendarID := input.CalendarID
		if calendarID == "" {
			calendarID = "primary"
		}

		created, err := svc.Events.QuickAdd(calendarID, input.Text).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("quick-adding event: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Event created.\n\nEvent ID: %s\nLink: %s\n\n%s",
					created.Id, created.HtmlLink, formatEvent(created, input.Account))},
			},
		}, nil, nil
	})
}

// --- list_event_instances ---

type listEventInstancesInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
	EventID    string `json:"event_id" jsonschema:"Recurring event ID to list instances of"`
	TimeMin    string `json:"time_min,omitempty" jsonschema:"Start of time range in RFC3339 format. Default: now"`
	TimeMax    string `json:"time_max,omitempty" jsonschema:"End of time range in RFC3339 format. Default: 30 days from now"`
	MaxResults int64  `json:"max_results,omitempty" jsonschema:"Maximum number of instances to return (default 25, max 100)"`
}

func registerListEventInstances(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "list_event_instances",
		Description: "List individual occurrences of a recurring calendar event. Use this to see when a repeating event occurs within a time range.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listEventInstancesInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
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
			timeMax = now.Add(30 * 24 * time.Hour).Format(time.RFC3339)
		}

		maxResults := input.MaxResults
		if maxResults <= 0 {
			maxResults = 25
		}
		if maxResults > 100 {
			maxResults = 100
		}

		resp, err := svc.Events.Instances(calendarID, input.EventID).
			TimeMin(timeMin).
			TimeMax(timeMax).
			MaxResults(maxResults).
			Do()
		if err != nil {
			return nil, nil, fmt.Errorf("listing event instances: %w", err)
		}

		var sb strings.Builder
		if len(resp.Items) == 0 {
			sb.WriteString("No instances found in the specified time range.")
		} else {
			fmt.Fprintf(&sb, "Found %d instances:\n\n", len(resp.Items))
			for _, event := range resp.Items {
				sb.WriteString(formatEvent(event, input.Account))
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

// --- move_event ---

type moveEventInput struct {
	Account       string `json:"account" jsonschema:"Account name"`
	CalendarID    string `json:"calendar_id,omitempty" jsonschema:"Source calendar ID (default: 'primary')"`
	EventID       string `json:"event_id" jsonschema:"Event ID to move"`
	DestinationID string `json:"destination_id" jsonschema:"Destination calendar ID"`
}

func registerMoveEvent(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "move_event",
		Description: "Move a calendar event to a different calendar. The event is removed from the source calendar and added to the destination.",
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input moveEventInput) (*mcp.CallToolResult, any, error) {
		if input.DestinationID == "" {
			return nil, nil, fmt.Errorf("destination_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		calendarID := input.CalendarID
		if calendarID == "" {
			calendarID = "primary"
		}

		moved, err := svc.Events.Move(calendarID, input.EventID, input.DestinationID).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("moving event: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Event moved to calendar %s.\n\nEvent ID: %s\nLink: %s\n\n%s",
					input.DestinationID, moved.Id, moved.HtmlLink, formatEvent(moved, input.Account))},
			},
		}, nil, nil
	})
}
