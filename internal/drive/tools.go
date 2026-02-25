// Package drive provides MCP tools for interacting with the Google Drive API.
package drive

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"google.golang.org/api/drive/v3"
)

// Scopes required by the Drive tools.
var Scopes = []string{
	drive.DriveReadonlyScope,
}

// RegisterTools registers all Drive MCP tools on the given server.
func RegisterTools(server *mcp.Server, mgr *auth.Manager) {
	registerAccountsList(server, mgr)
	registerSearch(server, mgr)
	registerList(server, mgr)
	registerGet(server, mgr)
	registerRead(server, mgr)
}

func newService(ctx context.Context, mgr *auth.Manager, account string) (*drive.Service, error) {
	opt, err := mgr.ClientOption(ctx, account, Scopes)
	if err != nil {
		return nil, err
	}
	return drive.NewService(ctx, opt)
}

// --- accounts_list ---

func registerAccountsList(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "accounts_list",
		Description: "List all configured Google accounts. Use this to discover available account names.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
		accounts := mgr.ListAccounts()
		if len(accounts) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "No accounts configured. Run 'google-mcp auth add <name>' to add one."},
				},
			}, nil, nil
		}
		var sb strings.Builder
		sb.WriteString("Configured accounts:\n")
		for name, email := range accounts {
			if email != "" {
				fmt.Fprintf(&sb, "  - %s (%s)\n", name, email)
			} else {
				fmt.Fprintf(&sb, "  - %s\n", name)
			}
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- drive_search ---

type searchInput struct {
	Account    string `json:"account" jsonschema:"Account name (e.g. 'personal', 'work') or 'all' to search all accounts"`
	Query      string `json:"query" jsonschema:"Drive search query (e.g. \"name contains 'report'\" or \"mimeType = 'application/pdf'\")"`
	MaxResults int64  `json:"max_results,omitempty" jsonschema:"Maximum number of results per account (default 10, max 50)"`
}

func registerSearch(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "drive_search",
		Description: "Search Google Drive files using Drive query syntax. Set account to 'all' to search across all accounts. Returns file IDs, names, and metadata.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, any, error) {
		accounts, err := mgr.ResolveAccounts(input.Account)
		if err != nil {
			return nil, nil, err
		}

		maxResults := input.MaxResults
		if maxResults <= 0 {
			maxResults = 10
		}
		if maxResults > 50 {
			maxResults = 50
		}

		var sb strings.Builder
		multiAccount := len(accounts) > 1

		for _, account := range accounts {
			svc, err := newService(ctx, mgr, account)
			if err != nil {
				if multiAccount {
					fmt.Fprintf(&sb, "=== Account: %s ===\nError: %v\n\n", account, err)
					continue
				}
				return nil, nil, fmt.Errorf("creating Drive service: %w", err)
			}

			resp, err := svc.Files.List().
				Q(input.Query).
				PageSize(maxResults).
				Fields("files(id,name,mimeType,size,modifiedTime,owners,webViewLink)").
				Do()
			if err != nil {
				if multiAccount {
					fmt.Fprintf(&sb, "=== Account: %s ===\nError searching: %v\n\n", account, err)
					continue
				}
				return nil, nil, fmt.Errorf("searching files: %w", err)
			}

			if multiAccount {
				fmt.Fprintf(&sb, "=== Account: %s ===\n", account)
			}

			if len(resp.Files) == 0 {
				sb.WriteString("No files found.\n\n")
				continue
			}

			sb.WriteString(formatFileList(resp.Files, account))
		}

		text := sb.String()
		if text == "" {
			text = "No files found."
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, nil, nil
	})
}

// --- drive_list ---

type listInput struct {
	Account    string `json:"account" jsonschema:"Account name (e.g. 'personal', 'work') or 'all' to list from all accounts"`
	FolderID   string `json:"folder_id,omitempty" jsonschema:"Folder ID to list contents of (default: root)"`
	MaxResults int64  `json:"max_results,omitempty" jsonschema:"Maximum number of results per account (default 20, max 100)"`
	OrderBy    string `json:"order_by,omitempty" jsonschema:"Sort order (e.g. 'modifiedTime desc', 'name'). Default: 'modifiedTime desc'"`
}

func registerList(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "drive_list",
		Description: "List files in Google Drive, optionally within a specific folder. Set account to 'all' to list from all accounts. Returns file IDs, names, and metadata.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listInput) (*mcp.CallToolResult, any, error) {
		accounts, err := mgr.ResolveAccounts(input.Account)
		if err != nil {
			return nil, nil, err
		}

		maxResults := input.MaxResults
		if maxResults <= 0 {
			maxResults = 20
		}
		if maxResults > 100 {
			maxResults = 100
		}

		orderBy := input.OrderBy
		if orderBy == "" {
			orderBy = "modifiedTime desc"
		}

		var sb strings.Builder
		multiAccount := len(accounts) > 1

		for _, account := range accounts {
			svc, err := newService(ctx, mgr, account)
			if err != nil {
				if multiAccount {
					fmt.Fprintf(&sb, "=== Account: %s ===\nError: %v\n\n", account, err)
					continue
				}
				return nil, nil, fmt.Errorf("creating Drive service: %w", err)
			}

			call := svc.Files.List().
				PageSize(maxResults).
				OrderBy(orderBy).
				Fields("files(id,name,mimeType,size,modifiedTime,owners,webViewLink)")

			if input.FolderID != "" {
				call = call.Q(fmt.Sprintf("'%s' in parents and trashed = false", input.FolderID))
			} else {
				call = call.Q("trashed = false")
			}

			resp, err := call.Do()
			if err != nil {
				if multiAccount {
					fmt.Fprintf(&sb, "=== Account: %s ===\nError listing: %v\n\n", account, err)
					continue
				}
				return nil, nil, fmt.Errorf("listing files: %w", err)
			}

			if multiAccount {
				fmt.Fprintf(&sb, "=== Account: %s ===\n", account)
			}

			if len(resp.Files) == 0 {
				sb.WriteString("No files found.\n\n")
				continue
			}

			sb.WriteString(formatFileList(resp.Files, account))
		}

		text := sb.String()
		if text == "" {
			text = "No files found."
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, nil, nil
	})
}

// --- drive_get ---

type getInput struct {
	Account string `json:"account" jsonschema:"Account name to use"`
	FileID  string `json:"file_id" jsonschema:"Google Drive file ID"`
}

func registerGet(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "drive_get",
		Description: "Get metadata for a specific Google Drive file by ID.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input getInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		file, err := svc.Files.Get(input.FileID).
			Fields("id,name,mimeType,size,description,modifiedTime,createdTime,owners,parents,webViewLink,webContentLink,exportLinks").
			Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting file: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Name: %s\n", file.Name)
		fmt.Fprintf(&sb, "ID: %s\n", file.Id)
		fmt.Fprintf(&sb, "MIME Type: %s\n", file.MimeType)
		if file.Size > 0 {
			fmt.Fprintf(&sb, "Size: %d bytes\n", file.Size)
		}
		if file.Description != "" {
			fmt.Fprintf(&sb, "Description: %s\n", file.Description)
		}
		fmt.Fprintf(&sb, "Created: %s\n", file.CreatedTime)
		fmt.Fprintf(&sb, "Modified: %s\n", file.ModifiedTime)
		if file.WebViewLink != "" {
			fmt.Fprintf(&sb, "Web Link: %s\n", file.WebViewLink)
		}
		if len(file.Owners) > 0 {
			var owners []string
			for _, o := range file.Owners {
				owners = append(owners, o.DisplayName)
			}
			fmt.Fprintf(&sb, "Owners: %s\n", strings.Join(owners, ", "))
		}
		if len(file.ExportLinks) > 0 {
			sb.WriteString("Export formats:\n")
			for mime := range file.ExportLinks {
				fmt.Fprintf(&sb, "  - %s\n", mime)
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- drive_read ---

type readInput struct {
	Account    string `json:"account" jsonschema:"Account name to use"`
	FileID     string `json:"file_id" jsonschema:"Google Drive file ID"`
	ExportMIME string `json:"export_mime,omitempty" jsonschema:"MIME type to export Google Docs/Sheets/Slides as (e.g. 'text/plain', 'text/csv', 'application/pdf'). Required for Google Workspace files."`
}

func registerRead(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "drive_read",
		Description: "Read/download the content of a Google Drive file. For Google Docs/Sheets/Slides, specify export_mime to choose the export format (e.g. 'text/plain'). Returns text content directly for text files, or base64 for binary files.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input readInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		// First, get file metadata to determine if it's a Google Workspace file.
		file, err := svc.Files.Get(input.FileID).Fields("id,name,mimeType,size").Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting file metadata: %w", err)
		}

		var body io.ReadCloser

		if isGoogleWorkspaceFile(file.MimeType) {
			// Google Workspace files must be exported.
			exportMIME := input.ExportMIME
			if exportMIME == "" {
				exportMIME = defaultExportMIME(file.MimeType)
			}
			resp, err := svc.Files.Export(input.FileID, exportMIME).Download()
			if err != nil {
				return nil, nil, fmt.Errorf("exporting file: %w", err)
			}
			body = resp.Body
		} else {
			resp, err := svc.Files.Get(input.FileID).Download()
			if err != nil {
				return nil, nil, fmt.Errorf("downloading file: %w", err)
			}
			body = resp.Body
		}
		defer body.Close()

		// Read with a size limit to avoid blowing up context.
		const maxSize = 512 * 1024 // 512 KB
		data, err := io.ReadAll(io.LimitReader(body, maxSize+1))
		if err != nil {
			return nil, nil, fmt.Errorf("reading file content: %w", err)
		}

		truncated := len(data) > maxSize
		if truncated {
			data = data[:maxSize]
		}

		// Return as text if it looks like text content.
		text := string(data)
		suffix := ""
		if truncated {
			suffix = "\n\n[Content truncated at 512 KB]"
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("File: %s (%s)\n\n%s%s", file.Name, file.MimeType, text, suffix)},
			},
		}, nil, nil
	})
}

// isGoogleWorkspaceFile returns true if the MIME type is a Google Workspace type
// that requires export rather than direct download.
func isGoogleWorkspaceFile(mimeType string) bool {
	switch mimeType {
	case "application/vnd.google-apps.document",
		"application/vnd.google-apps.spreadsheet",
		"application/vnd.google-apps.presentation",
		"application/vnd.google-apps.drawing",
		"application/vnd.google-apps.script":
		return true
	}
	return false
}

// defaultExportMIME returns the default export MIME type for a Google Workspace file.
func defaultExportMIME(mimeType string) string {
	switch mimeType {
	case "application/vnd.google-apps.document":
		return "text/plain"
	case "application/vnd.google-apps.spreadsheet":
		return "text/csv"
	case "application/vnd.google-apps.presentation":
		return "text/plain"
	case "application/vnd.google-apps.drawing":
		return "image/png"
	case "application/vnd.google-apps.script":
		return "application/vnd.google-apps.script+json"
	default:
		return "text/plain"
	}
}

// formatFileList formats a list of Drive files for display.
// The account parameter is included in each file entry for multi-account context.
func formatFileList(files []*drive.File, account string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d files:\n\n", len(files))
	for _, f := range files {
		fmt.Fprintf(&sb, "- Name: %s\n  ID: %s\n  Account: %s\n  Type: %s\n", f.Name, f.Id, account, f.MimeType)
		if f.Size > 0 {
			fmt.Fprintf(&sb, "  Size: %d bytes\n", f.Size)
		}
		if f.ModifiedTime != "" {
			fmt.Fprintf(&sb, "  Modified: %s\n", f.ModifiedTime)
		}
		if f.WebViewLink != "" {
			fmt.Fprintf(&sb, "  Link: %s\n", f.WebViewLink)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// AccountScopes returns the scopes used by Drive tools.
func AccountScopes() []string {
	return Scopes
}
