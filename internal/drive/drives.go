package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
	driveapi "google.golang.org/api/drive/v3"
)

// --- list_shared_drives ---

type listSharedDrivesInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	Query      string `json:"query,omitempty" jsonschema:"Search query to filter shared drives (e.g. \"name contains 'Engineering'\")"`
	MaxResults int64  `json:"max_results,omitempty" jsonschema:"Maximum number of results (default 20, max 100)"`
}

func registerListSharedDrives(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
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

func registerGetSharedDrive(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
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

// --- create_shared_drive ---

type createSharedDriveInput struct {
	Account string `json:"account" jsonschema:"Account name"`
	Name    string `json:"name" jsonschema:"Name for the new shared drive"`
}

func registerCreateSharedDrive(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "create_shared_drive",
		Description: "Create a new shared drive for team collaboration.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: server.BoolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input createSharedDriveInput) (*mcp.CallToolResult, any, error) {
		if input.Name == "" {
			return nil, nil, fmt.Errorf("name is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		// The API requires a unique requestId for idempotency.
		requestID := uuid.New().String()

		d, err := svc.Drives.Create(requestID, &driveapi.Drive{
			Name: input.Name,
		}).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("creating shared drive: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Shared drive created.\n\nDrive ID: %s\nName: %s", d.Id, d.Name)},
			},
		}, nil, nil
	})
}

// --- update_shared_drive ---

type updateSharedDriveInput struct {
	Account string `json:"account" jsonschema:"Account name"`
	DriveID string `json:"drive_id" jsonschema:"Shared drive ID to update"`
	Name    string `json:"name,omitempty" jsonschema:"New name for the shared drive (leave empty to keep current)"`
}

func registerUpdateSharedDrive(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "update_shared_drive",
		Description: "Update a shared drive's name or settings.",
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input updateSharedDriveInput) (*mcp.CallToolResult, any, error) {
		if input.DriveID == "" {
			return nil, nil, fmt.Errorf("drive_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		update := &driveapi.Drive{}
		if input.Name != "" {
			update.Name = input.Name
		}

		d, err := svc.Drives.Update(input.DriveID, update).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("updating shared drive: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Shared drive updated.\n\nDrive ID: %s\nName: %s", d.Id, d.Name)},
			},
		}, nil, nil
	})
}

// --- delete_shared_drive ---

type deleteSharedDriveInput struct {
	Account string `json:"account" jsonschema:"Account name"`
	DriveID string `json:"drive_id" jsonschema:"Shared drive ID to delete"`
}

func registerDeleteSharedDrive(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "delete_shared_drive",
		Description: "Delete a shared drive. The shared drive must be empty (no files or folders) before it can be deleted. This action is permanent.",
		Annotations: &mcp.ToolAnnotations{},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input deleteSharedDriveInput) (*mcp.CallToolResult, any, error) {
		if input.DriveID == "" {
			return nil, nil, fmt.Errorf("drive_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		if err := svc.Drives.Delete(input.DriveID).Do(); err != nil {
			return nil, nil, fmt.Errorf("deleting shared drive: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Shared drive %s deleted.", input.DriveID)},
			},
		}, nil, nil
	})
}
