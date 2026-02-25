package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
)

// --- list_changes ---

type listChangesInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	PageToken  string `json:"page_token" jsonschema:"Start page token from get_about or a previous list_changes response. Use 'start' to get the initial token."`
	MaxResults int64  `json:"max_results,omitempty" jsonschema:"Maximum number of changes to return (default 50, max 100)"`
}

func registerListChanges(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name: "list_changes",
		Description: `List changes to files and shared drives in Google Drive since a given page token.

Use page_token="start" to get the initial start token (returns a token without changes).
Then use the returned next_page_token or new_start_page_token in subsequent calls to track changes.`,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listChangesInput) (*mcp.CallToolResult, any, error) {
		if input.PageToken == "" {
			return nil, nil, fmt.Errorf("page_token is required (use 'start' to get the initial token)")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		// Special case: "start" fetches the initial start page token.
		if input.PageToken == "start" {
			resp, err := svc.Changes.GetStartPageToken().Do()
			if err != nil {
				return nil, nil, fmt.Errorf("getting start page token: %w", err)
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Start page token: %s\n\nUse this token in subsequent list_changes calls to track changes from this point forward.", resp.StartPageToken)},
				},
			}, nil, nil
		}

		maxResults := input.MaxResults
		if maxResults <= 0 {
			maxResults = 50
		}
		if maxResults > 100 {
			maxResults = 100
		}

		resp, err := svc.Changes.List(input.PageToken).
			PageSize(maxResults).
			Fields("nextPageToken,newStartPageToken,changes(changeType,removed,fileId,file(id,name,mimeType,trashed,modifiedTime,lastModifyingUser),driveId,drive(id,name))").
			IncludeItemsFromAllDrives(true).
			SupportsAllDrives(true).
			Do()
		if err != nil {
			return nil, nil, fmt.Errorf("listing changes: %w", err)
		}

		var sb strings.Builder
		if len(resp.Changes) == 0 {
			sb.WriteString("No changes found.\n")
		} else {
			fmt.Fprintf(&sb, "Found %d changes:\n\n", len(resp.Changes))
			for _, c := range resp.Changes {
				switch c.ChangeType {
				case "file":
					if c.Removed {
						fmt.Fprintf(&sb, "- REMOVED file: %s\n", c.FileId)
					} else if c.File != nil {
						action := "Modified"
						if c.File.Trashed {
							action = "Trashed"
						}
						fmt.Fprintf(&sb, "- %s: %s\n", action, c.File.Name)
						fmt.Fprintf(&sb, "  File ID: %s\n", c.File.Id)
						if c.File.MimeType != "" {
							fmt.Fprintf(&sb, "  Type: %s\n", c.File.MimeType)
						}
						if c.File.ModifiedTime != "" {
							fmt.Fprintf(&sb, "  Modified: %s\n", c.File.ModifiedTime)
						}
						if c.File.LastModifyingUser != nil && c.File.LastModifyingUser.DisplayName != "" {
							fmt.Fprintf(&sb, "  By: %s\n", c.File.LastModifyingUser.DisplayName)
						}
					}
				case "drive":
					if c.Removed {
						fmt.Fprintf(&sb, "- REMOVED drive: %s\n", c.DriveId)
					} else if c.Drive != nil {
						fmt.Fprintf(&sb, "- Drive changed: %s\n", c.Drive.Name)
						fmt.Fprintf(&sb, "  Drive ID: %s\n", c.Drive.Id)
					}
				default:
					fmt.Fprintf(&sb, "- Change type: %s (file: %s)\n", c.ChangeType, c.FileId)
				}
				sb.WriteString("\n")
			}
		}

		if resp.NewStartPageToken != "" {
			fmt.Fprintf(&sb, "New start page token: %s\n(No more changes. Use this token for future polling.)\n", resp.NewStartPageToken)
		} else if resp.NextPageToken != "" {
			fmt.Fprintf(&sb, "Next page token: %s\n(More changes available. Call again with this token.)\n", resp.NextPageToken)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}
