package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/bridge"
	"github.com/thegrumpylion/google-mcp/internal/localfs"
	"github.com/thegrumpylion/google-mcp/internal/server"
)

// resolveDriveAttachments fetches Drive files server-side and appends them
// to the composeInput's Attachments slice. The file bytes flow through
// server memory only — they never enter the LLM context window.
func resolveDriveAttachments(ctx context.Context, mgr *auth.Manager, input *composeInput) error {
	for i, da := range input.DriveAttachments {
		if da.DriveAccount == "" {
			return fmt.Errorf("drive_attachments[%d]: drive_account is required", i)
		}
		if da.FileID == "" {
			return fmt.Errorf("drive_attachments[%d]: file_id is required", i)
		}

		result, err := bridge.ReadDriveFile(ctx, mgr, bridge.ReadDriveFileParams{
			DriveAccount: da.DriveAccount,
			FileID:       da.FileID,
		})
		if err != nil {
			return fmt.Errorf("drive_attachments[%d] (%s): %w", i, da.FileID, err)
		}

		input.Attachments = append(input.Attachments, attachment{
			Name:     result.FileName,
			MIMEType: result.MIMEType,
			Content:  base64.StdEncoding.EncodeToString(result.Data),
		})
	}
	return nil
}

// resolveLocalAttachments reads local files and appends them to the
// composeInput's Attachments slice. Files are read from directories
// allowed via --allow-read-dir or --allow-write-dir.
func resolveLocalAttachments(lfs *localfs.FS, input *composeInput) error {
	for i, la := range input.LocalAttachments {
		if la.Path == "" {
			return fmt.Errorf("local_attachments[%d]: path is required", i)
		}

		data, _, err := lfs.ReadFile(la.Path)
		if err != nil {
			return fmt.Errorf("local_attachments[%d] (%s): %w", i, la.Path, err)
		}

		name := filepath.Base(la.Path)
		input.Attachments = append(input.Attachments, attachment{
			Name:     name,
			MIMEType: guessMIMEType(name),
			Content:  base64.StdEncoding.EncodeToString(data),
		})
	}
	return nil
}

// --- save_attachment_to_drive ---

type saveAttachmentToDriveInput struct {
	GmailAccount string `json:"account" jsonschema:"Gmail account name (source)"`
	MessageID    string `json:"message_id" jsonschema:"Gmail message ID that contains the attachment"`
	AttachmentID string `json:"attachment_id" jsonschema:"Attachment ID (from read_message or read_thread results)"`
	DriveAccount string `json:"drive_account" jsonschema:"Drive account name (destination)"`
	FileName     string `json:"file_name" jsonschema:"File name for the saved file (e.g. 'report.pdf')"`
	FolderID     string `json:"folder_id,omitempty" jsonschema:"Drive folder ID to save into (default: root)"`
}

func registerSaveAttachmentToDrive(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name: "save_attachment_to_drive",
		Description: `Save a Gmail attachment directly to Google Drive without downloading it first.

This transfers the file server-side — the attachment data never enters the conversation.
Use read_message to discover attachment IDs, then use this tool to save them to Drive.`,
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: server.BoolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input saveAttachmentToDriveInput) (*mcp.CallToolResult, any, error) {
		result, err := bridge.SaveAttachmentToDrive(ctx, mgr, bridge.SaveAttachmentToDriveParams{
			GmailAccount: input.GmailAccount,
			MessageID:    input.MessageID,
			AttachmentID: input.AttachmentID,
			DriveAccount: input.DriveAccount,
			FileName:     input.FileName,
			FolderID:     input.FolderID,
		})
		if err != nil {
			return nil, nil, err
		}

		text := fmt.Sprintf(
			"Attachment saved to Drive.\n\nFile ID: %s\nName: %s\nMIME Type: %s\nSize: %d bytes",
			result.FileID, result.FileName, result.MIMEType, result.Size,
		)
		if result.WebViewLink != "" {
			text += fmt.Sprintf("\nLink: %s", result.WebViewLink)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, nil, nil
	})
}
