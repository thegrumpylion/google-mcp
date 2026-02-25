package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
	gmailapi "google.golang.org/api/gmail/v1"
)

// attachmentInfo holds metadata about a message attachment.
type attachmentInfo struct {
	filename     string
	mimeType     string
	size         int64
	attachmentID string
}

// listAttachments recursively finds all attachments in a message payload.
func listAttachments(part *gmailapi.MessagePart) []attachmentInfo {
	if part == nil {
		return nil
	}

	var result []attachmentInfo

	// A part is an attachment if it has a filename and an attachment ID.
	if part.Filename != "" && part.Body != nil && part.Body.AttachmentId != "" {
		result = append(result, attachmentInfo{
			filename:     part.Filename,
			mimeType:     part.MimeType,
			size:         part.Body.Size,
			attachmentID: part.Body.AttachmentId,
		})
	}

	// Recurse into sub-parts.
	for _, p := range part.Parts {
		result = append(result, listAttachments(p)...)
	}

	return result
}

// --- gmail_get_attachment ---

type getAttachmentInput struct {
	Account      string `json:"account" jsonschema:"Account name"`
	MessageID    string `json:"message_id" jsonschema:"Gmail message ID that contains the attachment"`
	AttachmentID string `json:"attachment_id" jsonschema:"Attachment ID (from read or read_thread results)"`
	SaveTo       string `json:"save_to,omitempty" jsonschema:"Save to a local file instead of returning content (path relative to an allowed directory). Requires --allow-write-dir. Content never enters the conversation."`
}

func registerGetAttachment(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name: "get_attachment",
		Description: `Download a Gmail message attachment by ID.

By default, returns content in the conversation (text for text-like files, base64 for binary).
Set save_to to write the file to a local directory instead — content never enters the conversation.
Use read_message to discover attachment IDs.`,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input getAttachmentInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Gmail service: %w", err)
		}

		att, err := svc.Users.Messages.Attachments.Get("me", input.MessageID, input.AttachmentID).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting attachment: %w", err)
		}

		// The API returns URL-safe base64. Decode.
		data, err := base64.URLEncoding.DecodeString(att.Data)
		if err != nil {
			// Fall back to returning raw base64.
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Attachment data (base64, %d bytes encoded):\n%s", att.Size, att.Data)},
				},
			}, nil, nil
		}

		// If save_to is set, write to local filesystem instead of returning content.
		if input.SaveTo != "" {
			lfs := srv.LocalFS()
			if lfs == nil {
				return nil, nil, fmt.Errorf("local file access is not enabled (use --allow-write-dir)")
			}

			dir, err := lfs.WriteFile(input.SaveTo, data)
			if err != nil {
				return nil, nil, fmt.Errorf("saving attachment: %w", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Attachment saved to local disk.\n\nSize: %d bytes\nSaved to: %s/%s",
						len(data), dir, input.SaveTo)},
				},
			}, nil, nil
		}

		// If it's small enough and looks like text, return as text.
		const maxTextSize = 256 * 1024 // 256 KB
		if len(data) <= maxTextSize && isLikelyText(data) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Attachment content (%d bytes):\n\n%s", len(data), string(data))},
				},
			}, nil, nil
		}

		// Otherwise return base64.
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Attachment data (binary, %d bytes). Base64 encoded:\n%s",
					len(data), base64.StdEncoding.EncodeToString(data))},
			},
		}, nil, nil
	})
}

// isLikelyText checks if data appears to be text content by looking for
// null bytes and checking the ratio of printable characters.
func isLikelyText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	// Check first 512 bytes for null bytes (strong binary indicator).
	check := data
	if len(check) > 512 {
		check = check[:512]
	}
	for _, b := range check {
		if b == 0 {
			return false
		}
	}
	// Count printable ASCII + common whitespace.
	printable := 0
	for _, b := range check {
		if (b >= 32 && b < 127) || b == '\n' || b == '\r' || b == '\t' {
			printable++
		}
	}
	return float64(printable)/float64(len(check)) > 0.85
}

// formatAttachmentList formats attachments for a message, using the message ID for context.
func formatAttachmentList(msgID string, parts []*gmailapi.MessagePart) string {
	attachments := listAttachments(&gmailapi.MessagePart{Parts: parts})
	if len(attachments) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Attachments:\n")
	for _, a := range attachments {
		fmt.Fprintf(&sb, "  - %s (MIME: %s, Size: %d bytes, Attachment ID: %s)\n",
			a.filename, a.mimeType, a.size, a.attachmentID)
	}
	return sb.String()
}
