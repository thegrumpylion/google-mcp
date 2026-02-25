package gmail

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	gmailapi "google.golang.org/api/gmail/v1"
)

// newTestManager creates an auth.Manager with a temp config dir and dummy credentials.
func newTestManager(t *testing.T) *auth.Manager {
	t.Helper()
	dir := t.TempDir()
	creds := `{"installed":{"client_id":"x","client_secret":"y","auth_uri":"https://a","token_uri":"https://t","redirect_uris":["http://localhost"]}}`
	if err := os.WriteFile(filepath.Join(dir, "credentials.json"), []byte(creds), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr, err := auth.NewManager(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}

func TestRegisterTools(t *testing.T) {
	mgr := newTestManager(t)
	server := mcp.NewServer(&mcp.Implementation{Name: "test-gmail", Version: "test"}, nil)

	// This should not panic — the exact bug we fixed with jsonschema tags.
	RegisterTools(server, mgr)
}

func TestExtractBody_PlainText(t *testing.T) {
	encoded := base64.URLEncoding.EncodeToString([]byte("Hello, world!"))
	part := &gmailapi.MessagePart{
		MimeType: "text/plain",
		Body:     &gmailapi.MessagePartBody{Data: encoded},
	}

	got := extractBody(part)
	if got != "Hello, world!" {
		t.Errorf("extractBody() = %q, want %q", got, "Hello, world!")
	}
}

func TestExtractBody_HTML(t *testing.T) {
	encoded := base64.URLEncoding.EncodeToString([]byte("<p>Hello</p>"))
	part := &gmailapi.MessagePart{
		MimeType: "text/html",
		Body:     &gmailapi.MessagePartBody{Data: encoded},
	}

	got := extractBody(part)
	if got != "<p>Hello</p>" {
		t.Errorf("extractBody() = %q, want %q", got, "<p>Hello</p>")
	}
}

func TestExtractBody_MultipartPrefersPlain(t *testing.T) {
	plainEncoded := base64.URLEncoding.EncodeToString([]byte("plain text"))
	htmlEncoded := base64.URLEncoding.EncodeToString([]byte("<p>html</p>"))

	part := &gmailapi.MessagePart{
		MimeType: "multipart/alternative",
		Parts: []*gmailapi.MessagePart{
			{
				MimeType: "text/plain",
				Body:     &gmailapi.MessagePartBody{Data: plainEncoded},
			},
			{
				MimeType: "text/html",
				Body:     &gmailapi.MessagePartBody{Data: htmlEncoded},
			},
		},
	}

	got := extractBody(part)
	if got != "plain text" {
		t.Errorf("extractBody() = %q, want %q", got, "plain text")
	}
}

func TestExtractBody_MultipartFallsBackToHTML(t *testing.T) {
	htmlEncoded := base64.URLEncoding.EncodeToString([]byte("<p>html only</p>"))

	part := &gmailapi.MessagePart{
		MimeType: "multipart/alternative",
		Parts: []*gmailapi.MessagePart{
			{
				MimeType: "text/html",
				Body:     &gmailapi.MessagePartBody{Data: htmlEncoded},
			},
		},
	}

	got := extractBody(part)
	if got != "<p>html only</p>" {
		t.Errorf("extractBody() = %q, want %q", got, "<p>html only</p>")
	}
}

func TestExtractBody_Nil(t *testing.T) {
	got := extractBody(nil)
	if got != "" {
		t.Errorf("extractBody(nil) = %q, want empty", got)
	}
}

func TestExtractBody_NoBody(t *testing.T) {
	part := &gmailapi.MessagePart{
		MimeType: "text/plain",
		Body:     &gmailapi.MessagePartBody{},
	}
	got := extractBody(part)
	if got != "" {
		t.Errorf("extractBody() = %q, want empty", got)
	}
}

func TestExtractBody_NestedMultipart(t *testing.T) {
	plainEncoded := base64.URLEncoding.EncodeToString([]byte("deeply nested"))

	part := &gmailapi.MessagePart{
		MimeType: "multipart/mixed",
		Parts: []*gmailapi.MessagePart{
			{
				MimeType: "multipart/alternative",
				Parts: []*gmailapi.MessagePart{
					{
						MimeType: "text/plain",
						Body:     &gmailapi.MessagePartBody{Data: plainEncoded},
					},
				},
			},
		},
	}

	got := extractBody(part)
	if got != "deeply nested" {
		t.Errorf("extractBody() = %q, want %q", got, "deeply nested")
	}
}

func TestMime2047Encode_ASCII(t *testing.T) {
	got := mime2047Encode("Hello World")
	if got != "Hello World" {
		t.Errorf("mime2047Encode(%q) = %q, want %q", "Hello World", got, "Hello World")
	}
}

func TestMime2047Encode_Unicode(t *testing.T) {
	input := "Héllo Wörld"
	got := mime2047Encode(input)

	// Should be RFC 2047 encoded.
	expected := "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(input)) + "?="
	if got != expected {
		t.Errorf("mime2047Encode(%q) = %q, want %q", input, got, expected)
	}
}

func TestIsLikelyText(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"empty", []byte{}, true},
		{"ascii text", []byte("Hello, world!\nSecond line."), true},
		{"json", []byte(`{"key": "value"}`), true},
		{"null byte", []byte("hello\x00world"), false},
		{"binary", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, false}, // PNG header
		{"mostly printable", []byte("abcdefg\x80xyz"), true},                      // 10/11 = ~91% printable
		{"low printable ratio", []byte("abc\x80\x81xyz"), false},                  // 6/8 = 75% below threshold
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isLikelyText(tt.data)
			if got != tt.want {
				t.Errorf("isLikelyText(%q) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}
}

func TestListAttachments(t *testing.T) {
	part := &gmailapi.MessagePart{
		MimeType: "multipart/mixed",
		Parts: []*gmailapi.MessagePart{
			{
				MimeType: "text/plain",
				Body:     &gmailapi.MessagePartBody{Data: "aGVsbG8="},
			},
			{
				Filename: "report.pdf",
				MimeType: "application/pdf",
				Body: &gmailapi.MessagePartBody{
					AttachmentId: "att-123",
					Size:         1024,
				},
			},
			{
				Filename: "image.png",
				MimeType: "image/png",
				Body: &gmailapi.MessagePartBody{
					AttachmentId: "att-456",
					Size:         2048,
				},
			},
		},
	}

	attachments := listAttachments(part)
	if len(attachments) != 2 {
		t.Fatalf("listAttachments() returned %d, want 2", len(attachments))
	}

	if attachments[0].filename != "report.pdf" {
		t.Errorf("attachments[0].filename = %q, want \"report.pdf\"", attachments[0].filename)
	}
	if attachments[0].attachmentID != "att-123" {
		t.Errorf("attachments[0].attachmentID = %q, want \"att-123\"", attachments[0].attachmentID)
	}
	if attachments[1].filename != "image.png" {
		t.Errorf("attachments[1].filename = %q, want \"image.png\"", attachments[1].filename)
	}
}

func TestListAttachments_Nil(t *testing.T) {
	attachments := listAttachments(nil)
	if len(attachments) != 0 {
		t.Errorf("listAttachments(nil) returned %d, want 0", len(attachments))
	}
}

func TestListAttachments_NoAttachments(t *testing.T) {
	part := &gmailapi.MessagePart{
		MimeType: "text/plain",
		Body:     &gmailapi.MessagePartBody{Data: "aGVsbG8="},
	}
	attachments := listAttachments(part)
	if len(attachments) != 0 {
		t.Errorf("listAttachments() returned %d, want 0", len(attachments))
	}
}

func TestAccountScopes(t *testing.T) {
	scopes := AccountScopes()
	if len(scopes) == 0 {
		t.Error("AccountScopes() returned empty slice")
	}
}
