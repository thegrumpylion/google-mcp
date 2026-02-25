package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
	"google.golang.org/api/drive/v3"
)

// --- list_permissions ---

type listPermissionsInput struct {
	Account string `json:"account" jsonschema:"Account name"`
	FileID  string `json:"file_id" jsonschema:"Google Drive file or folder ID"`
}

func registerListPermissions(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "list_permissions",
		Description: "List all permissions (sharing settings) for a Google Drive file or folder. Shows who has access and their role.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listPermissionsInput) (*mcp.CallToolResult, any, error) {
		if input.FileID == "" {
			return nil, nil, fmt.Errorf("file_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		resp, err := svc.Permissions.List(input.FileID).
			SupportsAllDrives(true).
			Fields("permissions(id,role,type,emailAddress,domain,displayName,expirationTime,deleted)").
			Do()
		if err != nil {
			return nil, nil, fmt.Errorf("listing permissions: %w", err)
		}

		if len(resp.Permissions) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "No permissions found."},
				},
			}, nil, nil
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Found %d permissions:\n\n", len(resp.Permissions))
		for _, p := range resp.Permissions {
			sb.WriteString(formatPermission(p))
			sb.WriteString("\n")
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- get_permission ---

type getPermissionInput struct {
	Account      string `json:"account" jsonschema:"Account name"`
	FileID       string `json:"file_id" jsonschema:"Google Drive file or folder ID"`
	PermissionID string `json:"permission_id" jsonschema:"Permission ID to inspect"`
}

func registerGetPermission(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "get_permission",
		Description: "Get details of a specific permission on a Google Drive file or folder.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input getPermissionInput) (*mcp.CallToolResult, any, error) {
		if input.FileID == "" {
			return nil, nil, fmt.Errorf("file_id is required")
		}
		if input.PermissionID == "" {
			return nil, nil, fmt.Errorf("permission_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		perm, err := svc.Permissions.Get(input.FileID, input.PermissionID).
			SupportsAllDrives(true).
			Fields("id,role,type,emailAddress,domain,displayName,expirationTime,deleted,permissionDetails").
			Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting permission: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: formatPermission(perm)},
			},
		}, nil, nil
	})
}

// --- update_permission ---

type updatePermissionInput struct {
	Account      string `json:"account" jsonschema:"Account name"`
	FileID       string `json:"file_id" jsonschema:"Google Drive file or folder ID"`
	PermissionID string `json:"permission_id" jsonschema:"Permission ID to update"`
	Role         string `json:"role" jsonschema:"New role: 'reader', 'commenter', 'writer', or 'organizer'"`
}

func registerUpdatePermission(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name: "update_permission",
		Description: `Update a permission on a Google Drive file or folder. Use this to change the access level (role) for an existing permission.

Roles:
  - "reader" — View only
  - "commenter" — View and comment
  - "writer" — Edit
  - "organizer" — Manage (shared drives only)`,
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input updatePermissionInput) (*mcp.CallToolResult, any, error) {
		if input.FileID == "" {
			return nil, nil, fmt.Errorf("file_id is required")
		}
		if input.PermissionID == "" {
			return nil, nil, fmt.Errorf("permission_id is required")
		}

		switch input.Role {
		case "reader", "commenter", "writer", "organizer":
		default:
			return nil, nil, fmt.Errorf("invalid role %q: must be 'reader', 'commenter', 'writer', or 'organizer'", input.Role)
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		perm := &drive.Permission{
			Role: input.Role,
		}

		updated, err := svc.Permissions.Update(input.FileID, input.PermissionID, perm).
			SupportsAllDrives(true).
			Fields("id,role,type,emailAddress,domain,displayName").
			Do()
		if err != nil {
			return nil, nil, fmt.Errorf("updating permission: %w", err)
		}

		var sb strings.Builder
		sb.WriteString("Permission updated.\n\n")
		sb.WriteString(formatPermission(updated))

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- delete_permission ---

type deletePermissionInput struct {
	Account      string `json:"account" jsonschema:"Account name"`
	FileID       string `json:"file_id" jsonschema:"Google Drive file or folder ID"`
	PermissionID string `json:"permission_id" jsonschema:"Permission ID to delete (revoke access)"`
}

func registerDeletePermission(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "delete_permission",
		Description: "Delete a permission from a Google Drive file or folder, revoking access for that user, group, or domain.",
		Annotations: &mcp.ToolAnnotations{},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input deletePermissionInput) (*mcp.CallToolResult, any, error) {
		if input.FileID == "" {
			return nil, nil, fmt.Errorf("file_id is required")
		}
		if input.PermissionID == "" {
			return nil, nil, fmt.Errorf("permission_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		if err := svc.Permissions.Delete(input.FileID, input.PermissionID).
			SupportsAllDrives(true).
			Do(); err != nil {
			return nil, nil, fmt.Errorf("deleting permission: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Permission %s deleted from file %s.", input.PermissionID, input.FileID)},
			},
		}, nil, nil
	})
}

// formatPermission formats a single permission for display.
func formatPermission(p *drive.Permission) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "- Permission ID: %s\n", p.Id)
	fmt.Fprintf(&sb, "  Role: %s\n", p.Role)
	fmt.Fprintf(&sb, "  Type: %s\n", p.Type)
	if p.DisplayName != "" {
		fmt.Fprintf(&sb, "  Name: %s\n", p.DisplayName)
	}
	if p.EmailAddress != "" {
		fmt.Fprintf(&sb, "  Email: %s\n", p.EmailAddress)
	}
	if p.Domain != "" {
		fmt.Fprintf(&sb, "  Domain: %s\n", p.Domain)
	}
	if p.ExpirationTime != "" {
		fmt.Fprintf(&sb, "  Expires: %s\n", p.ExpirationTime)
	}
	if p.Deleted {
		sb.WriteString("  Status: Deleted\n")
	}
	return sb.String()
}
