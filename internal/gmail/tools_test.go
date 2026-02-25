package gmail

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/localfs"
	"github.com/thegrumpylion/google-mcp/internal/server"
	gmailapi "google.golang.org/api/gmail/v1"
)

// connect creates an in-memory client session connected to the given server.
// This follows the pattern from the MCP Go SDK's own tests.
func connect(t *testing.T, server *server.Server) *mcp.ClientSession {
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

func newTestServer(t *testing.T) *server.Server {
	t.Helper()
	mgr := newTestManager(t)
	server := server.NewServer(&mcp.Implementation{Name: "test-gmail", Version: "test"}, nil)
	RegisterTools(server, mgr)
	return server
}

func listTools(t *testing.T, server *server.Server) []*mcp.Tool {
	t.Helper()
	ctx := context.Background()
	session := connect(t, server)
	res, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	return res.Tools
}

func listToolNames(t *testing.T, server *server.Server) []string {
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
	server := server.NewServer(&mcp.Implementation{Name: "test-gmail", Version: "test"}, nil)

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
		"save_attachment_to_drive",
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
		"save_attachment_to_drive",
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

// --- compose / MIME tests ---

func TestBuildPlainMessage(t *testing.T) {
	input := composeInput{
		To:      "alice@example.com",
		Subject: "Test Subject",
		Body:    "Hello, world!",
		Cc:      "bob@example.com",
	}
	raw := buildPlainMessage(input, "")

	checks := []string{
		"To: alice@example.com\r\n",
		"Subject: Test Subject\r\n",
		"Cc: bob@example.com\r\n",
		"Content-Type: text/plain; charset=\"UTF-8\"\r\n",
		"\r\nHello, world!",
	}
	for _, want := range checks {
		if !strings.Contains(raw, want) {
			t.Errorf("buildPlainMessage() missing %q", want)
		}
	}
	// Should NOT contain multipart headers.
	if strings.Contains(raw, "MIME-Version") {
		t.Error("plain message should not contain MIME-Version")
	}
	if strings.Contains(raw, "multipart") {
		t.Error("plain message should not contain multipart")
	}
}

func TestBuildPlainMessage_WithReplyHeaders(t *testing.T) {
	input := composeInput{
		To:      "alice@example.com",
		Subject: "Re: Test",
		Body:    "Reply body",
	}
	replyHeaders := "In-Reply-To: <msg-123@example.com>\r\nReferences: <msg-123@example.com>\r\n"
	raw := buildPlainMessage(input, replyHeaders)

	if !strings.Contains(raw, "In-Reply-To: <msg-123@example.com>") {
		t.Error("missing In-Reply-To header")
	}
	if !strings.Contains(raw, "References: <msg-123@example.com>") {
		t.Error("missing References header")
	}
}

func TestBuildMultipartMessage(t *testing.T) {
	pdfContent := base64.StdEncoding.EncodeToString([]byte("fake pdf content"))
	input := composeInput{
		To:      "alice@example.com",
		Subject: "With Attachment",
		Body:    "See attached.",
		Attachments: []attachment{
			{
				Name:     "report.pdf",
				MIMEType: "application/pdf",
				Content:  pdfContent,
			},
		},
	}
	raw := buildMultipartMessage(input, "")

	checks := []string{
		"To: alice@example.com\r\n",
		"Subject: With Attachment\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: multipart/mixed; boundary=",
		"Content-Type: text/plain; charset=\"UTF-8\"",
		"See attached.",
		"Content-Type: application/pdf; name=\"report.pdf\"",
		"Content-Disposition: attachment; filename=\"report.pdf\"",
		"Content-Transfer-Encoding: base64",
	}
	for _, want := range checks {
		if !strings.Contains(raw, want) {
			t.Errorf("buildMultipartMessage() missing %q", want)
		}
	}
	// Should contain the base64 attachment content.
	if !strings.Contains(raw, pdfContent) {
		t.Error("multipart message should contain base64 attachment content")
	}
}

func TestBuildMultipartMessage_MultipleAttachments(t *testing.T) {
	att1 := base64.StdEncoding.EncodeToString([]byte("content1"))
	att2 := base64.StdEncoding.EncodeToString([]byte("content2"))
	input := composeInput{
		To:      "alice@example.com",
		Subject: "Two Files",
		Body:    "Here are two files.",
		Attachments: []attachment{
			{Name: "file1.txt", MIMEType: "text/plain", Content: att1},
			{Name: "file2.csv", MIMEType: "text/csv", Content: att2},
		},
	}
	raw := buildMultipartMessage(input, "")

	if !strings.Contains(raw, "file1.txt") {
		t.Error("missing first attachment")
	}
	if !strings.Contains(raw, "file2.csv") {
		t.Error("missing second attachment")
	}
	// Count boundary markers: should have opening for body + 2 attachments + closing.
	// Extract boundary from Content-Type header.
	idx := strings.Index(raw, "boundary=\"")
	if idx == -1 {
		t.Fatal("no boundary found")
	}
	boundaryStart := idx + len("boundary=\"")
	boundaryEnd := strings.Index(raw[boundaryStart:], "\"")
	boundary := raw[boundaryStart : boundaryStart+boundaryEnd]

	count := strings.Count(raw, "--"+boundary)
	// 3 opening boundaries (body + 2 attachments) + 1 closing boundary (--boundary--)
	if count != 4 {
		t.Errorf("expected 4 boundary markers, got %d", count)
	}
}

func TestBuildMultipartMessage_MIMETypeGuess(t *testing.T) {
	content := base64.StdEncoding.EncodeToString([]byte("data"))
	input := composeInput{
		To:      "alice@example.com",
		Subject: "Auto MIME",
		Body:    "See attached.",
		Attachments: []attachment{
			{Name: "photo.png", Content: content}, // No MIMEType set.
		},
	}
	raw := buildMultipartMessage(input, "")

	if !strings.Contains(raw, "Content-Type: image/png; name=\"photo.png\"") {
		t.Error("should auto-detect image/png from .png extension")
	}
}

func TestGenerateBoundary(t *testing.T) {
	b1 := generateBoundary()
	b2 := generateBoundary()

	if !strings.HasPrefix(b1, "mcp_") {
		t.Errorf("boundary should start with 'mcp_', got %q", b1)
	}
	if len(b1) != 4+32 { // "mcp_" + 32 random chars
		t.Errorf("boundary length = %d, want 36", len(b1))
	}
	if b1 == b2 {
		t.Error("two generated boundaries should not be identical")
	}
}

func TestWriteBase64Lines(t *testing.T) {
	// Create content longer than 76 chars.
	data := strings.Repeat("A", 200)
	var sb strings.Builder
	writeBase64Lines(&sb, data)
	result := sb.String()

	lines := strings.Split(strings.TrimRight(result, "\r\n"), "\r\n")
	for i, line := range lines {
		if i < len(lines)-1 && len(line) != 76 {
			t.Errorf("line[%d] length = %d, want 76", i, len(line))
		}
	}
	// Last line should be the remainder.
	if len(lines[len(lines)-1]) != 200-76*2 {
		t.Errorf("last line length = %d, want %d", len(lines[len(lines)-1]), 200-76*2)
	}
}

func TestWriteBase64Lines_Short(t *testing.T) {
	data := "short"
	var sb strings.Builder
	writeBase64Lines(&sb, data)
	result := sb.String()
	if result != "short\r\n" {
		t.Errorf("writeBase64Lines(%q) = %q, want %q", data, result, "short\r\n")
	}
}

func TestGuessMIMEType(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"report.pdf", "application/pdf"},
		{"PHOTO.PNG", "image/png"},
		{"data.csv", "text/csv"},
		{"archive.zip", "application/zip"},
		{"doc.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"sheet.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{"slides.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
		{"image.jpg", "image/jpeg"},
		{"image.jpeg", "image/jpeg"},
		{"image.gif", "image/gif"},
		{"icon.svg", "image/svg+xml"},
		{"photo.webp", "image/webp"},
		{"page.html", "text/html"},
		{"page.htm", "text/html"},
		{"config.json", "application/json"},
		{"config.xml", "application/xml"},
		{"config.yaml", "application/yaml"},
		{"config.yml", "application/yaml"},
		{"song.mp3", "audio/mpeg"},
		{"video.mp4", "video/mp4"},
		{"clip.mov", "video/quicktime"},
		{"movie.avi", "video/x-msvideo"},
		{"notes.txt", "text/plain"},
		{"backup.tar", "application/x-tar"},
		{"backup.gz", "application/gzip"},
		{"old.doc", "application/msword"},
		{"old.xls", "application/vnd.ms-excel"},
		{"old.ppt", "application/vnd.ms-powerpoint"},
		{"unknown.xyz", "application/octet-stream"},
		{"noext", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := guessMIMEType(tt.filename)
			if got != tt.want {
				t.Errorf("guessMIMEType(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestBuildMessage_AttachmentValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   composeInput
		wantErr string
	}{
		{
			name: "missing attachment name",
			input: composeInput{
				To: "a@b.com", Subject: "t", Body: "b",
				Attachments: []attachment{{Content: base64.StdEncoding.EncodeToString([]byte("x"))}},
			},
			wantErr: "name is required",
		},
		{
			name: "missing attachment content",
			input: composeInput{
				To: "a@b.com", Subject: "t", Body: "b",
				Attachments: []attachment{{Name: "file.txt"}},
			},
			wantErr: "content is required",
		},
		{
			name: "invalid base64",
			input: composeInput{
				To: "a@b.com", Subject: "t", Body: "b",
				Attachments: []attachment{{Name: "file.txt", Content: "not-valid-base64!!!"}},
			},
			wantErr: "invalid base64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// buildMessage requires a gmail service for reply-to, but nil is fine
			// when replyToMsgID is empty and we expect validation to fail first.
			_, err := buildMessage(nil, tt.input, "")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestBuildMessage_NoAttachments_PlainOutput(t *testing.T) {
	input := composeInput{
		To:      "alice@example.com",
		Subject: "Plain",
		Body:    "No attachments here.",
	}
	result, err := buildMessage(nil, input, "")
	if err != nil {
		t.Fatal(err)
	}

	// Decode and verify it's a plain message.
	raw, err := base64.URLEncoding.DecodeString(result.Raw)
	if err != nil {
		t.Fatal(err)
	}
	msg := string(raw)
	if strings.Contains(msg, "multipart") {
		t.Error("message without attachments should not be multipart")
	}
	if !strings.Contains(msg, "Content-Type: text/plain") {
		t.Error("plain message should have text/plain content type")
	}
}

func TestBuildMessage_WithAttachments_MultipartOutput(t *testing.T) {
	content := base64.StdEncoding.EncodeToString([]byte("file data"))
	input := composeInput{
		To:      "alice@example.com",
		Subject: "With Attachment",
		Body:    "See attached.",
		Attachments: []attachment{
			{Name: "test.txt", MIMEType: "text/plain", Content: content},
		},
	}
	result, err := buildMessage(nil, input, "")
	if err != nil {
		t.Fatal(err)
	}

	raw, err := base64.URLEncoding.DecodeString(result.Raw)
	if err != nil {
		t.Fatal(err)
	}
	msg := string(raw)
	if !strings.Contains(msg, "multipart/mixed") {
		t.Error("message with attachments should be multipart/mixed")
	}
	if !strings.Contains(msg, "MIME-Version: 1.0") {
		t.Error("multipart message should have MIME-Version header")
	}
	if !strings.Contains(msg, "Content-Disposition: attachment; filename=\"test.txt\"") {
		t.Error("should contain attachment disposition")
	}
}

func TestToolNames_WithLocalFS(t *testing.T) {
	mgr := newTestManager(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("x"), 0644)

	lfs, err := localfs.New([]localfs.Dir{
		{Path: dir, Mode: localfs.ModeRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lfs.Close()

	srv := server.NewServer(&mcp.Implementation{Name: "test-gmail", Version: "test"}, nil)
	srv.SetLocalFS(lfs)
	RegisterTools(srv, mgr)

	got := listToolNames(t, srv)

	// Should include all 26 base tools + 2 localfs tools = 28.
	if len(got) != 28 {
		t.Fatalf("got %d tools, want 28\ngot: %v", len(got), got)
	}

	// Verify the localfs tools are present.
	names := make(map[string]bool)
	for _, n := range got {
		names[n] = true
	}
	if !names["list_local_files"] {
		t.Error("expected list_local_files tool")
	}
	if !names["read_local_file"] {
		t.Error("expected read_local_file tool")
	}
}
