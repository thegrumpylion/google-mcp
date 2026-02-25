package drive

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
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
	server := mcp.NewServer(&mcp.Implementation{Name: "test-drive", Version: "test"}, nil)
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

func TestAccountScopes(t *testing.T) {
	scopes := AccountScopes()
	if len(scopes) == 0 {
		t.Error("AccountScopes() returned empty slice")
	}
}
