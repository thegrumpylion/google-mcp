package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
)

// --- get_about ---

type getAboutInput struct {
	Account string `json:"account" jsonschema:"Account name"`
}

func registerGetAbout(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_about",
		Description: "Get Google Drive account information including storage quota, user details, and supported export formats.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input getAboutInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		about, err := svc.About.Get().
			Fields("user,storageQuota,maxUploadSize,exportFormats,importFormats").
			Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting about info: %w", err)
		}

		var sb strings.Builder

		// User info.
		if about.User != nil {
			sb.WriteString("User:\n")
			if about.User.DisplayName != "" {
				fmt.Fprintf(&sb, "  Name: %s\n", about.User.DisplayName)
			}
			if about.User.EmailAddress != "" {
				fmt.Fprintf(&sb, "  Email: %s\n", about.User.EmailAddress)
			}
		}

		// Storage quota.
		if about.StorageQuota != nil {
			sb.WriteString("\nStorage Quota:\n")
			if about.StorageQuota.Limit > 0 {
				fmt.Fprintf(&sb, "  Limit: %s\n", formatBytes(about.StorageQuota.Limit))
			} else {
				sb.WriteString("  Limit: Unlimited\n")
			}
			fmt.Fprintf(&sb, "  Usage: %s\n", formatBytes(about.StorageQuota.Usage))
			fmt.Fprintf(&sb, "  Usage in Drive: %s\n", formatBytes(about.StorageQuota.UsageInDrive))
			fmt.Fprintf(&sb, "  Usage in Trash: %s\n", formatBytes(about.StorageQuota.UsageInDriveTrash))
		}

		if about.MaxUploadSize > 0 {
			fmt.Fprintf(&sb, "\nMax Upload Size: %s\n", formatBytes(about.MaxUploadSize))
		}

		// Export formats (Google Workspace → downloadable).
		if len(about.ExportFormats) > 0 {
			sb.WriteString("\nExport Formats:\n")
			for src, targets := range about.ExportFormats {
				fmt.Fprintf(&sb, "  %s →\n", src)
				for _, t := range targets {
					fmt.Fprintf(&sb, "    - %s\n", t)
				}
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// formatBytes formats byte counts into human-readable form.
func formatBytes(b int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
		tb = gb * 1024
	)
	switch {
	case b >= tb:
		return fmt.Sprintf("%.2f TB (%d bytes)", float64(b)/float64(tb), b)
	case b >= gb:
		return fmt.Sprintf("%.2f GB (%d bytes)", float64(b)/float64(gb), b)
	case b >= mb:
		return fmt.Sprintf("%.2f MB (%d bytes)", float64(b)/float64(mb), b)
	case b >= kb:
		return fmt.Sprintf("%.2f KB (%d bytes)", float64(b)/float64(kb), b)
	default:
		return fmt.Sprintf("%d bytes", b)
	}
}
