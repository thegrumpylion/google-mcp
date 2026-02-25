package drive

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
)

// --- empty_trash ---

type emptyTrashInput struct {
	Account string `json:"account" jsonschema:"Account name"`
}

func registerEmptyTrash(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "empty_trash",
		Description: "Permanently delete all files in the Google Drive trash. This action cannot be undone.",
		Annotations: &mcp.ToolAnnotations{},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input emptyTrashInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		if err := svc.Files.EmptyTrash().Do(); err != nil {
			return nil, nil, fmt.Errorf("emptying trash: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Trash emptied. All trashed files have been permanently deleted."},
			},
		}, nil, nil
	})
}
