package gmail

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
)

// --- gmail_get_profile ---

type getProfileInput struct {
	Account string `json:"account" jsonschema:"Account name or 'all' for all accounts"`
}

func registerGetProfile(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "get_profile",
		Description: "Get the authenticated user's Gmail profile. Returns email address, total messages, total threads, and current history ID.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input getProfileInput) (*mcp.CallToolResult, any, error) {
		accounts, err := mgr.ResolveAccounts(input.Account)
		if err != nil {
			return nil, nil, err
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
				return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
			}

			profile, err := svc.Users.GetProfile("me").Do()
			if err != nil {
				if multiAccount {
					fmt.Fprintf(&sb, "=== Account: %s ===\nError getting profile: %v\n\n", account, err)
					continue
				}
				return nil, nil, fmt.Errorf("getting profile: %w", err)
			}

			if multiAccount {
				fmt.Fprintf(&sb, "=== Account: %s ===\n", account)
			}
			fmt.Fprintf(&sb, "Email: %s\nTotal messages: %d\nTotal threads: %d\nHistory ID: %d\n\n",
				profile.EmailAddress, profile.MessagesTotal, profile.ThreadsTotal, profile.HistoryId)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}
