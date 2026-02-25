package drive

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/localfs"
	"github.com/thegrumpylion/google-mcp/internal/server"
	driveapi "google.golang.org/api/drive/v3"
)

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
	server := server.NewServer(&mcp.Implementation{Name: "test-drive", Version: "test"}, nil)
	RegisterTools(server, mgr)
}

func TestIsGoogleWorkspaceFile(t *testing.T) {
	tests := []struct {
		mimeType string
		want     bool
	}{
		{"application/vnd.google-apps.document", true},
		{"application/vnd.google-apps.spreadsheet", true},
		{"application/vnd.google-apps.presentation", true},
		{"application/vnd.google-apps.drawing", true},
		{"application/vnd.google-apps.script", true},
		{"application/pdf", false},
		{"text/plain", false},
		{"image/png", false},
		{"application/vnd.google-apps.folder", false},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			got := isGoogleWorkspaceFile(tt.mimeType)
			if got != tt.want {
				t.Errorf("isGoogleWorkspaceFile(%q) = %v, want %v", tt.mimeType, got, tt.want)
			}
		})
	}
}

func TestDefaultExportMIME(t *testing.T) {
	tests := []struct {
		mimeType string
		want     string
	}{
		{"application/vnd.google-apps.document", "text/plain"},
		{"application/vnd.google-apps.spreadsheet", "text/csv"},
		{"application/vnd.google-apps.presentation", "text/plain"},
		{"application/vnd.google-apps.drawing", "image/png"},
		{"application/vnd.google-apps.script", "application/vnd.google-apps.script+json"},
		{"unknown/type", "text/plain"},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			got := defaultExportMIME(tt.mimeType)
			if got != tt.want {
				t.Errorf("defaultExportMIME(%q) = %q, want %q", tt.mimeType, got, tt.want)
			}
		})
	}
}

func TestFormatFileList(t *testing.T) {
	files := []*driveapi.File{
		{
			Id:           "file-1",
			Name:         "report.pdf",
			MimeType:     "application/pdf",
			Size:         1024,
			ModifiedTime: "2024-01-15T10:00:00Z",
			WebViewLink:  "https://drive.google.com/file/d/file-1/view",
		},
		{
			Id:       "file-2",
			Name:     "notes.txt",
			MimeType: "text/plain",
		},
	}

	result := formatFileList(files, "personal")

	if !strings.Contains(result, "Found 2 files") {
		t.Error("result should contain file count")
	}
	if !strings.Contains(result, "report.pdf") {
		t.Error("result should contain first file name")
	}
	if !strings.Contains(result, "notes.txt") {
		t.Error("result should contain second file name")
	}
	if !strings.Contains(result, "file-1") {
		t.Error("result should contain first file ID")
	}
	if !strings.Contains(result, "Account: personal") {
		t.Error("result should contain account name")
	}
	if !strings.Contains(result, "1024 bytes") {
		t.Error("result should contain file size")
	}
	if !strings.Contains(result, "2024-01-15") {
		t.Error("result should contain modified time")
	}
}

func TestFormatFileList_Empty(t *testing.T) {
	result := formatFileList([]*driveapi.File{}, "work")
	if !strings.Contains(result, "Found 0 files") {
		t.Errorf("formatFileList() = %q, want 'Found 0 files'", result)
	}
}

func TestFormatPermission(t *testing.T) {
	perm := &driveapi.Permission{
		Id:           "perm-123",
		Role:         "writer",
		Type:         "user",
		DisplayName:  "Alice",
		EmailAddress: "alice@example.com",
	}

	result := formatPermission(perm)

	if !strings.Contains(result, "perm-123") {
		t.Error("result should contain permission ID")
	}
	if !strings.Contains(result, "writer") {
		t.Error("result should contain role")
	}
	if !strings.Contains(result, "user") {
		t.Error("result should contain type")
	}
	if !strings.Contains(result, "Alice") {
		t.Error("result should contain display name")
	}
	if !strings.Contains(result, "alice@example.com") {
		t.Error("result should contain email address")
	}
}

func TestFormatPermission_Anyone(t *testing.T) {
	perm := &driveapi.Permission{
		Id:   "anyoneWithLink",
		Role: "reader",
		Type: "anyone",
	}

	result := formatPermission(perm)

	if !strings.Contains(result, "anyone") {
		t.Error("result should contain type 'anyone'")
	}
	if strings.Contains(result, "Email:") {
		t.Error("result should not contain email for 'anyone' type")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 bytes"},
		{512, "512 bytes"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1048576, "1.00 MB"},
		{1073741824, "1.00 GB"},
		{1099511627776, "1.00 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatBytes(tt.bytes)
			if !strings.Contains(got, tt.want) && got != tt.want {
				t.Errorf("formatBytes(%d) = %q, want to contain %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestAccountScopes(t *testing.T) {
	scopes := AccountScopes()
	if len(scopes) == 0 {
		t.Error("AccountScopes() returned empty slice")
	}
}

func newTestServer(t *testing.T) *server.Server {
	t.Helper()
	mgr := newTestManager(t)
	server := server.NewServer(&mcp.Implementation{Name: "test-drive", Version: "test"}, nil)
	RegisterTools(server, mgr)
	return server
}

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

func TestToolNames(t *testing.T) {
	server := newTestServer(t)
	tools := listTools(t, server)
	got := make([]string, 0, len(tools))
	for _, tool := range tools {
		got = append(got, tool.Name)
	}
	sort.Strings(got)

	want := []string{
		"copy_file",
		"create_folder",
		"delete_file",
		"delete_permission",
		"empty_trash",
		"get_about",
		"get_file",
		"get_permission",
		"get_shared_drive",
		"list_accounts",
		"list_files",
		"list_permissions",
		"list_shared_drives",
		"move_file",
		"read_file",
		"search_files",
		"share_file",
		"update_file",
		"update_permission",
		"upload_file",
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
		"list_accounts", "search_files", "list_files", "get_file", "read_file",
		"list_permissions", "get_permission", "get_about", "list_shared_drives", "get_shared_drive",
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
		"upload_file", "update_file", "delete_file",
		"create_folder", "move_file", "copy_file", "share_file",
		"update_permission", "delete_permission", "empty_trash",
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

	srv := server.NewServer(&mcp.Implementation{Name: "test-drive", Version: "test"}, nil)
	srv.SetLocalFS(lfs)
	RegisterTools(srv, mgr)

	tools := listTools(t, srv)
	got := make([]string, 0, len(tools))
	for _, tool := range tools {
		got = append(got, tool.Name)
	}
	sort.Strings(got)

	// Should include all 20 base tools + 2 localfs tools = 22.
	if len(got) != 22 {
		t.Fatalf("got %d tools, want 22\ngot: %v", len(got), got)
	}

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
