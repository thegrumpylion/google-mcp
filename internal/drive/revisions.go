package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
)

// --- list_revisions ---

type listRevisionsInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	FileID     string `json:"file_id" jsonschema:"Google Drive file ID"`
	MaxResults int64  `json:"max_results,omitempty" jsonschema:"Maximum number of revisions to return (default 20, max 100)"`
}

func registerListRevisions(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "list_revisions",
		Description: "List revisions (version history) of a Google Drive file. Shows revision IDs, modification times, and authors.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listRevisionsInput) (*mcp.CallToolResult, any, error) {
		if input.FileID == "" {
			return nil, nil, fmt.Errorf("file_id is required")
		}

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

		resp, err := svc.Revisions.List(input.FileID).
			PageSize(maxResults).
			Fields("revisions(id,mimeType,modifiedTime,size,lastModifyingUser,keepForever,publishAuto,published,publishedOutsideDomain)").
			Do()
		if err != nil {
			return nil, nil, fmt.Errorf("listing revisions: %w", err)
		}

		if len(resp.Revisions) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "No revisions found."},
				},
			}, nil, nil
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Found %d revisions:\n\n", len(resp.Revisions))
		for _, r := range resp.Revisions {
			fmt.Fprintf(&sb, "- Revision ID: %s\n", r.Id)
			if r.ModifiedTime != "" {
				fmt.Fprintf(&sb, "  Modified: %s\n", r.ModifiedTime)
			}
			if r.LastModifyingUser != nil {
				if r.LastModifyingUser.DisplayName != "" {
					fmt.Fprintf(&sb, "  Modified by: %s\n", r.LastModifyingUser.DisplayName)
				}
				if r.LastModifyingUser.EmailAddress != "" {
					fmt.Fprintf(&sb, "  Email: %s\n", r.LastModifyingUser.EmailAddress)
				}
			}
			if r.MimeType != "" {
				fmt.Fprintf(&sb, "  MIME type: %s\n", r.MimeType)
			}
			if r.Size > 0 {
				fmt.Fprintf(&sb, "  Size: %s\n", formatBytes(r.Size))
			}
			if r.KeepForever {
				sb.WriteString("  Keep forever: true\n")
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

// --- get_revision ---

type getRevisionInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	FileID     string `json:"file_id" jsonschema:"Google Drive file ID"`
	RevisionID string `json:"revision_id" jsonschema:"Revision ID to retrieve"`
}

func registerGetRevision(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "get_revision",
		Description: "Get details of a specific file revision including modification time, author, size, and publishing status.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input getRevisionInput) (*mcp.CallToolResult, any, error) {
		if input.FileID == "" {
			return nil, nil, fmt.Errorf("file_id is required")
		}
		if input.RevisionID == "" {
			return nil, nil, fmt.Errorf("revision_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		r, err := svc.Revisions.Get(input.FileID, input.RevisionID).
			Fields("id,mimeType,modifiedTime,size,lastModifyingUser,keepForever,publishAuto,published,publishedOutsideDomain,originalFilename,md5Checksum,exportLinks").
			Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting revision: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Revision ID: %s\n", r.Id)
		if r.ModifiedTime != "" {
			fmt.Fprintf(&sb, "Modified: %s\n", r.ModifiedTime)
		}
		if r.LastModifyingUser != nil {
			if r.LastModifyingUser.DisplayName != "" {
				fmt.Fprintf(&sb, "Modified by: %s\n", r.LastModifyingUser.DisplayName)
			}
			if r.LastModifyingUser.EmailAddress != "" {
				fmt.Fprintf(&sb, "Email: %s\n", r.LastModifyingUser.EmailAddress)
			}
		}
		if r.MimeType != "" {
			fmt.Fprintf(&sb, "MIME type: %s\n", r.MimeType)
		}
		if r.Size > 0 {
			fmt.Fprintf(&sb, "Size: %s\n", formatBytes(r.Size))
		}
		if r.OriginalFilename != "" {
			fmt.Fprintf(&sb, "Original filename: %s\n", r.OriginalFilename)
		}
		if r.Md5Checksum != "" {
			fmt.Fprintf(&sb, "MD5: %s\n", r.Md5Checksum)
		}
		fmt.Fprintf(&sb, "Keep forever: %v\n", r.KeepForever)
		if r.Published {
			sb.WriteString("Published: true\n")
			fmt.Fprintf(&sb, "Publish auto: %v\n", r.PublishAuto)
			fmt.Fprintf(&sb, "Published outside domain: %v\n", r.PublishedOutsideDomain)
		}
		if len(r.ExportLinks) > 0 {
			sb.WriteString("\nExport links:\n")
			for mime, link := range r.ExportLinks {
				fmt.Fprintf(&sb, "  %s: %s\n", mime, link)
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- delete_revision ---

type deleteRevisionInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	FileID     string `json:"file_id" jsonschema:"Google Drive file ID"`
	RevisionID string `json:"revision_id" jsonschema:"Revision ID to delete"`
}

func registerDeleteRevision(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "delete_revision",
		Description: "Delete a specific revision of a Google Drive file. The last remaining revision cannot be deleted.",
		Annotations: &mcp.ToolAnnotations{},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input deleteRevisionInput) (*mcp.CallToolResult, any, error) {
		if input.FileID == "" {
			return nil, nil, fmt.Errorf("file_id is required")
		}
		if input.RevisionID == "" {
			return nil, nil, fmt.Errorf("revision_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		if err := svc.Revisions.Delete(input.FileID, input.RevisionID).Do(); err != nil {
			return nil, nil, fmt.Errorf("deleting revision: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Revision %s deleted from file %s.", input.RevisionID, input.FileID)},
			},
		}, nil, nil
	})
}
