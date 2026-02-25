// Package drive provides MCP tools for interacting with the Google Drive API.
package drive

import (
	"context"

	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
	"google.golang.org/api/drive/v3"
)

// Scopes required by the Drive tools.
var Scopes = []string{
	drive.DriveScope,
}

// RegisterTools registers all Drive MCP tools on the given server.
func RegisterTools(srv *server.Server, mgr *auth.Manager) {
	server.RegisterAccountsListTool(srv, mgr)
	server.RegisterLocalFSTools(srv)
	// files.go
	registerSearch(srv, mgr)
	registerList(srv, mgr)
	registerGet(srv, mgr)
	registerRead(srv, mgr)
	registerUpload(srv, mgr)
	registerUpdate(srv, mgr)
	registerDelete(srv, mgr)
	registerCreateFolder(srv, mgr)
	registerMove(srv, mgr)
	registerCopy(srv, mgr)
	// permissions.go
	registerShare(srv, mgr)
	registerListPermissions(srv, mgr)
	registerGetPermission(srv, mgr)
	registerUpdatePermission(srv, mgr)
	registerDeletePermission(srv, mgr)
	// trash.go
	registerEmptyTrash(srv, mgr)
	// about.go
	registerGetAbout(srv, mgr)
	// drives.go
	registerListSharedDrives(srv, mgr)
	registerGetSharedDrive(srv, mgr)
	registerCreateSharedDrive(srv, mgr)
	registerUpdateSharedDrive(srv, mgr)
	registerDeleteSharedDrive(srv, mgr)
	// revisions.go
	registerListRevisions(srv, mgr)
	registerGetRevision(srv, mgr)
	registerDeleteRevision(srv, mgr)
	// changes.go
	registerListChanges(srv, mgr)
}

func newService(ctx context.Context, mgr *auth.Manager, account string) (*drive.Service, error) {
	opt, err := mgr.ClientOption(ctx, account, Scopes)
	if err != nil {
		return nil, err
	}
	return drive.NewService(ctx, opt)
}

// AccountScopes returns the scopes used by Drive tools.
func AccountScopes() []string {
	return Scopes
}
