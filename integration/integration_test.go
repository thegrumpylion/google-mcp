// Package integration contains end-to-end tests that launch the google-mcp
// binary and interact with it via the MCP protocol over stdio.
//
// These tests require:
//   - Valid credentials at ~/.config/google-mcp/credentials.json
//   - At least one configured account in ~/.config/google-mcp/tokens.json
//
// Tests are automatically skipped when credentials or accounts are not available.
// Use -short to skip integration tests explicitly.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	binaryPath string
	configDir  string
)

// TestMain builds the binary once and determines the config directory.
func TestMain(m *testing.M) {
	// Determine config dir.
	configDir = os.Getenv("GOOGLE_MCP_CONFIG_DIR")
	if configDir == "" {
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				fmt.Fprintf(os.Stderr, "cannot determine home dir: %v\n", err)
				os.Exit(1)
			}
			xdg = filepath.Join(home, ".config")
		}
		configDir = filepath.Join(xdg, "google-mcp")
	}

	// Build binary to a temp location.
	tmp, err := os.MkdirTemp("", "google-mcp-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot create temp dir: %v\n", err)
		os.Exit(1)
	}
	binaryPath = filepath.Join(tmp, "google-mcp")

	build := exec.Command("go", "build", "-o", binaryPath, "..")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "cannot build binary: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmp)
	os.Exit(code)
}

// skipIfNoCredentials skips the test if credentials or accounts aren't available.
func skipIfNoCredentials(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	credsPath := filepath.Join(configDir, "credentials.json")
	if _, err := os.Stat(credsPath); err != nil {
		t.Skipf("skipping: credentials.json not found at %s", credsPath)
	}

	tokensPath := filepath.Join(configDir, "tokens.json")
	data, err := os.ReadFile(tokensPath)
	if err != nil {
		t.Skipf("skipping: tokens.json not found at %s", tokensPath)
	}

	var cfg struct {
		Accounts map[string]json.RawMessage `json:"accounts"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Skipf("skipping: cannot parse tokens.json: %v", err)
	}
	if len(cfg.Accounts) == 0 {
		t.Skip("skipping: no accounts configured")
	}
}

// firstAccount returns the name of the first account in tokens.json.
func firstAccount(t *testing.T) string {
	t.Helper()
	tokensPath := filepath.Join(configDir, "tokens.json")
	data, err := os.ReadFile(tokensPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Accounts map[string]json.RawMessage `json:"accounts"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	for name := range cfg.Accounts {
		return name
	}
	t.Fatal("no accounts found")
	return ""
}

// accountCount returns the number of configured accounts.
func accountCount(t *testing.T) int {
	t.Helper()
	tokensPath := filepath.Join(configDir, "tokens.json")
	data, err := os.ReadFile(tokensPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Accounts map[string]json.RawMessage `json:"accounts"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	return len(cfg.Accounts)
}

// connectServer starts the MCP server with the given subcommand and returns
// a connected session. The caller should defer session.Close().
func connectServer(ctx context.Context, t *testing.T, subcommand string) *mcp.ClientSession {
	t.Helper()

	cmd := exec.Command(binaryPath, subcommand, "--config-dir", configDir)
	client := mcp.NewClient(&mcp.Implementation{Name: "integration-test", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connecting to %s server: %v", subcommand, err)
	}
	return session
}

// textContent extracts the text from the first TextContent in a result.
func textContent(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	t.Fatal("no text content in result")
	return ""
}

// --- Gmail integration tests ---

func TestGmail_ToolsList(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "gmail")
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	expectedTools := []string{
		"accounts_list", "gmail_search", "gmail_read", "gmail_read_thread",
		"gmail_send", "gmail_list_labels", "gmail_modify", "gmail_get_attachment",
		"gmail_draft_create", "gmail_draft_list", "gmail_draft_send",
	}

	if len(tools.Tools) != len(expectedTools) {
		t.Errorf("got %d tools, want %d", len(tools.Tools), len(expectedTools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools.Tools {
		toolNames[tool.Name] = true
	}
	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestGmail_AccountsList(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "gmail")
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "accounts_list"})
	if err != nil {
		t.Fatalf("accounts_list: %v", err)
	}

	text := textContent(t, result)
	if !strings.Contains(text, "Configured accounts:") {
		t.Errorf("unexpected output: %s", text)
	}

	// Should list at least one account.
	count := accountCount(t)
	lines := strings.Count(text, "  - ")
	if lines < count {
		t.Errorf("listed %d accounts, want at least %d", lines, count)
	}
}

func TestGmail_Search_SingleAccount(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "gmail")
	defer session.Close()

	account := firstAccount(t)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gmail_search",
		Arguments: map[string]any{
			"account":     account,
			"query":       "newer_than:7d",
			"max_results": 1,
		},
	})
	if err != nil {
		t.Fatalf("gmail_search: %v", err)
	}

	text := textContent(t, result)
	// Should either find messages or say "No messages found."
	if !strings.Contains(text, "ID:") && !strings.Contains(text, "No messages found") {
		t.Errorf("unexpected search output: %s", text)
	}
}

func TestGmail_Search_AllAccounts(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "gmail")
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gmail_search",
		Arguments: map[string]any{
			"account":     "all",
			"query":       "newer_than:7d",
			"max_results": 1,
		},
	})
	if err != nil {
		t.Fatalf("gmail_search (all): %v", err)
	}

	text := textContent(t, result)

	// With multiple accounts, should contain "=== Account:" headers.
	count := accountCount(t)
	if count > 1 {
		headers := strings.Count(text, "=== Account:")
		if headers < count {
			t.Errorf("got %d account headers, want at least %d", headers, count)
		}
	}
}

func TestGmail_ReadMessage(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "gmail")
	defer session.Close()

	account := firstAccount(t)

	// First, search for a message.
	searchResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gmail_search",
		Arguments: map[string]any{
			"account":     account,
			"query":       "newer_than:30d",
			"max_results": 1,
		},
	})
	if err != nil {
		t.Fatalf("gmail_search: %v", err)
	}

	searchText := textContent(t, searchResult)
	if strings.Contains(searchText, "No messages found") {
		t.Skip("no messages to read")
	}

	// Extract the message ID from "ID: <id>".
	msgID := extractID(searchText)
	if msgID == "" {
		t.Fatal("could not extract message ID from search results")
	}

	// Read the message.
	readResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gmail_read",
		Arguments: map[string]any{
			"account":    account,
			"message_id": msgID,
		},
	})
	if err != nil {
		t.Fatalf("gmail_read: %v", err)
	}

	text := textContent(t, readResult)
	if !strings.Contains(text, "Thread ID:") {
		t.Errorf("gmail_read missing Thread ID in output: %s", text[:min(len(text), 200)])
	}
	if !strings.Contains(text, "From:") {
		t.Errorf("gmail_read missing From header in output")
	}
}

func TestGmail_ReadThread(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "gmail")
	defer session.Close()

	account := firstAccount(t)

	// Search for a message to get a thread ID.
	searchResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gmail_search",
		Arguments: map[string]any{
			"account":     account,
			"query":       "newer_than:30d",
			"max_results": 1,
		},
	})
	if err != nil {
		t.Fatalf("gmail_search: %v", err)
	}

	searchText := textContent(t, searchResult)
	if strings.Contains(searchText, "No messages found") {
		t.Skip("no messages for thread test")
	}

	msgID := extractID(searchText)
	if msgID == "" {
		t.Fatal("could not extract message ID")
	}

	// Read the message to get the thread ID.
	readResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gmail_read",
		Arguments: map[string]any{
			"account":    account,
			"message_id": msgID,
		},
	})
	if err != nil {
		t.Fatalf("gmail_read: %v", err)
	}

	readText := textContent(t, readResult)
	threadID := extractAfter(readText, "Thread ID: ")
	if threadID == "" {
		t.Fatal("could not extract thread ID from gmail_read output")
	}

	// Read the thread.
	threadResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gmail_read_thread",
		Arguments: map[string]any{
			"account":   account,
			"thread_id": threadID,
		},
	})
	if err != nil {
		t.Fatalf("gmail_read_thread: %v", err)
	}

	threadText := textContent(t, threadResult)
	if !strings.Contains(threadText, "Thread ID:") {
		t.Errorf("gmail_read_thread missing Thread ID")
	}
	if !strings.Contains(threadText, "Messages:") {
		t.Errorf("gmail_read_thread missing Messages count")
	}
}

func TestGmail_ListLabels(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "gmail")
	defer session.Close()

	account := firstAccount(t)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gmail_list_labels",
		Arguments: map[string]any{
			"account": account,
		},
	})
	if err != nil {
		t.Fatalf("gmail_list_labels: %v", err)
	}

	text := textContent(t, result)
	// Every Gmail account has INBOX.
	if !strings.Contains(text, "INBOX") {
		t.Errorf("gmail_list_labels missing INBOX label")
	}
}

func TestGmail_ListLabels_AllAccounts(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "gmail")
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gmail_list_labels",
		Arguments: map[string]any{
			"account": "all",
		},
	})
	if err != nil {
		t.Fatalf("gmail_list_labels (all): %v", err)
	}

	text := textContent(t, result)
	count := accountCount(t)
	if count > 1 {
		headers := strings.Count(text, "=== Account:")
		if headers < count {
			t.Errorf("got %d account headers, want at least %d", headers, count)
		}
	}
}

func TestGmail_DraftList(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "gmail")
	defer session.Close()

	account := firstAccount(t)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gmail_draft_list",
		Arguments: map[string]any{
			"account":     account,
			"max_results": 5,
		},
	})
	if err != nil {
		t.Fatalf("gmail_draft_list: %v", err)
	}

	text := textContent(t, result)
	// Should return either drafts or "No drafts found."
	if !strings.Contains(text, "Draft ID:") && !strings.Contains(text, "No drafts found") {
		t.Errorf("unexpected draft list output: %s", text)
	}
}

// --- Drive integration tests ---

func TestDrive_ToolsList(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "drive")
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	expectedTools := []string{
		"accounts_list", "drive_search", "drive_list", "drive_get", "drive_read",
	}

	if len(tools.Tools) != len(expectedTools) {
		t.Errorf("got %d tools, want %d", len(tools.Tools), len(expectedTools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools.Tools {
		toolNames[tool.Name] = true
	}
	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestDrive_Search(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "drive")
	defer session.Close()

	account := firstAccount(t)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "drive_search",
		Arguments: map[string]any{
			"account":     account,
			"query":       "trashed = false",
			"max_results": 2,
		},
	})
	if err != nil {
		t.Fatalf("drive_search: %v", err)
	}

	text := textContent(t, result)
	if !strings.Contains(text, "ID:") && !strings.Contains(text, "No files found") {
		t.Errorf("unexpected drive_search output: %s", text)
	}
}

func TestDrive_List(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "drive")
	defer session.Close()

	account := firstAccount(t)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "drive_list",
		Arguments: map[string]any{
			"account":     account,
			"max_results": 2,
		},
	})
	if err != nil {
		t.Fatalf("drive_list: %v", err)
	}

	text := textContent(t, result)
	if !strings.Contains(text, "ID:") && !strings.Contains(text, "No files found") {
		t.Errorf("unexpected drive_list output: %s", text)
	}
}

func TestDrive_GetAndRead(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "drive")
	defer session.Close()

	account := firstAccount(t)

	// First, find a file.
	listResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "drive_list",
		Arguments: map[string]any{
			"account":     account,
			"max_results": 1,
		},
	})
	if err != nil {
		t.Fatalf("drive_list: %v", err)
	}

	listText := textContent(t, listResult)
	if strings.Contains(listText, "No files found") {
		t.Skip("no files to test drive_get/drive_read")
	}

	fileID := extractAfter(listText, "ID: ")
	if fileID == "" {
		t.Fatal("could not extract file ID from drive_list output")
	}

	// Get file metadata.
	getResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "drive_get",
		Arguments: map[string]any{
			"account": account,
			"file_id": fileID,
		},
	})
	if err != nil {
		t.Fatalf("drive_get: %v", err)
	}

	getText := textContent(t, getResult)
	if !strings.Contains(getText, "Name:") {
		t.Errorf("drive_get missing Name in output")
	}
	if !strings.Contains(getText, "MIME Type:") {
		t.Errorf("drive_get missing MIME Type in output")
	}
}

func TestDrive_Search_AllAccounts(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "drive")
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "drive_search",
		Arguments: map[string]any{
			"account":     "all",
			"query":       "trashed = false",
			"max_results": 1,
		},
	})
	if err != nil {
		t.Fatalf("drive_search (all): %v", err)
	}

	text := textContent(t, result)
	count := accountCount(t)
	if count > 1 {
		headers := strings.Count(text, "=== Account:")
		if headers < count {
			t.Errorf("got %d account headers, want at least %d", headers, count)
		}
	}
}

// --- Calendar integration tests ---

func TestCalendar_ToolsList(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "calendar")
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	expectedTools := []string{
		"accounts_list", "calendar_list_calendars", "calendar_list_events",
		"calendar_get_event", "calendar_create_event", "calendar_update_event",
		"calendar_delete_event", "calendar_respond_event",
	}

	if len(tools.Tools) != len(expectedTools) {
		t.Errorf("got %d tools, want %d", len(tools.Tools), len(expectedTools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools.Tools {
		toolNames[tool.Name] = true
	}
	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestCalendar_ListCalendars(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "calendar")
	defer session.Close()

	account := firstAccount(t)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "calendar_list_calendars",
		Arguments: map[string]any{
			"account": account,
		},
	})
	if err != nil {
		t.Fatalf("calendar_list_calendars: %v", err)
	}

	text := textContent(t, result)
	if !strings.Contains(text, "Found") || !strings.Contains(text, "calendars") {
		t.Errorf("unexpected calendar_list_calendars output: %s", text[:min(len(text), 200)])
	}
}

func TestCalendar_ListEvents(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "calendar")
	defer session.Close()

	account := firstAccount(t)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "calendar_list_events",
		Arguments: map[string]any{
			"account":     account,
			"max_results": 3,
		},
	})
	if err != nil {
		t.Fatalf("calendar_list_events: %v", err)
	}

	text := textContent(t, result)
	// Should either list events or say no events found.
	if !strings.Contains(text, "Found") && !strings.Contains(text, "No events found") {
		t.Errorf("unexpected calendar_list_events output: %s", text[:min(len(text), 200)])
	}
}

func TestCalendar_GetEvent(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "calendar")
	defer session.Close()

	account := firstAccount(t)

	// First, list events to find one.
	listResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "calendar_list_events",
		Arguments: map[string]any{
			"account":     account,
			"max_results": 1,
		},
	})
	if err != nil {
		t.Fatalf("calendar_list_events: %v", err)
	}

	listText := textContent(t, listResult)
	if strings.Contains(listText, "No events found") {
		t.Skip("no events to test calendar_get_event")
	}

	eventID := extractAfter(listText, "ID: ")
	if eventID == "" {
		t.Fatal("could not extract event ID from calendar_list_events output")
	}

	// Get the event.
	getResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "calendar_get_event",
		Arguments: map[string]any{
			"account":  account,
			"event_id": eventID,
		},
	})
	if err != nil {
		t.Fatalf("calendar_get_event: %v", err)
	}

	getText := textContent(t, getResult)
	if !strings.Contains(getText, "Event:") {
		t.Errorf("calendar_get_event missing Event header")
	}
	if !strings.Contains(getText, eventID) {
		t.Errorf("calendar_get_event missing event ID in output")
	}
}

func TestCalendar_ListCalendars_AllAccounts(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	session := connectServer(ctx, t, "calendar")
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "calendar_list_calendars",
		Arguments: map[string]any{
			"account": "all",
		},
	})
	if err != nil {
		t.Fatalf("calendar_list_calendars (all): %v", err)
	}

	text := textContent(t, result)
	count := accountCount(t)
	if count > 1 {
		headers := strings.Count(text, "=== Account:")
		if headers < count {
			t.Errorf("got %d account headers, want at least %d", headers, count)
		}
	}
}

// --- helpers ---

// extractID extracts the first "ID: <value>" from text.
func extractID(text string) string {
	return extractAfter(text, "ID: ")
}

// extractAfter extracts the value after the first occurrence of prefix,
// up to the next newline.
func extractAfter(text, prefix string) string {
	idx := strings.Index(text, prefix)
	if idx == -1 {
		return ""
	}
	start := idx + len(prefix)
	end := strings.Index(text[start:], "\n")
	if end == -1 {
		return strings.TrimSpace(text[start:])
	}
	return strings.TrimSpace(text[start : start+end])
}
