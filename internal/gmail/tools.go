// Package gmail provides MCP tools for interacting with the Gmail API.
package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
	"google.golang.org/api/gmail/v1"
)

// Scopes required by the Gmail tools.
// MailGoogleComScope grants full access to the mailbox including permanent
// deletion, settings, send, and all read/write operations. It is a superset
// of GmailModifyScope, GmailSendScope, and GmailSettingsBasicScope.
var Scopes = []string{
	gmail.MailGoogleComScope,
}

// RegisterTools registers all Gmail MCP tools on the given server.
func RegisterTools(srv *server.Server, mgr *auth.Manager) {
	server.RegisterAccountsListTool(srv, mgr)
	// messages.go
	registerGetProfile(srv, mgr)
	registerSearch(srv, mgr)
	registerRead(srv, mgr)
	registerSend(srv, mgr)
	registerModify(srv, mgr)
	registerDeleteMessage(srv, mgr)
	// threads.go
	registerListThreads(srv, mgr)
	registerReadThread(srv, mgr)
	registerThreadModify(srv, mgr)
	registerTrashThread(srv, mgr)
	registerUntrashThread(srv, mgr)
	// labels.go
	registerListLabels(srv, mgr)
	registerGetLabel(srv, mgr)
	registerCreateLabel(srv, mgr)
	registerDeleteLabel(srv, mgr)
	// attachments.go
	registerGetAttachment(srv, mgr)
	// drafts.go
	registerDraftCreate(srv, mgr)
	registerDraftList(srv, mgr)
	registerDraftGet(srv, mgr)
	registerDraftUpdate(srv, mgr)
	registerDraftDelete(srv, mgr)
	registerDraftSend(srv, mgr)
	// settings.go
	registerGetVacation(srv, mgr)
	registerUpdateVacation(srv, mgr)
}

func newService(ctx context.Context, mgr *auth.Manager, account string) (*gmail.Service, error) {
	opt, err := mgr.ClientOption(ctx, account, Scopes)
	if err != nil {
		return nil, err
	}
	return gmail.NewService(ctx, opt)
}

// extractBody recursively extracts text content from a message payload.
func extractBody(part *gmail.MessagePart) string {
	if part == nil {
		return ""
	}

	// Prefer text/plain, fall back to text/html.
	if part.MimeType == "text/plain" && part.Body != nil && part.Body.Data != "" {
		data, err := base64.URLEncoding.DecodeString(part.Body.Data)
		if err != nil {
			return "(error decoding body)"
		}
		return string(data)
	}

	// For multipart messages, recurse into parts.
	// Prefer text/plain over text/html.
	var htmlBody string
	for _, p := range part.Parts {
		if p.MimeType == "text/plain" {
			if body := extractBody(p); body != "" {
				return body
			}
		}
		if p.MimeType == "text/html" {
			htmlBody = extractBody(p)
		}
		// Recurse into nested multipart.
		if strings.HasPrefix(p.MimeType, "multipart/") {
			if body := extractBody(p); body != "" {
				return body
			}
		}
	}

	// Fall back to HTML if no plain text found.
	if htmlBody == "" && part.MimeType == "text/html" && part.Body != nil && part.Body.Data != "" {
		data, err := base64.URLEncoding.DecodeString(part.Body.Data)
		if err != nil {
			return "(error decoding body)"
		}
		return string(data)
	}

	return htmlBody
}

// mime2047Encode performs a simple RFC 2047 encoding for the Subject header.
func mime2047Encode(s string) string {
	// Check if encoding is needed (non-ASCII characters).
	for _, r := range s {
		if r > 127 {
			return fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(s)))
		}
	}
	return s
}

// AccountScopes returns the scopes used by Gmail tools.
func AccountScopes() []string {
	return Scopes
}
