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

func newGmailCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gmail",
		Short: "Start the Gmail MCP server (stdio)",
		Long: `Starts an MCP server over stdio with Gmail tools:
  accounts_list, gmail_search, gmail_read, gmail_read_thread, gmail_send,
  gmail_list_labels, gmail_modify, gmail_get_attachment,
  gmail_draft_create, gmail_draft_list, gmail_draft_send.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newManager()
			if err != nil {
				return err
			}

			server := mcp.NewServer(&mcp.Implementation{
				Name:    "google-mcp-gmail",
				Version: version,
			}, nil)

			gmail.RegisterTools(server, mgr)

			return server.Run(context.Background(), &mcp.StdioTransport{})
		},
	}
}

func newDriveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "drive",
		Short: "Start the Google Drive MCP server (stdio)",
		Long:  "Starts an MCP server over stdio with Drive tools: drive_search, drive_list, drive_get, drive_read, accounts_list.",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newManager()
			if err != nil {
				return err
			}

			server := mcp.NewServer(&mcp.Implementation{
				Name:    "google-mcp-drive",
				Version: version,
			}, nil)

			drive.RegisterTools(server, mgr)

			return server.Run(context.Background(), &mcp.StdioTransport{})
		},
	}
}

func newCalendarCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "calendar",
		Short: "Start the Google Calendar MCP server (stdio)",
		Long: `Starts an MCP server over stdio with Calendar tools:
  accounts_list, calendar_list_calendars, calendar_list_events, calendar_get_event,
  calendar_create_event, calendar_update_event, calendar_delete_event, calendar_respond_event.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newManager()
			if err != nil {
				return err
			}

			server := mcp.NewServer(&mcp.Implementation{
				Name:    "google-mcp-calendar",
				Version: version,
			}, nil)

			calendar.RegisterTools(server, mgr)

			return server.Run(context.Background(), &mcp.StdioTransport{})
		},
	}
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
