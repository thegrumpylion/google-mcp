package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
)

// --- list_shared_drives ---

type listSharedDrivesInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	Query      string `json:"query,omitempty" jsonschema:"Search query to filter shared drives (e.g. \"name contains 'Engineering'\")"`
	MaxResults int64  `json:"max_results,omitempty" jsonschema:"Maximum number of results (default 20, max 100)"`
}

func registerListSharedDrives(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_shared_drives",
		Description: "List shared drives the user has access to. Optionally filter by name using a search query.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listSharedDrivesInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		maxResults := input.MaxResults
		if maxResults <= 0 {
			maxResults = 20
		}
		if maxResults > 100 {
			maxResults = 100
		}

		call := svc.Drives.List().
			PageSize(maxResults).
			Fields("drives(id,name,createdTime,hidden,restrictions)")

		if input.Query != "" {
			call = call.Q(input.Query)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, nil, fmt.Errorf("listing shared drives: %w", err)
		}

		if len(resp.Drives) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "No shared drives found."},
				},
			}, nil, nil
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Found %d shared drives:\n\n", len(resp.Drives))
		for _, d := range resp.Drives {
			fmt.Fprintf(&sb, "- Name: %s\n  Drive ID: %s\n", d.Name, d.Id)
			if d.CreatedTime != "" {
				fmt.Fprintf(&sb, "  Created: %s\n", d.CreatedTime)
			}
			if d.Hidden {
				sb.WriteString("  Hidden: true\n")
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

// --- get_shared_drive ---

type getSharedDriveInput struct {
	Account string `json:"account" jsonschema:"Account name"`
	DriveID string `json:"drive_id" jsonschema:"Shared drive ID"`
}

func registerGetSharedDrive(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_shared_drive",
		Description: "Get details of a specific shared drive including name, creation time, and restrictions.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input getSharedDriveInput) (*mcp.CallToolResult, any, error) {
		if input.DriveID == "" {
			return nil, nil, fmt.Errorf("drive_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		d, err := svc.Drives.Get(input.DriveID).
			Fields("id,name,createdTime,hidden,colorRgb,restrictions,capabilities").
			Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting shared drive: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Name: %s\n", d.Name)
		fmt.Fprintf(&sb, "Drive ID: %s\n", d.Id)
		if d.CreatedTime != "" {
			fmt.Fprintf(&sb, "Created: %s\n", d.CreatedTime)
		}
		if d.ColorRgb != "" {
			fmt.Fprintf(&sb, "Color: %s\n", d.ColorRgb)
		}
		if d.Hidden {
			sb.WriteString("Hidden: true\n")
		}

		if d.Restrictions != nil {
			sb.WriteString("\nRestrictions:\n")
			fmt.Fprintf(&sb, "  Domain users only: %v\n", d.Restrictions.DomainUsersOnly)
			fmt.Fprintf(&sb, "  Drive members only: %v\n", d.Restrictions.DriveMembersOnly)
			fmt.Fprintf(&sb, "  Copy requires writer permission: %v\n", d.Restrictions.CopyRequiresWriterPermission)
			fmt.Fprintf(&sb, "  Admin managed: %v\n", d.Restrictions.AdminManagedRestrictions)
		}

		if d.Capabilities != nil {
			sb.WriteString("\nCapabilities:\n")
			fmt.Fprintf(&sb, "  Can add children: %v\n", d.Capabilities.CanAddChildren)
			fmt.Fprintf(&sb, "  Can manage members: %v\n", d.Capabilities.CanManageMembers)
			fmt.Fprintf(&sb, "  Can rename drive: %v\n", d.Capabilities.CanRenameDrive)
			fmt.Fprintf(&sb, "  Can delete drive: %v\n", d.Capabilities.CanDeleteDrive)
			fmt.Fprintf(&sb, "  Can share: %v\n", d.Capabilities.CanShare)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}
