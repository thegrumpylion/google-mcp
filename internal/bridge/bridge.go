// Package bridge provides cross-service functions that transfer data between
// Google APIs server-side, avoiding the need to round-trip file content through
// the LLM's context window.
package bridge

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/thegrumpylion/google-mcp/internal/auth"
	driveapi "google.golang.org/api/drive/v3"
	gmailapi "google.golang.org/api/gmail/v1"
)

// Gmail scopes needed by bridge functions.
var GmailScopes = []string{gmailapi.MailGoogleComScope}

// Drive scopes needed by bridge functions.
var DriveScopes = []string{driveapi.DriveScope}

func newGmailService(ctx context.Context, mgr *auth.Manager, account string) (*gmailapi.Service, error) {
	opt, err := mgr.ClientOption(ctx, account, GmailScopes)
	if err != nil {
		return nil, err
	}
	return gmailapi.NewService(ctx, opt)
}

func newDriveService(ctx context.Context, mgr *auth.Manager, account string) (*driveapi.Service, error) {
	opt, err := mgr.ClientOption(ctx, account, DriveScopes)
	if err != nil {
		return nil, err
	}
	return driveapi.NewService(ctx, opt)
}

// SaveAttachmentToDriveParams holds the parameters for SaveAttachmentToDrive.
type SaveAttachmentToDriveParams struct {
	GmailAccount string
	MessageID    string
	AttachmentID string
	DriveAccount string
	FileName     string // If empty, must be provided by caller.
	FolderID     string // Optional destination folder.
}

// SaveAttachmentToDriveResult holds the result of SaveAttachmentToDrive.
type SaveAttachmentToDriveResult struct {
	FileID      string
	FileName    string
	MIMEType    string
	Size        int64
	WebViewLink string
}

// SaveAttachmentToDrive downloads a Gmail attachment and uploads it directly
// to Google Drive without the data ever entering the LLM context window.
func SaveAttachmentToDrive(ctx context.Context, mgr *auth.Manager, params SaveAttachmentToDriveParams) (*SaveAttachmentToDriveResult, error) {
	if params.GmailAccount == "" {
		return nil, fmt.Errorf("gmail account is required")
	}
	if params.MessageID == "" {
		return nil, fmt.Errorf("message_id is required")
	}
	if params.AttachmentID == "" {
		return nil, fmt.Errorf("attachment_id is required")
	}
	if params.DriveAccount == "" {
		return nil, fmt.Errorf("drive account is required")
	}
	if params.FileName == "" {
		return nil, fmt.Errorf("file_name is required")
	}

	// Download attachment from Gmail.
	gmailSvc, err := newGmailService(ctx, mgr, params.GmailAccount)
	if err != nil {
		return nil, fmt.Errorf("creating Gmail service: %w", err)
	}

	att, err := gmailSvc.Users.Messages.Attachments.Get("me", params.MessageID, params.AttachmentID).Do()
	if err != nil {
		return nil, fmt.Errorf("getting attachment: %w", err)
	}

	data, err := base64.URLEncoding.DecodeString(att.Data)
	if err != nil {
		return nil, fmt.Errorf("decoding attachment data: %w", err)
	}

	// Upload to Drive.
	driveSvc, err := newDriveService(ctx, mgr, params.DriveAccount)
	if err != nil {
		return nil, fmt.Errorf("creating Drive service: %w", err)
	}

	file := &driveapi.File{Name: params.FileName}
	if params.FolderID != "" {
		file.Parents = []string{params.FolderID}
	}

	created, err := driveSvc.Files.Create(file).
		Media(bytes.NewReader(data)).
		Fields("id,name,mimeType,size,webViewLink").
		Do()
	if err != nil {
		return nil, fmt.Errorf("uploading to Drive: %w", err)
	}

	return &SaveAttachmentToDriveResult{
		FileID:      created.Id,
		FileName:    created.Name,
		MIMEType:    created.MimeType,
		Size:        created.Size,
		WebViewLink: created.WebViewLink,
	}, nil
}

// ReadDriveFileParams holds the parameters for ReadDriveFile.
type ReadDriveFileParams struct {
	DriveAccount string
	FileID       string
}

// ReadDriveFileResult holds the result of ReadDriveFile.
type ReadDriveFileResult struct {
	Data     []byte
	FileName string
	MIMEType string
}

// ReadDriveFile downloads a file from Google Drive and returns its raw bytes.
// This is used by the attach_drive_file tool to attach Drive files to emails
// without the data transiting through the LLM context window.
func ReadDriveFile(ctx context.Context, mgr *auth.Manager, params ReadDriveFileParams) (*ReadDriveFileResult, error) {
	if params.DriveAccount == "" {
		return nil, fmt.Errorf("drive account is required")
	}
	if params.FileID == "" {
		return nil, fmt.Errorf("file_id is required")
	}

	driveSvc, err := newDriveService(ctx, mgr, params.DriveAccount)
	if err != nil {
		return nil, fmt.Errorf("creating Drive service: %w", err)
	}

	// Get metadata first.
	file, err := driveSvc.Files.Get(params.FileID).Fields("id,name,mimeType,size").Do()
	if err != nil {
		return nil, fmt.Errorf("getting file metadata: %w", err)
	}

	// Google Workspace files need export; regular files use download.
	var body io.ReadCloser
	if isGoogleWorkspaceFile(file.MimeType) {
		exportMIME := defaultExportMIME(file.MimeType)
		resp, err := driveSvc.Files.Export(params.FileID, exportMIME).Download()
		if err != nil {
			return nil, fmt.Errorf("exporting file: %w", err)
		}
		body = resp.Body
		file.MimeType = exportMIME
	} else {
		resp, err := driveSvc.Files.Get(params.FileID).Download()
		if err != nil {
			return nil, fmt.Errorf("downloading file: %w", err)
		}
		body = resp.Body
	}
	defer body.Close()

	// Limit to 25MB (Gmail's practical attachment limit).
	const maxSize = 25 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(body, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("reading file content: %w", err)
	}
	if len(data) > maxSize {
		return nil, fmt.Errorf("file exceeds 25 MB attachment limit (%s)", file.Name)
	}

	return &ReadDriveFileResult{
		Data:     data,
		FileName: file.Name,
		MIMEType: file.MimeType,
	}, nil
}

// GetDriveFileMetadataParams holds the parameters for GetDriveFileMetadata.
type GetDriveFileMetadataParams struct {
	DriveAccount string
	FileID       string
}

// GetDriveFileMetadataResult holds the result of GetDriveFileMetadata.
type GetDriveFileMetadataResult struct {
	FileID      string
	FileName    string
	MIMEType    string
	WebViewLink string
}

// GetDriveFileMetadata returns metadata for a Drive file without downloading
// its content. This is used for Calendar event attachments where only a
// reference (fileUrl, title, mimeType) is needed, not the actual file bytes.
func GetDriveFileMetadata(ctx context.Context, mgr *auth.Manager, params GetDriveFileMetadataParams) (*GetDriveFileMetadataResult, error) {
	if params.DriveAccount == "" {
		return nil, fmt.Errorf("drive account is required")
	}
	if params.FileID == "" {
		return nil, fmt.Errorf("file_id is required")
	}

	driveSvc, err := newDriveService(ctx, mgr, params.DriveAccount)
	if err != nil {
		return nil, fmt.Errorf("creating Drive service: %w", err)
	}

	file, err := driveSvc.Files.Get(params.FileID).Fields("id,name,mimeType,webViewLink").Do()
	if err != nil {
		return nil, fmt.Errorf("getting file metadata: %w", err)
	}

	return &GetDriveFileMetadataResult{
		FileID:      file.Id,
		FileName:    file.Name,
		MIMEType:    file.MimeType,
		WebViewLink: file.WebViewLink,
	}, nil
}

// isGoogleWorkspaceFile returns true if the MIME type is a Google Workspace type.
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
