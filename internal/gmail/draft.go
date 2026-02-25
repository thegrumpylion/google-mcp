package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	gmailapi "google.golang.org/api/gmail/v1"
)

// --- gmail_draft_create ---

type draftCreateInput struct {
	Account string `json:"account" jsonschema:"required,description=Account name to use"`
	To      string `json:"to" jsonschema:"required,description=Recipient email address"`
	Subject string `json:"subject" jsonschema:"required,description=Email subject line"`
	Body    string `json:"body" jsonschema:"required,description=Email body (plain text)"`
	Cc      string `json:"cc,omitempty" jsonschema:"description=CC recipients (comma-separated email addresses)"`
	Bcc     string `json:"bcc,omitempty" jsonschema:"description=BCC recipients (comma-separated email addresses)"`
	ReplyTo string `json:"reply_to,omitempty" jsonschema:"description=Message ID to reply to (sets In-Reply-To and References headers)"`
}

func registerDraftCreate(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gmail_draft_create",
		Description: "Create a Gmail draft. The draft is saved but not sent. Use gmail_draft_send to send it later, or gmail_draft_list to see all drafts.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input draftCreateInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		// Build the RFC 2822 message.
		var raw strings.Builder
		fmt.Fprintf(&raw, "To: %s\r\n", input.To)
		if input.Cc != "" {
			fmt.Fprintf(&raw, "Cc: %s\r\n", input.Cc)
		}
		if input.Bcc != "" {
			fmt.Fprintf(&raw, "Bcc: %s\r\n", input.Bcc)
		}
		fmt.Fprintf(&raw, "Subject: %s\r\n", mime2047Encode(input.Subject))
		fmt.Fprintf(&raw, "Content-Type: text/plain; charset=\"UTF-8\"\r\n")
		if input.ReplyTo != "" {
			origMsg, err := svc.Users.Messages.Get("me", input.ReplyTo).Format("metadata").MetadataHeaders("Message-Id").Do()
			if err == nil && origMsg.Payload != nil {
				for _, h := range origMsg.Payload.Headers {
					if h.Name == "Message-Id" {
						fmt.Fprintf(&raw, "In-Reply-To: %s\r\n", h.Value)
						fmt.Fprintf(&raw, "References: %s\r\n", h.Value)
					}
				}
			}
		}
		raw.WriteString("\r\n")
		raw.WriteString(input.Body)

		draft := &gmailapi.Draft{
			Message: &gmailapi.Message{
				Raw: base64.URLEncoding.EncodeToString([]byte(raw.String())),
			},
		}

		if input.ReplyTo != "" {
			// Look up the thread ID so the draft stays in the conversation.
			origMsg, err := svc.Users.Messages.Get("me", input.ReplyTo).Format("minimal").Do()
			if err == nil {
				draft.Message.ThreadId = origMsg.ThreadId
			}
		}

		created, err := svc.Users.Drafts.Create("me", draft).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("creating draft: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Draft created successfully.\nDraft ID: %s\nMessage ID: %s",
					created.Id, created.Message.Id)},
			},
		}, nil, nil
	})
}

// --- gmail_draft_list ---

type draftListInput struct {
	Account    string `json:"account" jsonschema:"required,description=Account name (e.g. 'personal'\\, 'work') or 'all' to list drafts from all accounts"`
	MaxResults int64  `json:"max_results,omitempty" jsonschema:"description=Maximum number of drafts per account (default 20\\, max 100)"`
}

func registerDraftList(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gmail_draft_list",
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

// --- gmail_draft_send ---

type draftSendInput struct {
	Account string `json:"account" jsonschema:"required,description=Account name to use"`
	DraftID string `json:"draft_id" jsonschema:"required,description=Draft ID to send (from gmail_draft_list or gmail_draft_create)"`
}

func registerDraftSend(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gmail_draft_send",
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
				&mcp.TextContent{Text: fmt.Sprintf("Draft sent successfully.\nMessage ID: %s\nThread ID: %s",
					sent.Id, sent.ThreadId)},
			},
		}, nil, nil
	})
}
