package gmail

import (
	"encoding/base64"
	"fmt"
	"strings"

	gmailapi "google.golang.org/api/gmail/v1"
)

// composeInput holds the common fields for composing an email message.
type composeInput struct {
	To      string `json:"to" jsonschema:"Recipient email address"`
	Subject string `json:"subject" jsonschema:"Email subject line"`
	Body    string `json:"body" jsonschema:"Email body (plain text)"`
	Cc      string `json:"cc,omitempty" jsonschema:"CC recipients (comma-separated email addresses)"`
	Bcc     string `json:"bcc,omitempty" jsonschema:"BCC recipients (comma-separated email addresses)"`
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
func buildMessage(svc *gmailapi.Service, input composeInput, replyToMsgID string) (*composeResult, error) {
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

	var threadID string

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
					fmt.Fprintf(&raw, "In-Reply-To: %s\r\n", h.Value)
					fmt.Fprintf(&raw, "References: %s\r\n", h.Value)
				}
			}
		}
		threadID = origMsg.ThreadId
	}

	raw.WriteString("\r\n")
	raw.WriteString(input.Body)

	return &composeResult{
		Raw:      base64.URLEncoding.EncodeToString([]byte(raw.String())),
		ThreadID: threadID,
	}, nil
}
