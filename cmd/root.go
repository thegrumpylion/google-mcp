// Package cmd implements the CLI commands for google-mcp.
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/calendar"
	"github.com/thegrumpylion/google-mcp/internal/drive"
	"github.com/thegrumpylion/google-mcp/internal/gmail"
	"github.com/thegrumpylion/google-mcp/internal/localfs"
	"github.com/thegrumpylion/google-mcp/internal/server"
)

var (
	configDir       string
	credentialsFile string
	version         = "dev"
)

// SetVersion sets the version string used in the CLI and MCP server.
func SetVersion(v string) {
	version = v
}

func newManager() (*auth.Manager, error) {
	return auth.NewManager(configDir, credentialsFile)
}

// NewRootCmd creates the root cobra command.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "google-mcp",
		Short: "Google MCP servers for Gmail, Drive, and Calendar",
		Long: `google-mcp provides Model Context Protocol (MCP) servers for Google services.

Each service runs as a separate MCP server via subcommands:
  google-mcp gmail      - Gmail MCP server
  google-mcp drive      - Google Drive MCP server
  google-mcp calendar   - Google Calendar MCP server

Setup:
  1. Download OAuth credentials from https://console.cloud.google.com/apis/credentials
  2. Place the file at ~/.config/google-mcp/credentials.json (or use --credentials)
  3. Add accounts: google-mcp auth add <name>`,
	}

	root.PersistentFlags().StringVar(&configDir, "config-dir", "", "config directory (default: $XDG_CONFIG_HOME/google-mcp)")
	root.PersistentFlags().StringVar(&credentialsFile, "credentials", "", "path to Google OAuth credentials.json (default: <config-dir>/credentials.json)")

	root.AddCommand(
		newAuthCmd(),
		newGmailCmd(),
		newDriveCmd(),
		newCalendarCmd(),
	)

	return root
}

// --- auth commands ---

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage Google account authentication",
	}

	cmd.AddCommand(
		newAuthAddCmd(),
		newAuthListCmd(),
		newAuthRemoveCmd(),
	)

	return cmd
}

func newAuthAddCmd() *cobra.Command {
	var scopes []string

	cmd := &cobra.Command{
		Use:   "add <account-name>",
		Short: "Add a Google account via OAuth browser flow",
		Long: `Authenticate a Google account and store the token under the given name.
The name is your own label (e.g. "personal", "work").

Requires credentials.json from Google Cloud Console at the default
path (~/.config/google-mcp/credentials.json) or via --credentials.

By default, all scopes (Gmail, Drive, Calendar) are requested. Use --scopes to limit.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newManager()
			if err != nil {
				return err
			}

			name := args[0]

			// Build scope list.
			allScopes := mergeScopes(gmail.AccountScopes(), drive.AccountScopes(), calendar.AccountScopes())
			if len(scopes) > 0 {
				allScopes = scopes
			}

			return mgr.Authenticate(cmd.Context(), name, allScopes)
		},
	}

	cmd.Flags().StringSliceVar(&scopes, "scopes", nil, "specific OAuth scopes to request (default: all Gmail+Drive+Calendar scopes)")

	return cmd
}

func newAuthListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newManager()
			if err != nil {
				return err
			}

			accounts := mgr.ListAccounts()
			if len(accounts) == 0 {
				fmt.Println("No accounts configured.")
				return nil
			}

			fmt.Println("Configured accounts:")
			for name, email := range accounts {
				if email != "" {
					fmt.Printf("  - %s (%s)\n", name, email)
				} else {
					fmt.Printf("  - %s\n", name)
				}
			}
			return nil
		},
	}
}

func newAuthRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <account-name>",
		Short: "Remove a stored account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newManager()
			if err != nil {
				return err
			}
			if err := mgr.RemoveAccount(args[0]); err != nil {
				return err
			}
			fmt.Printf("Account %q removed.\n", args[0])
			return nil
		},
	}
}

// --- MCP server commands ---

// toolFilterFlags holds the CLI flags for tool filtering.
type toolFilterFlags struct {
	readOnly bool
	enable   []string
	disable  []string
}

// addToolFilterFlags adds --read-only, --enable, and --disable flags to a command.
func addToolFilterFlags(cmd *cobra.Command, f *toolFilterFlags) {
	cmd.Flags().BoolVar(&f.readOnly, "read-only", false, "only expose read-only tools (no mutations)")
	cmd.Flags().StringSliceVar(&f.enable, "enable", nil, "whitelist of tool names to expose (comma-separated)")
	cmd.Flags().StringSliceVar(&f.disable, "disable", nil, "blacklist of tool names to hide (comma-separated)")
	cmd.MarkFlagsMutuallyExclusive("enable", "disable")
}

// toToolFilter converts the CLI flags to an server.ToolFilter.
func (f *toolFilterFlags) toToolFilter() server.ToolFilter {
	return server.ToolFilter{
		ReadOnly: f.readOnly,
		Enable:   f.enable,
		Disable:  f.disable,
	}
}

// localFSFlags holds the CLI flags for local filesystem access.
type localFSFlags struct {
	readDirs  []string
	writeDirs []string
}

// addLocalFSFlags adds --allow-read-dir and --allow-write-dir flags to a command.
func addLocalFSFlags(cmd *cobra.Command, f *localFSFlags) {
	cmd.Flags().StringSliceVar(&f.readDirs, "allow-read-dir", nil, "local directories to allow reading from (repeatable, comma-separated)")
	cmd.Flags().StringSliceVar(&f.writeDirs, "allow-write-dir", nil, "local directories to allow reading and writing (repeatable, comma-separated)")
}

// toLocalFS creates a localfs.FS from the CLI flags.
// Returns nil if no directories are configured (local file access disabled).
func (f *localFSFlags) toLocalFS() (*localfs.FS, error) {
	if len(f.readDirs) == 0 && len(f.writeDirs) == 0 {
		return nil, nil
	}
	var dirs []localfs.Dir
	for _, d := range f.readDirs {
		dirs = append(dirs, localfs.Dir{Path: d, Mode: localfs.ModeRead})
	}
	for _, d := range f.writeDirs {
		dirs = append(dirs, localfs.Dir{Path: d, Mode: localfs.ModeReadWrite})
	}
	return localfs.New(dirs)
}

func newGmailCmd() *cobra.Command {
	var flags toolFilterFlags
	var fsFlags localFSFlags
	cmd := &cobra.Command{
		Use:   "gmail",
		Short: "Start the Gmail MCP server (stdio)",
		Long: `Starts an MCP server over stdio with Gmail tools:
  list_accounts, search_messages, read_message, read_thread, send_message,
  list_labels, modify_messages, get_attachment,
  create_draft, list_drafts, send_draft, and more.

Use --read-only to expose only read-only tools.
Use --enable or --disable for granular tool control.
Use --allow-read-dir to enable local file attachments (opt-in, secure).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newManager()
			if err != nil {
				return err
			}

			srv := server.NewServer(&mcp.Implementation{
				Name:    "google-mcp-gmail",
				Version: version,
			}, nil)

			lfs, err := fsFlags.toLocalFS()
			if err != nil {
				return err
			}
			if lfs != nil {
				defer lfs.Close()
				srv.SetLocalFS(lfs)
			}

			gmail.RegisterTools(srv, mgr)

			if err := srv.ApplyFilter(flags.toToolFilter()); err != nil {
				return err
			}

			return srv.Run(context.Background(), &mcp.StdioTransport{})
		},
	}
	addToolFilterFlags(cmd, &flags)
	addLocalFSFlags(cmd, &fsFlags)
	return cmd
}

func newDriveCmd() *cobra.Command {
	var flags toolFilterFlags
	var fsFlags localFSFlags
	cmd := &cobra.Command{
		Use:   "drive",
		Short: "Start the Google Drive MCP server (stdio)",
		Long: `Starts an MCP server over stdio with Drive tools:
  list_accounts, search_files, list_files, get_file, read_file, upload_file,
  update_file, delete_file, create_folder, move_file, copy_file, share_file.

Use --read-only to expose only read-only tools.
Use --enable or --disable for granular tool control.
Use --allow-read-dir to enable uploading local files (opt-in, secure).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newManager()
			if err != nil {
				return err
			}

			srv := server.NewServer(&mcp.Implementation{
				Name:    "google-mcp-drive",
				Version: version,
			}, nil)

			lfs, err := fsFlags.toLocalFS()
			if err != nil {
				return err
			}
			if lfs != nil {
				defer lfs.Close()
				srv.SetLocalFS(lfs)
			}

			drive.RegisterTools(srv, mgr)

			if err := srv.ApplyFilter(flags.toToolFilter()); err != nil {
				return err
			}

			return srv.Run(context.Background(), &mcp.StdioTransport{})
		},
	}
	addToolFilterFlags(cmd, &flags)
	addLocalFSFlags(cmd, &fsFlags)
	return cmd
}

func newCalendarCmd() *cobra.Command {
	var flags toolFilterFlags
	var fsFlags localFSFlags
	cmd := &cobra.Command{
		Use:   "calendar",
		Short: "Start the Google Calendar MCP server (stdio)",
		Long: `Starts an MCP server over stdio with Calendar tools:
  list_accounts, list_calendars, list_events, get_event,
  create_event, update_event, delete_event, respond_event.

Use --read-only to expose only read-only tools.
Use --enable or --disable for granular tool control.
Use --allow-read-dir to enable local file access (opt-in, secure).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newManager()
			if err != nil {
				return err
			}

			srv := server.NewServer(&mcp.Implementation{
				Name:    "google-mcp-calendar",
				Version: version,
			}, nil)

			lfs, err := fsFlags.toLocalFS()
			if err != nil {
				return err
			}
			if lfs != nil {
				defer lfs.Close()
				srv.SetLocalFS(lfs)
			}

			calendar.RegisterTools(srv, mgr)

			if err := srv.ApplyFilter(flags.toToolFilter()); err != nil {
				return err
			}

			return srv.Run(context.Background(), &mcp.StdioTransport{})
		},
	}
	addToolFilterFlags(cmd, &flags)
	addLocalFSFlags(cmd, &fsFlags)
	return cmd
}

// --- helpers ---

func mergeScopes(scopeSets ...[]string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, scopes := range scopeSets {
		for _, s := range scopes {
			if !seen[s] {
				seen[s] = true
				result = append(result, s)
			}
		}
	}
	return result
}

// Execute runs the root command.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
