package server

import (
	"context"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
