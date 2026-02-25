package calendar

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
	calendarapi "google.golang.org/api/calendar/v3"
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
	server := server.NewServer(&mcp.Implementation{Name: "test-calendar", Version: "test"}, nil)
	RegisterTools(server, mgr)
}

func TestIsDateOnly(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"2024-01-15", true},
		{"2024-12-31", true},
		{"2024-01-15T09:00:00-05:00", false},
		{"2024-01-15T00:00:00Z", false},
		{"not-a-date", false},
		{"", false},
		{"2024-1-5", false}, // wrong format
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isDateOnly(tt.input)
			if got != tt.want {
				t.Errorf("isDateOnly(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatEvent(t *testing.T) {
	event := &calendarapi.Event{
		Summary:  "Team Standup",
		Id:       "event-123",
		Location: "Room A",
		Status:   "confirmed",
		Start: &calendarapi.EventDateTime{
			DateTime: "2024-01-15T09:00:00-05:00",
		},
		End: &calendarapi.EventDateTime{
			DateTime: "2024-01-15T09:30:00-05:00",
		},
	}

	result := formatEvent(event, "work")

	if !strings.Contains(result, "Team Standup") {
		t.Error("result should contain event summary")
	}
	if !strings.Contains(result, "event-123") {
		t.Error("result should contain event ID")
	}
	if !strings.Contains(result, "Account: work") {
		t.Error("result should contain account name")
	}
	if !strings.Contains(result, "Room A") {
		t.Error("result should contain location")
	}
	if !strings.Contains(result, "2024-01-15T09:00:00") {
		t.Error("result should contain start time")
	}
	if !strings.Contains(result, "2024-01-15T09:30:00") {
		t.Error("result should contain end time")
	}
}

func TestFormatEvent_AllDay(t *testing.T) {
	event := &calendarapi.Event{
		Summary: "Holiday",
		Id:      "event-456",
		Start:   &calendarapi.EventDateTime{Date: "2024-12-25"},
		End:     &calendarapi.EventDateTime{Date: "2024-12-26"},
	}

	result := formatEvent(event, "personal")

	if !strings.Contains(result, "all day") {
		t.Error("result should indicate all-day event")
	}
	if !strings.Contains(result, "2024-12-25") {
		t.Error("result should contain start date")
	}
}

func TestFormatEventDetailed(t *testing.T) {
	event := &calendarapi.Event{
		Summary:     "Project Review",
		Id:          "event-789",
		Description: "Quarterly project review",
		Location:    "Conference Room B",
		Status:      "confirmed",
		HtmlLink:    "https://calendar.google.com/event?id=event-789",
		Start:       &calendarapi.EventDateTime{DateTime: "2024-01-15T14:00:00-05:00"},
		End:         &calendarapi.EventDateTime{DateTime: "2024-01-15T15:00:00-05:00"},
		Creator:     &calendarapi.EventCreator{Email: "creator@example.com"},
		Organizer:   &calendarapi.EventOrganizer{Email: "organizer@example.com"},
		Attendees: []*calendarapi.EventAttendee{
			{Email: "alice@example.com", DisplayName: "Alice", ResponseStatus: "accepted"},
			{Email: "bob@example.com", ResponseStatus: "tentative"},
		},
		Recurrence: []string{"RRULE:FREQ=WEEKLY;BYDAY=MO"},
	}

	result := formatEventDetailed(event)

	checks := []struct {
		label string
		want  string
	}{
		{"summary", "Project Review"},
		{"ID", "event-789"},
		{"description", "Quarterly project review"},
		{"location", "Conference Room B"},
		{"status", "confirmed"},
		{"link", "https://calendar.google.com/event?id=event-789"},
		{"creator", "creator@example.com"},
		{"organizer", "organizer@example.com"},
		{"attendee name", "Alice"},
		{"attendee status", "accepted"},
		{"attendee email fallback", "bob@example.com"},
		{"recurrence", "RRULE:FREQ=WEEKLY;BYDAY=MO"},
	}

	for _, check := range checks {
		if !strings.Contains(result, check.want) {
			t.Errorf("result missing %s: want substring %q in:\n%s", check.label, check.want, result)
		}
	}
}

func TestFormatEventDetailed_Minimal(t *testing.T) {
	event := &calendarapi.Event{
		Summary: "Quick Chat",
		Id:      "event-min",
	}

	result := formatEventDetailed(event)

	if !strings.Contains(result, "Quick Chat") {
		t.Error("result should contain summary")
	}
	if !strings.Contains(result, "event-min") {
		t.Error("result should contain ID")
	}
	// Should not panic or include empty sections.
	if strings.Contains(result, "Attendees:") {
		t.Error("result should not contain Attendees section when empty")
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
	server := server.NewServer(&mcp.Implementation{Name: "test-calendar", Version: "test"}, nil)
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
		"create_event",
		"delete_event",
		"get_event",
		"list_accounts",
		"list_calendars",
		"list_events",
		"respond_event",
		"update_event",
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
		"list_accounts", "list_calendars", "list_events", "get_event",
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
		"create_event", "update_event", "delete_event", "respond_event",
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

	srv := server.NewServer(&mcp.Implementation{Name: "test-calendar", Version: "test"}, nil)
	srv.SetLocalFS(lfs)
	RegisterTools(srv, mgr)

	tools := listTools(t, srv)
	got := make([]string, 0, len(tools))
	for _, tool := range tools {
		got = append(got, tool.Name)
	}
	sort.Strings(got)

	// Should include all 8 base tools + 2 localfs tools = 10.
	if len(got) != 10 {
		t.Fatalf("got %d tools, want 10\ngot: %v", len(got), got)
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
