// Package drive provides MCP tools for interacting with the Google Drive API.
package drive

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"google.golang.org/api/drive/v3"
)

// Scopes required by the Drive tools.
var Scopes = []string{
	drive.DriveScope,
}

// RegisterTools registers all Drive MCP tools on the given server.
func RegisterTools(server *mcp.Server, mgr *auth.Manager) {
	registerAccountsList(server, mgr)
	registerSearch(server, mgr)
	registerList(server, mgr)
	registerGet(server, mgr)
	registerRead(server, mgr)
	registerUpload(server, mgr)
	registerUpdate(server, mgr)
	registerDelete(server, mgr)
	registerCreateFolder(server, mgr)
	registerMove(server, mgr)
	registerCopy(server, mgr)
	registerShare(server, mgr)
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
		Name:        "search",
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
		Name:        "list",
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
		Name:        "get",
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
		Name:        "read",
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

// --- drive_upload ---

type uploadInput struct {
	Account  string `json:"account" jsonschema:"Account name to use"`
	Name     string `json:"name" jsonschema:"File name (e.g. 'report.txt')"`
	Content  string `json:"content" jsonschema:"File content as text, or base64-encoded binary data"`
	MIMEType string `json:"mime_type,omitempty" jsonschema:"MIME type of the file (e.g. 'text/plain', 'application/pdf'). Auto-detected if omitted."`
	FolderID string `json:"folder_id,omitempty" jsonschema:"Parent folder ID to upload into (default: root)"`
	Base64   bool   `json:"base64,omitempty" jsonschema:"Set to true if content is base64-encoded binary data"`
}

func registerUpload(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "upload",
		Description: `Upload a new file to Google Drive. Provide content as plain text or base64-encoded binary.

For text files, just pass the content directly. For binary files, base64-encode the content and set base64=true.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input uploadInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		file := &drive.File{Name: input.Name}
		if input.MIMEType != "" {
			file.MimeType = input.MIMEType
		}
		if input.FolderID != "" {
			file.Parents = []string{input.FolderID}
		}

		var reader io.Reader
		if input.Base64 {
			data, err := base64.StdEncoding.DecodeString(input.Content)
			if err != nil {
				return nil, nil, fmt.Errorf("decoding base64 content: %w", err)
			}
			reader = bytes.NewReader(data)
		} else {
			reader = strings.NewReader(input.Content)
		}

		created, err := svc.Files.Create(file).Media(reader).
			Fields("id,name,mimeType,size,webViewLink").Do()
		if err != nil {
			return nil, nil, fmt.Errorf("uploading file: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "File uploaded successfully!\n\n")
		fmt.Fprintf(&sb, "Name: %s\n", created.Name)
		fmt.Fprintf(&sb, "ID: %s\n", created.Id)
		fmt.Fprintf(&sb, "MIME Type: %s\n", created.MimeType)
		if created.Size > 0 {
			fmt.Fprintf(&sb, "Size: %d bytes\n", created.Size)
		}
		if created.WebViewLink != "" {
			fmt.Fprintf(&sb, "Link: %s\n", created.WebViewLink)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- drive_update ---

type updateInput struct {
	Account     string `json:"account" jsonschema:"Account name to use"`
	FileID      string `json:"file_id" jsonschema:"Google Drive file ID to update"`
	Name        string `json:"name,omitempty" jsonschema:"New file name (leave empty to keep current)"`
	Description string `json:"description,omitempty" jsonschema:"New file description (leave empty to keep current)"`
	MIMEType    string `json:"mime_type,omitempty" jsonschema:"New MIME type (leave empty to keep current)"`
}

func registerUpdate(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update",
		Description: "Update file metadata on Google Drive (rename, change description, change MIME type). Only specified fields are changed.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input updateInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		file := &drive.File{}
		if input.Name != "" {
			file.Name = input.Name
		}
		if input.Description != "" {
			file.Description = input.Description
		}
		if input.MIMEType != "" {
			file.MimeType = input.MIMEType
		}

		updated, err := svc.Files.Update(input.FileID, file).
			Fields("id,name,mimeType,size,description,modifiedTime,webViewLink").Do()
		if err != nil {
			return nil, nil, fmt.Errorf("updating file: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "File updated successfully!\n\n")
		fmt.Fprintf(&sb, "Name: %s\n", updated.Name)
		fmt.Fprintf(&sb, "ID: %s\n", updated.Id)
		fmt.Fprintf(&sb, "MIME Type: %s\n", updated.MimeType)
		if updated.Description != "" {
			fmt.Fprintf(&sb, "Description: %s\n", updated.Description)
		}
		if updated.WebViewLink != "" {
			fmt.Fprintf(&sb, "Link: %s\n", updated.WebViewLink)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- drive_delete ---

type deleteInput struct {
	Account     string `json:"account" jsonschema:"Account name to use"`
	FileID      string `json:"file_id" jsonschema:"Google Drive file ID to delete"`
	Permanently bool   `json:"permanently,omitempty" jsonschema:"If true, permanently delete instead of moving to trash (default: false, moves to trash)"`
}

func registerDelete(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "delete",
		Description: `Delete a file from Google Drive. By default, moves the file to trash.

Set permanently=true to permanently delete the file (cannot be undone).`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input deleteInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		if input.Permanently {
			if err := svc.Files.Delete(input.FileID).Do(); err != nil {
				return nil, nil, fmt.Errorf("deleting file: %w", err)
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("File %s permanently deleted.", input.FileID)},
				},
			}, nil, nil
		}

		// Move to trash by updating the trashed property.
		_, err = svc.Files.Update(input.FileID, &drive.File{
			Trashed:         true,
			ForceSendFields: []string{"Trashed"},
		}).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("trashing file: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("File %s moved to trash.", input.FileID)},
			},
		}, nil, nil
	})
}

// --- drive_create_folder ---

type createFolderInput struct {
	Account  string `json:"account" jsonschema:"Account name to use"`
	Name     string `json:"name" jsonschema:"Folder name"`
	ParentID string `json:"parent_id,omitempty" jsonschema:"Parent folder ID (default: root)"`
}

func registerCreateFolder(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_folder",
		Description: "Create a new folder in Google Drive, optionally inside an existing folder.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input createFolderInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		folder := &drive.File{
			Name:     input.Name,
			MimeType: "application/vnd.google-apps.folder",
		}
		if input.ParentID != "" {
			folder.Parents = []string{input.ParentID}
		}

		created, err := svc.Files.Create(folder).Fields("id,name,webViewLink").Do()
		if err != nil {
			return nil, nil, fmt.Errorf("creating folder: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Folder created successfully!\n\n")
		fmt.Fprintf(&sb, "Name: %s\n", created.Name)
		fmt.Fprintf(&sb, "ID: %s\n", created.Id)
		if created.WebViewLink != "" {
			fmt.Fprintf(&sb, "Link: %s\n", created.WebViewLink)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- drive_move ---

type moveInput struct {
	Account      string `json:"account" jsonschema:"Account name to use"`
	FileID       string `json:"file_id" jsonschema:"Google Drive file ID to move"`
	DestFolderID string `json:"dest_folder_id" jsonschema:"Destination folder ID"`
}

func registerMove(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "move",
		Description: "Move a file to a different folder in Google Drive.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input moveInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		// Get current parents to remove them.
		file, err := svc.Files.Get(input.FileID).Fields("parents").Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting file parents: %w", err)
		}

		previousParents := strings.Join(file.Parents, ",")

		updated, err := svc.Files.Update(input.FileID, &drive.File{}).
			AddParents(input.DestFolderID).
			RemoveParents(previousParents).
			Fields("id,name,parents,webViewLink").Do()
		if err != nil {
			return nil, nil, fmt.Errorf("moving file: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("File moved successfully!\n\nName: %s\nID: %s\nNew parent: %s\nLink: %s",
					updated.Name, updated.Id, input.DestFolderID, updated.WebViewLink)},
			},
		}, nil, nil
	})
}

// --- drive_copy ---

type copyInput struct {
	Account  string `json:"account" jsonschema:"Account name to use"`
	FileID   string `json:"file_id" jsonschema:"Google Drive file ID to copy"`
	Name     string `json:"name,omitempty" jsonschema:"Name for the copy (default: 'Copy of <original>')"`
	FolderID string `json:"folder_id,omitempty" jsonschema:"Destination folder ID for the copy (default: same folder)"`
}

func registerCopy(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "copy",
		Description: "Create a copy of a file in Google Drive, optionally with a new name or in a different folder.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input copyInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		copyFile := &drive.File{}
		if input.Name != "" {
			copyFile.Name = input.Name
		}
		if input.FolderID != "" {
			copyFile.Parents = []string{input.FolderID}
		}

		copied, err := svc.Files.Copy(input.FileID, copyFile).
			Fields("id,name,mimeType,size,webViewLink").Do()
		if err != nil {
			return nil, nil, fmt.Errorf("copying file: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "File copied successfully!\n\n")
		fmt.Fprintf(&sb, "Name: %s\n", copied.Name)
		fmt.Fprintf(&sb, "ID: %s\n", copied.Id)
		fmt.Fprintf(&sb, "MIME Type: %s\n", copied.MimeType)
		if copied.Size > 0 {
			fmt.Fprintf(&sb, "Size: %d bytes\n", copied.Size)
		}
		if copied.WebViewLink != "" {
			fmt.Fprintf(&sb, "Link: %s\n", copied.WebViewLink)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- drive_share ---

type shareInput struct {
	Account      string `json:"account" jsonschema:"Account name to use"`
	FileID       string `json:"file_id" jsonschema:"Google Drive file ID to share"`
	EmailAddress string `json:"email_address,omitempty" jsonschema:"Email address to share with (required for 'user' and 'group' types)"`
	Role         string `json:"role" jsonschema:"Permission role: 'reader', 'commenter', 'writer', or 'organizer'"`
	Type         string `json:"type" jsonschema:"Permission type: 'user', 'group', 'domain', or 'anyone'"`
	Domain       string `json:"domain,omitempty" jsonschema:"Domain to share with (required for 'domain' type)"`
	SendEmail    bool   `json:"send_email,omitempty" jsonschema:"Send a notification email to the user (default: false)"`
	Message      string `json:"message,omitempty" jsonschema:"Custom message to include in the notification email"`
}

func registerShare(server *mcp.Server, mgr *auth.Manager) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "share",
		Description: `Share a Google Drive file by adding permissions.

Types:
  - "user" — Share with a specific user (requires email_address)
  - "group" — Share with a Google Group (requires email_address)
  - "domain" — Share with an entire domain (requires domain)
  - "anyone" — Share with anyone who has the link

Roles:
  - "reader" — View only
  - "commenter" — View and comment
  - "writer" — Edit
  - "organizer" — Manage (shared drives only)`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input shareInput) (*mcp.CallToolResult, any, error) {
		// Validate inputs.
		switch input.Type {
		case "user", "group":
			if input.EmailAddress == "" {
				return nil, nil, fmt.Errorf("email_address is required for type %q", input.Type)
			}
		case "domain":
			if input.Domain == "" {
				return nil, nil, fmt.Errorf("domain is required for type 'domain'")
			}
		case "anyone":
		default:
			return nil, nil, fmt.Errorf("invalid type %q: must be 'user', 'group', 'domain', or 'anyone'", input.Type)
		}

		switch input.Role {
		case "reader", "commenter", "writer", "organizer":
		default:
			return nil, nil, fmt.Errorf("invalid role %q: must be 'reader', 'commenter', 'writer', or 'organizer'", input.Role)
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Drive service: %w", err)
		}

		perm := &drive.Permission{
			Role:         input.Role,
			Type:         input.Type,
			EmailAddress: input.EmailAddress,
			Domain:       input.Domain,
		}

		call := svc.Permissions.Create(input.FileID, perm).
			Fields("id,role,type,emailAddress,domain")

		if input.SendEmail {
			call = call.SendNotificationEmail(true)
			if input.Message != "" {
				call = call.EmailMessage(input.Message)
			}
		} else {
			call = call.SendNotificationEmail(false)
		}

		created, err := call.Do()
		if err != nil {
			return nil, nil, fmt.Errorf("sharing file: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "File shared successfully!\n\n")
		fmt.Fprintf(&sb, "Permission ID: %s\n", created.Id)
		fmt.Fprintf(&sb, "Role: %s\n", created.Role)
		fmt.Fprintf(&sb, "Type: %s\n", created.Type)
		if created.EmailAddress != "" {
			fmt.Fprintf(&sb, "Shared with: %s\n", created.EmailAddress)
		}
		if created.Domain != "" {
			fmt.Fprintf(&sb, "Domain: %s\n", created.Domain)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
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
