package calendar

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
)

// --- get_colors ---

type getColorsInput struct {
	Account string `json:"account" jsonschema:"Account name"`
}

func registerGetColors(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "get_colors",
		Description: "Get the available color palette for calendars and events. Returns color IDs with their background and foreground hex values. Use these IDs when setting colors on calendars or events.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input getColorsInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		colors, err := svc.Colors.Get().Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting colors: %w", err)
		}

		var sb strings.Builder

		if len(colors.Calendar) > 0 {
			sb.WriteString("Calendar colors:\n")
			ids := make([]string, 0, len(colors.Calendar))
			for id := range colors.Calendar {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				c := colors.Calendar[id]
				fmt.Fprintf(&sb, "  %s: background=%s foreground=%s\n", id, c.Background, c.Foreground)
			}
		}

		if len(colors.Event) > 0 {
			sb.WriteString("\nEvent colors:\n")
			ids := make([]string, 0, len(colors.Event))
			for id := range colors.Event {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				c := colors.Event[id]
				fmt.Fprintf(&sb, "  %s: background=%s foreground=%s\n", id, c.Background, c.Foreground)
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}
