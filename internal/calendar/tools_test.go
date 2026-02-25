package calendar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
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
	server := mcp.NewServer(&mcp.Implementation{Name: "test-calendar", Version: "test"}, nil)
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
