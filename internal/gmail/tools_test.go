package gmail

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	gmailapi "google.golang.org/api/gmail/v1"
)

// connect creates an in-memory client session connected to the given server.
// This follows the pattern from the MCP Go SDK's own tests.
func connect(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

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

func newTestServer(t *testing.T) *mcp.Server {
	t.Helper()
	mgr := newTestManager(t)
	server := mcp.NewServer(&mcp.Implementation{Name: "test-gmail", Version: "test"}, nil)
	RegisterTools(server, mgr)
	return server
}

func listTools(t *testing.T, server *mcp.Server) []*mcp.Tool {
	t.Helper()
	ctx := context.Background()
	session := connect(t, server)
	res, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	return res.Tools
}

func listToolNames(t *testing.T, server *mcp.Server) []string {
	t.Helper()
	tools := listTools(t, server)
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

func TestRegisterTools(t *testing.T) {
	mgr := newTestManager(t)
	server := mcp.NewServer(&mcp.Implementation{Name: "test-gmail", Version: "test"}, nil)

	// This should not panic — the exact bug we fixed with jsonschema tags.
	RegisterTools(server, mgr)
}

func TestToolNames(t *testing.T) {
	server := newTestServer(t)
	got := listToolNames(t, server)

	want := []string{
		"create_draft",
		"create_label",
		"delete_draft",
		"delete_label",
		"delete_message",
		"get_attachment",
		"get_draft",
		"get_label",
		"get_profile",
		"get_vacation",
		"list_accounts",
		"list_drafts",
		"list_labels",
		"list_threads",
		"modify_messages",
		"modify_thread",
		"read_message",
		"read_thread",
		"search_messages",
		"send_draft",
		"send_message",
		"trash_thread",
		"untrash_thread",
		"update_draft",
		"update_vacation",
	}

	if len(got) != len(want) {
		t.Fatalf("got %d tools, want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tool[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestToolAnnotations(t *testing.T) {
	server := newTestServer(t)
	tools := listTools(t, server)

	toolMap := make(map[string]*mcp.Tool)
	for _, tool := range tools {
		toolMap[tool.Name] = tool
	}

	readOnly := []string{
		"list_accounts", "search_messages", "read_message", "read_thread",
		"list_labels", "get_attachment", "list_drafts", "get_draft",
		"get_profile", "get_label", "get_vacation", "list_threads",
	}
	for _, name := range readOnly {
		tool := toolMap[name]
		if tool == nil {
			t.Errorf("tool %q not found", name)
			continue
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %q should have ReadOnlyHint=true", name)
		}
	}

	mutations := []string{
		"send_message", "modify_messages", "modify_thread",
		"create_draft", "update_draft", "delete_draft", "send_draft",
		"create_label", "delete_label", "delete_message",
		"trash_thread", "untrash_thread", "update_vacation",
	}
	for _, name := range mutations {
		tool := toolMap[name]
		if tool == nil {
			t.Errorf("tool %q not found", name)
			continue
		}
		if tool.Annotations != nil && tool.Annotations.ReadOnlyHint {
			t.Errorf("mutation tool %q should not have ReadOnlyHint=true", name)
		}
	}
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
