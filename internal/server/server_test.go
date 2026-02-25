package server

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/localfs"
)

// dummyHandler is a no-op tool handler for testing.
func dummyHandler(_ context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{}, nil, nil
}

// newFilterTestServer creates a Server with a mix of read-only and mutation tools.
func newFilterTestServer(t *testing.T) *Server {
	t.Helper()
	s := NewServer(&mcp.Implementation{Name: "filter-test", Version: "test"}, nil)

	AddTool(s, &mcp.Tool{
		Name:        "read_a",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, dummyHandler)
	AddTool(s, &mcp.Tool{
		Name:        "read_b",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, dummyHandler)
	AddTool(s, &mcp.Tool{
		Name:        "mutate_a",
		Annotations: &mcp.ToolAnnotations{},
	}, dummyHandler)
	AddTool(s, &mcp.Tool{
		Name:        "mutate_b",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: BoolPtr(false)},
	}, dummyHandler)

	return s
}

// listToolNames connects an in-memory client and returns sorted tool names.
func listToolNames(t *testing.T, s *Server) []string {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

func TestServerToolsMetadata(t *testing.T) {
	s := newFilterTestServer(t)
	tools := s.Tools()

	if len(tools) != 4 {
		t.Fatalf("got %d tools, want 4", len(tools))
	}

	// Verify read-only flags are captured correctly.
	m := make(map[string]bool)
	for _, ti := range tools {
		m[ti.Name] = ti.ReadOnly
	}
	if !m["read_a"] || !m["read_b"] {
		t.Error("read_a and read_b should be ReadOnly=true")
	}
	if m["mutate_a"] || m["mutate_b"] {
		t.Error("mutate_a and mutate_b should be ReadOnly=false")
	}
}

func TestApplyFilter_NoFilter(t *testing.T) {
	s := newFilterTestServer(t)
	if err := s.ApplyFilter(ToolFilter{}); err != nil {
		t.Fatal(err)
	}
	got := listToolNames(t, s)
	want := []string{"mutate_a", "mutate_b", "read_a", "read_b"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tool[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestApplyFilter_ReadOnly(t *testing.T) {
	s := newFilterTestServer(t)
	if err := s.ApplyFilter(ToolFilter{ReadOnly: true}); err != nil {
		t.Fatal(err)
	}
	got := listToolNames(t, s)
	want := []string{"read_a", "read_b"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tool[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestApplyFilter_Enable(t *testing.T) {
	s := newFilterTestServer(t)
	if err := s.ApplyFilter(ToolFilter{Enable: []string{"read_a", "mutate_b"}}); err != nil {
		t.Fatal(err)
	}
	got := listToolNames(t, s)
	want := []string{"mutate_b", "read_a"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tool[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestApplyFilter_Disable(t *testing.T) {
	s := newFilterTestServer(t)
	if err := s.ApplyFilter(ToolFilter{Disable: []string{"mutate_a"}}); err != nil {
		t.Fatal(err)
	}
	got := listToolNames(t, s)
	want := []string{"mutate_b", "read_a", "read_b"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tool[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestApplyFilter_ReadOnlyWithEnable(t *testing.T) {
	s := newFilterTestServer(t)
	if err := s.ApplyFilter(ToolFilter{ReadOnly: true, Enable: []string{"read_a"}}); err != nil {
		t.Fatal(err)
	}
	got := listToolNames(t, s)
	want := []string{"read_a"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestApplyFilter_ReadOnlyWithDisable(t *testing.T) {
	s := newFilterTestServer(t)
	if err := s.ApplyFilter(ToolFilter{ReadOnly: true, Disable: []string{"read_b"}}); err != nil {
		t.Fatal(err)
	}
	got := listToolNames(t, s)
	want := []string{"read_a"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestApplyFilter_EnableAndDisable_Error(t *testing.T) {
	s := newFilterTestServer(t)
	err := s.ApplyFilter(ToolFilter{Enable: []string{"read_a"}, Disable: []string{"mutate_a"}})
	if err == nil {
		t.Fatal("expected error for enable+disable, got nil")
	}
}

func TestApplyFilter_UnknownTool_Error(t *testing.T) {
	s := newFilterTestServer(t)
	err := s.ApplyFilter(ToolFilter{Enable: []string{"nonexistent"}})
	if err == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
}

func TestApplyFilter_ReadOnlyEnableMutation_Error(t *testing.T) {
	s := newFilterTestServer(t)
	err := s.ApplyFilter(ToolFilter{ReadOnly: true, Enable: []string{"mutate_a"}})
	if err == nil {
		t.Fatal("expected error for enabling mutation tool in read-only mode, got nil")
	}
}

// --- Local FS tools tests ---

// setupLocalFSDir creates a temp directory with some files for testing.
func setupLocalFSDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("Hello, world!"), 0644)
	os.WriteFile(filepath.Join(dir, "data.csv"), []byte("a,b,c\n1,2,3"), 0644)
	os.WriteFile(filepath.Join(dir, "subdir", "nested.txt"), []byte("nested content"), 0644)

	return dir
}

// newLocalFSTestServer creates a Server with localFS configured and tools registered.
func newLocalFSTestServer(t *testing.T, dir string) *Server {
	t.Helper()
	s := NewServer(&mcp.Implementation{Name: "localfs-test", Version: "test"}, nil)

	lfs, err := localfs.New([]localfs.Dir{
		{Path: dir, Mode: localfs.ModeReadWrite},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lfs.Close() })
	s.SetLocalFS(lfs)
	RegisterLocalFSTools(s)
	return s
}

// callTool connects and invokes a tool, returning the text content.
func callTool(t *testing.T, s *Server, name string, args map[string]any) string {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })

	result, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}

	var texts []string
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			texts = append(texts, tc.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func TestRegisterLocalFSTools_NilFS(t *testing.T) {
	s := NewServer(&mcp.Implementation{Name: "no-fs-test", Version: "test"}, nil)
	// No localFS set — should be a no-op.
	RegisterLocalFSTools(s)
	got := listToolNames(t, s)
	if len(got) != 0 {
		t.Fatalf("expected 0 tools, got %v", got)
	}
}

func TestRegisterLocalFSTools_Registered(t *testing.T) {
	dir := setupLocalFSDir(t)
	s := newLocalFSTestServer(t, dir)
	got := listToolNames(t, s)
	want := []string{"list_local_files", "read_local_file"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tool[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLocalFSTools_ReadOnly(t *testing.T) {
	dir := setupLocalFSDir(t)
	s := newLocalFSTestServer(t, dir)

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %q should have ReadOnlyHint=true", tool.Name)
		}
	}
}

func TestListLocalFiles_Root(t *testing.T) {
	dir := setupLocalFSDir(t)
	s := newLocalFSTestServer(t, dir)

	result := callTool(t, s, "list_local_files", nil)
	if !strings.Contains(result, "hello.txt") {
		t.Error("expected hello.txt in listing")
	}
	if !strings.Contains(result, "data.csv") {
		t.Error("expected data.csv in listing")
	}
	if !strings.Contains(result, "subdir/") {
		t.Error("expected subdir/ in listing")
	}
}

func TestListLocalFiles_Subdir(t *testing.T) {
	dir := setupLocalFSDir(t)
	s := newLocalFSTestServer(t, dir)

	result := callTool(t, s, "list_local_files", map[string]any{"path": "subdir"})
	if !strings.Contains(result, "nested.txt") {
		t.Error("expected nested.txt in listing")
	}
}

func TestListLocalFiles_DescriptionContainsDirs(t *testing.T) {
	dir := setupLocalFSDir(t)
	s := newLocalFSTestServer(t, dir)

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if tool.Name == "list_local_files" {
			if !strings.Contains(tool.Description, dir) {
				t.Errorf("list_local_files description should contain directory path %q", dir)
			}
			if !strings.Contains(tool.Description, "read-write") {
				t.Error("list_local_files description should contain access mode")
			}
		}
	}
}

func TestReadLocalFile(t *testing.T) {
	dir := setupLocalFSDir(t)
	s := newLocalFSTestServer(t, dir)

	result := callTool(t, s, "read_local_file", map[string]any{"path": "hello.txt"})
	if result != "Hello, world!" {
		t.Errorf("got %q, want %q", result, "Hello, world!")
	}
}

func TestReadLocalFile_Nested(t *testing.T) {
	dir := setupLocalFSDir(t)
	s := newLocalFSTestServer(t, dir)

	result := callTool(t, s, "read_local_file", map[string]any{"path": "subdir/nested.txt"})
	if result != "nested content" {
		t.Errorf("got %q, want %q", result, "nested content")
	}
}

func TestReadLocalFile_Binary(t *testing.T) {
	dir := setupLocalFSDir(t)
	// Write a binary file.
	os.WriteFile(filepath.Join(dir, "binary.bin"), []byte{0x89, 0x50, 0x4E, 0x47, 0x00, 0x0D, 0x0A}, 0644)

	s := newLocalFSTestServer(t, dir)
	result := callTool(t, s, "read_local_file", map[string]any{"path": "binary.bin"})
	if !strings.Contains(result, "Binary file") {
		t.Errorf("expected binary file message, got %q", result)
	}
}

func TestReadLocalFile_EmptyPath(t *testing.T) {
	dir := setupLocalFSDir(t)
	s := newLocalFSTestServer(t, dir)

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })

	result, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "read_local_file",
		Arguments: map[string]any{"path": ""},
	})
	if err != nil {
		// Protocol-level error is also acceptable.
		return
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for empty path")
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
		{"binary", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, false},
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
