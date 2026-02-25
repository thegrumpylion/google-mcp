package calendar

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
	"google.golang.org/api/calendar/v3"
)

// --- query_free_busy ---

type queryFreeBusyInput struct {
	Account   string   `json:"account" jsonschema:"Account name"`
	Calendars []string `json:"calendars" jsonschema:"Calendar IDs or email addresses to check availability for"`
	TimeMin   string   `json:"time_min" jsonschema:"Start of time range in RFC3339 format (e.g. '2024-01-15T00:00:00Z')"`
	TimeMax   string   `json:"time_max" jsonschema:"End of time range in RFC3339 format"`
	TimeZone  string   `json:"time_zone,omitempty" jsonschema:"IANA timezone for the response (default: UTC)"`
}

func registerQueryFreeBusy(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "query_free_busy",
		Description: "Check availability (free/busy) for one or more users or calendars within a time range. Useful for finding open slots before scheduling meetings.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input queryFreeBusyInput) (*mcp.CallToolResult, any, error) {
		if len(input.Calendars) == 0 {
			return nil, nil, fmt.Errorf("calendars is required: provide at least one calendar ID or email address")
		}
		if input.TimeMin == "" {
			return nil, nil, fmt.Errorf("time_min is required")
		}
		if input.TimeMax == "" {
			return nil, nil, fmt.Errorf("time_max is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		items := make([]*calendar.FreeBusyRequestItem, len(input.Calendars))
		for i, cal := range input.Calendars {
			items[i] = &calendar.FreeBusyRequestItem{Id: cal}
		}

		fbReq := &calendar.FreeBusyRequest{
			TimeMin:  input.TimeMin,
			TimeMax:  input.TimeMax,
			TimeZone: input.TimeZone,
			Items:    items,
		}

		resp, err := svc.Freebusy.Query(fbReq).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("querying free/busy: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Free/Busy results (%s to %s):\n\n", resp.TimeMin, resp.TimeMax)

		// Sort calendar IDs for deterministic output.
		calIDs := make([]string, 0, len(resp.Calendars))
		for id := range resp.Calendars {
			calIDs = append(calIDs, id)
		}
		sort.Strings(calIDs)

		for _, calID := range calIDs {
			fbCal := resp.Calendars[calID]
			fmt.Fprintf(&sb, "Calendar: %s\n", calID)

			if len(fbCal.Errors) > 0 {
				for _, e := range fbCal.Errors {
					fmt.Fprintf(&sb, "  Error: %s - %s\n", e.Domain, e.Reason)
				}
				sb.WriteString("\n")
				continue
			}

			if len(fbCal.Busy) == 0 {
				sb.WriteString("  Status: Free (no busy periods)\n\n")
				continue
			}

			fmt.Fprintf(&sb, "  Busy periods (%d):\n", len(fbCal.Busy))
			for _, period := range fbCal.Busy {
				start := period.Start
				end := period.End
				// Try to compute duration for readability.
				if ts, err := time.Parse(time.RFC3339, start); err == nil {
					if te, err := time.Parse(time.RFC3339, end); err == nil {
						dur := te.Sub(ts)
						fmt.Fprintf(&sb, "  - %s to %s (%s)\n", start, end, formatDuration(dur))
						continue
					}
				}
				fmt.Fprintf(&sb, "  - %s to %s\n", start, end)
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

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60

	if hours == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%dm", hours, minutes)
}
