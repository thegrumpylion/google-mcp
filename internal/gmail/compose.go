package gmail

import (
	"encoding/base64"
	"fmt"
	"math/rand/v2"
	"strings"

	gmailapi "google.golang.org/api/gmail/v1"
)

// attachment represents a file attachment for an email message.
type attachment struct {
	Name     string `json:"name" jsonschema:"Filename (e.g. 'report.pdf')"`
	MIMEType string `json:"mime_type" jsonschema:"MIME type (e.g. 'application/pdf', 'text/csv'). Auto-detected from name if omitted."`
	Content  string `json:"content" jsonschema:"File content as base64-encoded data"`
}

// driveAttachment references a Google Drive file to attach to an email.
// The file content is fetched server-side and never enters the LLM context.
type driveAttachment struct {
	DriveAccount string `json:"drive_account" jsonschema:"Drive account name"`
	FileID       string `json:"file_id" jsonschema:"Google Drive file ID to attach"`
}

// localAttachment references a local file to attach to an email.
// The file is read from a directory allowed via --allow-read-dir or --allow-write-dir.
// Requires local file access to be enabled on the server.
type localAttachment struct {
	Path string `json:"path" jsonschema:"File path relative to an allowed directory (e.g. 'reports/q4.pdf')"`
}

// composeInput holds the common fields for composing an email message.
type composeInput struct {
	To               string            `json:"to" jsonschema:"Recipient email address"`
	Subject          string            `json:"subject" jsonschema:"Email subject line"`
	Body             string            `json:"body" jsonschema:"Email body (plain text)"`
	Cc               string            `json:"cc,omitempty" jsonschema:"CC recipients (comma-separated email addresses)"`
	Bcc              string            `json:"bcc,omitempty" jsonschema:"BCC recipients (comma-separated email addresses)"`
	Attachments      []attachment      `json:"attachments,omitempty" jsonschema:"File attachments (base64-encoded content)"`
	DriveAttachments []driveAttachment `json:"drive_attachments,omitempty" jsonschema:"Google Drive files to attach (fetched server-side, content never enters conversation)"`
	LocalAttachments []localAttachment `json:"local_attachments,omitempty" jsonschema:"Local files to attach (read from directories allowed via --allow-read-dir). Requires local file access to be enabled."`
}

// composeResult holds the result of building an email message.
type composeResult struct {
	// Raw is the base64url-encoded RFC 2822 message.
	Raw string
	// ThreadID is set when replying to an existing message.
	ThreadID string
}

// buildMessage builds an RFC 2822 message from the compose input.
// If replyToMsgID is non-empty, the original message is fetched to set
// In-Reply-To/References headers and resolve the thread ID.
// When attachments are present, the message is built as multipart/mixed.
func buildMessage(svc *gmailapi.Service, input composeInput, replyToMsgID string) (*composeResult, error) {
	// Validate attachments upfront.
	for i, att := range input.Attachments {
		if att.Name == "" {
			return nil, fmt.Errorf("attachment[%d]: name is required", i)
		}
		if att.Content == "" {
			return nil, fmt.Errorf("attachment[%d] (%s): content is required", i, att.Name)
		}
		// Validate base64 content.
		if _, err := base64.StdEncoding.DecodeString(att.Content); err != nil {
			return nil, fmt.Errorf("attachment[%d] (%s): invalid base64 content: %w", i, att.Name, err)
		}
	}

	var threadID string

	// Resolve reply-to headers and thread ID.
	var replyHeaders string
	if replyToMsgID != "" {
		origMsg, err := svc.Users.Messages.Get("me", replyToMsgID).
			Format("metadata").
			MetadataHeaders("Message-Id").
			Do()
		if err != nil {
			return nil, fmt.Errorf("fetching reply-to message %s: %w", replyToMsgID, err)
		}
		if origMsg.Payload != nil {
			for _, h := range origMsg.Payload.Headers {
				if h.Name == "Message-Id" {
					replyHeaders = fmt.Sprintf("In-Reply-To: %s\r\nReferences: %s\r\n", h.Value, h.Value)
				}
			}
		}
		threadID = origMsg.ThreadId
	}

	var raw string
	if len(input.Attachments) == 0 {
		raw = buildPlainMessage(input, replyHeaders)
	} else {
		raw = buildMultipartMessage(input, replyHeaders)
	}

	return &composeResult{
		Raw:      base64.URLEncoding.EncodeToString([]byte(raw)),
		ThreadID: threadID,
	}, nil
}

// buildPlainMessage builds a simple text/plain RFC 2822 message.
func buildPlainMessage(input composeInput, replyHeaders string) string {
	var raw strings.Builder
	writeCommonHeaders(&raw, input, replyHeaders)
	fmt.Fprintf(&raw, "Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	raw.WriteString("\r\n")
	raw.WriteString(input.Body)
	return raw.String()
}

// buildMultipartMessage builds a multipart/mixed RFC 2822 message with attachments.
func buildMultipartMessage(input composeInput, replyHeaders string) string {
	boundary := generateBoundary()

	var raw strings.Builder
	writeCommonHeaders(&raw, input, replyHeaders)
	fmt.Fprintf(&raw, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&raw, "Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundary)
	raw.WriteString("\r\n")

	// Text body part.
	fmt.Fprintf(&raw, "--%s\r\n", boundary)
	fmt.Fprintf(&raw, "Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	raw.WriteString("\r\n")
	raw.WriteString(input.Body)
	raw.WriteString("\r\n")

	// Attachment parts.
	for _, att := range input.Attachments {
		mimeType := att.MIMEType
		if mimeType == "" {
			mimeType = guessMIMEType(att.Name)
		}
		fmt.Fprintf(&raw, "--%s\r\n", boundary)
		fmt.Fprintf(&raw, "Content-Type: %s; name=\"%s\"\r\n", mimeType, att.Name)
		fmt.Fprintf(&raw, "Content-Disposition: attachment; filename=\"%s\"\r\n", att.Name)
		fmt.Fprintf(&raw, "Content-Transfer-Encoding: base64\r\n")
		raw.WriteString("\r\n")
		// Write base64 content in 76-char lines per RFC 2045.
		writeBase64Lines(&raw, att.Content)
		raw.WriteString("\r\n")
	}

	// Closing boundary.
	fmt.Fprintf(&raw, "--%s--\r\n", boundary)
	return raw.String()
}

// writeCommonHeaders writes the shared headers (To, Cc, Bcc, Subject, reply).
func writeCommonHeaders(w *strings.Builder, input composeInput, replyHeaders string) {
	fmt.Fprintf(w, "To: %s\r\n", input.To)
	if input.Cc != "" {
		fmt.Fprintf(w, "Cc: %s\r\n", input.Cc)
	}
	if input.Bcc != "" {
		fmt.Fprintf(w, "Bcc: %s\r\n", input.Bcc)
	}
	fmt.Fprintf(w, "Subject: %s\r\n", mime2047Encode(input.Subject))
	if replyHeaders != "" {
		w.WriteString(replyHeaders)
	}
}

// generateBoundary creates a random MIME boundary string.
func generateBoundary() string {
	const chars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, 32)
	for i := range b {
		b[i] = chars[rand.IntN(len(chars))]
	}
	return "mcp_" + string(b)
}

// writeBase64Lines writes base64-encoded data in 76-character lines per RFC 2045.
func writeBase64Lines(w *strings.Builder, data string) {
	for len(data) > 0 {
		end := 76
		if end > len(data) {
			end = len(data)
		}
		w.WriteString(data[:end])
		w.WriteString("\r\n")
		data = data[end:]
	}
}

// guessMIMEType returns a MIME type based on the file extension.
func guessMIMEType(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(lower, ".zip"):
		return "application/zip"
	case strings.HasSuffix(lower, ".gz"), strings.HasSuffix(lower, ".gzip"):
		return "application/gzip"
	case strings.HasSuffix(lower, ".tar"):
		return "application/x-tar"
	case strings.HasSuffix(lower, ".doc"):
		return "application/msword"
	case strings.HasSuffix(lower, ".docx"):
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case strings.HasSuffix(lower, ".xls"):
		return "application/vnd.ms-excel"
	case strings.HasSuffix(lower, ".xlsx"):
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case strings.HasSuffix(lower, ".ppt"):
		return "application/vnd.ms-powerpoint"
	case strings.HasSuffix(lower, ".pptx"):
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".txt"):
		return "text/plain"
	case strings.HasSuffix(lower, ".csv"):
		return "text/csv"
	case strings.HasSuffix(lower, ".html"), strings.HasSuffix(lower, ".htm"):
		return "text/html"
	case strings.HasSuffix(lower, ".json"):
		return "application/json"
	case strings.HasSuffix(lower, ".xml"):
		return "application/xml"
	case strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"):
		return "application/yaml"
	case strings.HasSuffix(lower, ".mp3"):
		return "audio/mpeg"
	case strings.HasSuffix(lower, ".mp4"):
		return "video/mp4"
	case strings.HasSuffix(lower, ".mov"):
		return "video/quicktime"
	case strings.HasSuffix(lower, ".avi"):
		return "video/x-msvideo"
	default:
		return "application/octet-stream"
	}
}
