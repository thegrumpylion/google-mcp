package gmail

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
	gmailapi "google.golang.org/api/gmail/v1"
)

// --- gmail_draft_create ---

type draftCreateInput struct {
	Account string `json:"account" jsonschema:"Account name"`
	composeInput
	ReplyToMessageID string `json:"reply_to_message_id,omitempty" jsonschema:"Message ID to reply to (sets In-Reply-To and References headers, keeps thread)"`
}

func registerDraftCreate(srv *server.Server, mgr *auth.Manager) {
	desc := "Create a Gmail draft. The draft is saved but not sent. Use send_draft to send it later, or list_drafts to see all drafts." + srv.ReadDirsDescription()

	server.AddTool(srv, &mcp.Tool{
		Name: "create_draft",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: server.BoolPtr(false),
		},
		Description: desc,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input draftCreateInput) (*mcp.CallToolResult, any, error) {
		if len(input.LocalAttachments) > 0 {
			lfs := srv.LocalFS()
			if lfs == nil {
				return nil, nil, fmt.Errorf("local file access is not enabled (use --allow-read-dir)")
			}
			if err := resolveLocalAttachments(lfs, &input.composeInput); err != nil {
				return nil, nil, err
			}
		}

		if err := resolveDriveAttachments(ctx, mgr, &input.composeInput); err != nil {
			return nil, nil, err
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		result, err := buildMessage(svc, input.composeInput, input.ReplyToMessageID)
		if err != nil {
			return nil, nil, err
		}

		draft := &gmailapi.Draft{
			Message: &gmailapi.Message{
				Raw:      result.Raw,
				ThreadId: result.ThreadID,
			},
		}

		created, err := svc.Users.Drafts.Create("me", draft).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("creating draft: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Draft created.\n\nDraft ID: %s\nMessage ID: %s",
					created.Id, created.Message.Id)},
			},
		}, nil, nil
	})
}

// --- gmail_draft_list ---

type draftListInput struct {
	Account    string `json:"account" jsonschema:"Account name or 'all' for all accounts"`
	MaxResults int64  `json:"max_results,omitempty" jsonschema:"Maximum number of drafts per account (default 20, max 100)"`
}

func registerDraftList(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "list_drafts",
		Description: "List Gmail drafts. Set account to 'all' to list from all accounts. Returns draft IDs and message snippets.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input draftListInput) (*mcp.CallToolResult, any, error) {
		accounts, err := mgr.ResolveAccounts(input.Account)
		if err != nil {
			return nil, nil, err
		}

		maxResults := input.MaxResults
		if maxResults <= 0 {
			maxResults = 20
		}
		if maxResults > 100 {
			maxResults = 100
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

			resp, err := svc.Users.Drafts.List("me").MaxResults(maxResults).Do()
			if err != nil {
				if multiAccount {
					fmt.Fprintf(&sb, "=== Account: %s ===\nError listing drafts: %v\n\n", account, err)
					continue
				}
				return nil, nil, fmt.Errorf("listing drafts: %w", err)
			}

			if multiAccount {
				fmt.Fprintf(&sb, "=== Account: %s ===\n", account)
			}

			if len(resp.Drafts) == 0 {
				sb.WriteString("No drafts found.\n\n")
				continue
			}

			fmt.Fprintf(&sb, "Found %d drafts:\n\n", len(resp.Drafts))
			for _, draft := range resp.Drafts {
				fmt.Fprintf(&sb, "- Draft ID: %s\n  Message ID: %s\n  Account: %s\n",
					draft.Id, draft.Message.Id, account)

				// Fetch snippet for the draft message.
				detail, err := svc.Users.Messages.Get("me", draft.Message.Id).Format("metadata").MetadataHeaders("To", "Subject").Do()
				if err == nil {
					headers := make(map[string]string)
					if detail.Payload != nil {
						for _, h := range detail.Payload.Headers {
							headers[h.Name] = h.Value
						}
					}
					if to := headers["To"]; to != "" {
						fmt.Fprintf(&sb, "  To: %s\n", to)
					}
					if subj := headers["Subject"]; subj != "" {
						fmt.Fprintf(&sb, "  Subject: %s\n", subj)
					}
					if detail.Snippet != "" {
						fmt.Fprintf(&sb, "  Snippet: %s\n", detail.Snippet)
					}
				}
				sb.WriteString("\n")
			}
		}

		text := sb.String()
		if text == "" {
			text = "No drafts found."
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, nil, nil
	})
}

// --- gmail_draft_get ---

type draftGetInput struct {
	Account string `json:"account" jsonschema:"Account name"`
	DraftID string `json:"draft_id" jsonschema:"Draft ID to read (from draft_list or draft_create)"`
}

func registerDraftGet(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "get_draft",
		Description: "Read the full content of a Gmail draft by ID. Returns headers, body text, and draft metadata.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input draftGetInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		draft, err := svc.Users.Drafts.Get("me", input.DraftID).Format("full").Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting draft: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Draft ID: %s\n", draft.Id)
		fmt.Fprintf(&sb, "Message ID: %s\n", draft.Message.Id)

		if draft.Message.Payload != nil {
			for _, h := range draft.Message.Payload.Headers {
				switch h.Name {
				case "From", "To", "Cc", "Bcc", "Subject", "Date":
					fmt.Fprintf(&sb, "%s: %s\n", h.Name, h.Value)
				}
			}
		}
		sb.WriteString("\n")

		body := extractBody(draft.Message.Payload)
		if body != "" {
			sb.WriteString(body)
		} else {
			sb.WriteString("(no text content)")
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- gmail_draft_update ---

type draftUpdateInput struct {
	Account string `json:"account" jsonschema:"Account name"`
	DraftID string `json:"draft_id" jsonschema:"Draft ID to update (from draft_list or draft_create)"`
	composeInput
}

func registerDraftUpdate(srv *server.Server, mgr *auth.Manager) {
	desc := "Update an existing Gmail draft with new content. Replaces the draft message entirely." + srv.ReadDirsDescription()

	server.AddTool(srv, &mcp.Tool{
		Name: "update_draft",
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
		},
		Description: desc,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input draftUpdateInput) (*mcp.CallToolResult, any, error) {
		if len(input.LocalAttachments) > 0 {
			lfs := srv.LocalFS()
			if lfs == nil {
				return nil, nil, fmt.Errorf("local file access is not enabled (use --allow-read-dir)")
			}
			if err := resolveLocalAttachments(lfs, &input.composeInput); err != nil {
				return nil, nil, err
			}
		}

		if err := resolveDriveAttachments(ctx, mgr, &input.composeInput); err != nil {
			return nil, nil, err
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		result, err := buildMessage(svc, input.composeInput, "")
		if err != nil {
			return nil, nil, err
		}

		draft := &gmailapi.Draft{
			Message: &gmailapi.Message{
				Raw: result.Raw,
			},
		}

		updated, err := svc.Users.Drafts.Update("me", input.DraftID, draft).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("updating draft: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Draft updated.\n\nDraft ID: %s\nMessage ID: %s",
					updated.Id, updated.Message.Id)},
			},
		}, nil, nil
	})
}

// --- gmail_draft_delete ---

type draftDeleteInput struct {
	Account string `json:"account" jsonschema:"Account name"`
	DraftID string `json:"draft_id" jsonschema:"Draft ID to delete (from draft_list or draft_create)"`
}

func registerDraftDelete(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "delete_draft",
		Annotations: &mcp.ToolAnnotations{},
		Description: "Delete a Gmail draft permanently. The draft message is removed and cannot be recovered.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input draftDeleteInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		if err := svc.Users.Drafts.Delete("me", input.DraftID).Do(); err != nil {
			return nil, nil, fmt.Errorf("deleting draft: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Draft %s deleted.", input.DraftID)},
			},
		}, nil, nil
	})
}

// --- gmail_draft_send ---

type draftSendInput struct {
	Account string `json:"account" jsonschema:"Account name"`
	DraftID string `json:"draft_id" jsonschema:"Draft ID to send (from draft_list or draft_create)"`
}

func registerDraftSend(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "send_draft",
		Annotations: &mcp.ToolAnnotations{},
		Description: "Send an existing Gmail draft. The draft is removed after sending.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input draftSendInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		sent, err := svc.Users.Drafts.Send("me", &gmailapi.Draft{Id: input.DraftID}).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("sending draft: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Draft sent.\n\nMessage ID: %s\nThread ID: %s",
					sent.Id, sent.ThreadId)},
			},
		}, nil, nil
	})
}
