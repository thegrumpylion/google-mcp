// Package server provides a wrapper around the MCP SDK server that captures
// tool metadata at registration time, enabling runtime filtering by read-only
// status, whitelists, and blacklists.
package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/localfs"
)

// BoolPtr returns a pointer to a bool value. Useful for MCP ToolAnnotations
// fields like DestructiveHint and OpenWorldHint which are *bool.
func BoolPtr(v bool) *bool { return &v }

// ToolInfo describes a registered tool for filtering purposes.
type ToolInfo struct {
	Name     string
	ReadOnly bool
}

// Server wraps an mcp.Server to capture tool metadata at registration time.
// Use AddTool to register tools; it records each tool's name and read-only
// status automatically. After all tools are registered, call ApplyFilter to
// remove tools that don't match the desired filter.
type Server struct {
	*mcp.Server
	tools   []ToolInfo
	localFS *localfs.FS
}

// NewServer creates a new Server wrapper around an mcp.Server.
func NewServer(impl *mcp.Implementation, opts *mcp.ServerOptions) *Server {
	return &Server{Server: mcp.NewServer(impl, opts)}
}

// SetLocalFS sets the local filesystem access for the server.
// Tools can use LocalFS() to read/write local files within allowed directories.
func (s *Server) SetLocalFS(fs *localfs.FS) {
	s.localFS = fs
}

// LocalFS returns the local filesystem access, or nil if not configured.
func (s *Server) LocalFS() *localfs.FS {
	return s.localFS
}

// Tools returns the metadata for all registered tools.
func (s *Server) Tools() []ToolInfo {
	return s.tools
}

// AddTool registers a typed tool on the server and records its metadata.
// This is a free generic function because Go does not allow generic methods
// on types — the same pattern the MCP SDK uses for mcp.AddTool.
func AddTool[In, Out any](s *Server, t *mcp.Tool, h mcp.ToolHandlerFor[In, Out]) {
	s.tools = append(s.tools, ToolInfo{
		Name:     t.Name,
		ReadOnly: t.Annotations != nil && t.Annotations.ReadOnlyHint,
	})
	mcp.AddTool(s.Server, t, h)
}

// WriteDirsDescription returns a description snippet listing the configured
// write-enabled directories, suitable for appending to a tool description.
// Returns an empty string if no local filesystem is configured or there are
// no write directories.
func (s *Server) WriteDirsDescription() string {
	if s.localFS == nil {
		return ""
	}
	var sb strings.Builder
	for _, d := range s.localFS.Dirs() {
		if d.Mode == localfs.ModeReadWrite {
			fmt.Fprintf(&sb, "  - %s\n", d.Path)
		}
	}
	if sb.Len() == 0 {
		return ""
	}
	return "\n\nAllowed write directories (for save_to paths):\n" + sb.String()
}

// ReadDirsDescription returns a description snippet listing all configured
// directories (both read-only and read-write), suitable for appending to a
// tool description. Returns an empty string if no local filesystem is configured.
func (s *Server) ReadDirsDescription() string {
	if s.localFS == nil {
		return ""
	}
	dirs := s.localFS.Dirs()
	if len(dirs) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\nAllowed local directories (for local file paths):\n")
	for _, d := range dirs {
		mode := "read-only"
		if d.Mode == localfs.ModeReadWrite {
			mode = "read-write"
		}
		fmt.Fprintf(&sb, "  - %s (%s)\n", d.Path, mode)
	}
	return sb.String()
}

// ToolFilter configures which tools are exposed by an MCP server.
type ToolFilter struct {
	// ReadOnly limits the server to read-only tools.
	ReadOnly bool
	// Enable is a whitelist of tool names to expose. Mutually exclusive with Disable.
	Enable []string
	// Disable is a blacklist of tool names to hide. Mutually exclusive with Enable.
	Disable []string
}

// ApplyFilter removes tools from the server based on the filter configuration.
// Returns an error if the filter is invalid (e.g. enable and disable both set,
// or referencing unknown tool names).
func (s *Server) ApplyFilter(filter ToolFilter) error {
	if len(filter.Enable) > 0 && len(filter.Disable) > 0 {
		return fmt.Errorf("--enable and --disable are mutually exclusive")
	}

	// Build the base set: all tools or read-only only.
	baseSet := make(map[string]bool, len(s.tools))
	allTools := make(map[string]bool, len(s.tools))
	for _, t := range s.tools {
		allTools[t.Name] = true
		if filter.ReadOnly {
			if t.ReadOnly {
				baseSet[t.Name] = true
			}
		} else {
			baseSet[t.Name] = true
		}
	}

	// If read-only mode, remove all non-read-only tools first.
	if filter.ReadOnly {
		var remove []string
		for _, t := range s.tools {
			if !t.ReadOnly {
				remove = append(remove, t.Name)
			}
		}
		if len(remove) > 0 {
			s.RemoveTools(remove...)
		}
	}

	// Apply enable (whitelist).
	if len(filter.Enable) > 0 {
		for _, name := range filter.Enable {
			if !baseSet[name] {
				if allTools[name] && filter.ReadOnly {
					return fmt.Errorf("tool %q is not a read-only tool", name)
				}
				return fmt.Errorf("unknown tool %q", name)
			}
		}
		enabled := make(map[string]bool, len(filter.Enable))
		for _, name := range filter.Enable {
			enabled[name] = true
		}
		var remove []string
		for name := range baseSet {
			if !enabled[name] {
				remove = append(remove, name)
			}
		}
		if len(remove) > 0 {
			s.RemoveTools(remove...)
		}
	}

	// Apply disable (blacklist).
	if len(filter.Disable) > 0 {
		for _, name := range filter.Disable {
			if !baseSet[name] {
				if allTools[name] && filter.ReadOnly {
					return fmt.Errorf("tool %q is not a read-only tool", name)
				}
				return fmt.Errorf("unknown tool %q", name)
			}
		}
		s.RemoveTools(filter.Disable...)
	}

	return nil
}

// RegisterAccountsListTool registers the list_accounts tool on the given server.
// This tool is shared across all servers (Gmail, Drive, Calendar).
func RegisterAccountsListTool(s *Server, mgr *auth.Manager) {
	AddTool(s, &mcp.Tool{
		Name:        "list_accounts",
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
