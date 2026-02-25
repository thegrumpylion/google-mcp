# google-mcp

MCP servers for Google services — Gmail, Google Drive, and Google Calendar.

Each service runs as a separate [Model Context Protocol](https://modelcontextprotocol.io/) server, designed for use with AI coding assistants and MCP-compatible clients.

Supports multiple Google accounts (e.g. "personal", "work") with a single binary.

## Features

- **Gmail** — search, read, send (with attachments), draft, label management, attachments, vacation settings, cross-service Drive integration
- **Google Drive** — search, list, read, upload, copy, move, share, permissions, shared drives, trash
- **Google Calendar** — list, create, update, delete events, manage invitations
- **Multi-account** — use `account="all"` to query across all accounts at once
- **Per-service servers** — run only what you need
- **Tool filtering** — `--read-only`, `--enable`, `--disable` for granular control

## Install

### From source

Requires [Go 1.25+](https://go.dev/dl/).

```sh
go install github.com/thegrumpylion/google-mcp@latest
```

Or clone and build:

```sh
git clone https://github.com/thegrumpylion/google-mcp.git
cd google-mcp
go build -o ~/.local/bin/google-mcp .
```

## Google Cloud Setup

Before using google-mcp, you need OAuth credentials from a Google Cloud project. This is a one-time setup.

### 1. Create a Google Cloud Project

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Click the project dropdown at the top and select **New Project**
3. Give it a name (e.g. "MCP") and click **Create**
4. Make sure your new project is selected in the project dropdown

### 2. Enable APIs

Enable the APIs for the services you want to use:

- **Gmail API** — [Enable here](https://console.cloud.google.com/apis/library/gmail.googleapis.com)
- **Google Drive API** — [Enable here](https://console.cloud.google.com/apis/library/drive.googleapis.com)
- **Google Calendar API** — [Enable here](https://console.cloud.google.com/apis/library/calendar-json.googleapis.com)

### 3. Configure the OAuth Consent Screen

1. Go to [OAuth consent screen](https://console.cloud.google.com/auth/audience)
2. Click **Get started**
3. Fill in the required fields:
   - **App name**: e.g. "google-mcp"
   - **User support email**: select your email
   - **Audience**: select **External**
4. Accept the defaults and click **Create**
5. Under **Audience** > **Test users**, click **Add users**
6. Add the email addresses of every Google account you plan to use with google-mcp (e.g. your personal Gmail, work account, etc.)
7. Click **Save**

> **Note:** While the app is in "Testing" status, only users listed as test users can authorize. This is fine for personal use — you don't need to publish or verify the app.

### 4. Add OAuth Scopes

1. Go to [Data Access](https://console.cloud.google.com/auth/scopes) (under APIs & Services > Data Access)
2. Click **Add or Remove Scopes**
3. Add the following scopes depending on which services you want:

| Service  | Scope | Description |
|----------|-------|-------------|
| Gmail    | `https://www.googleapis.com/auth/gmail.modify` | Read, compose, send, and manage labels |
| Gmail    | `https://www.googleapis.com/auth/gmail.send` | Send email on your behalf |
| Drive    | `https://www.googleapis.com/auth/drive` | Full access to Google Drive |
| Calendar | `https://www.googleapis.com/auth/calendar.readonly` | Read calendar events |
| Calendar | `https://www.googleapis.com/auth/calendar.events` | Create, update, delete events |

4. Click **Update** and then **Save**

### 5. Create OAuth Credentials

1. Go to [Credentials](https://console.cloud.google.com/apis/credentials)
2. Click **Create Credentials** > **OAuth client ID**
3. Select **Desktop app** as the application type
4. Give it a name (e.g. "google-mcp")
5. Click **Create**
6. Click **Download JSON** (or the download icon next to your new client ID)
7. Save the file as `credentials.json` in your config directory:

```sh
mkdir -p ~/.config/google-mcp
mv ~/Downloads/client_secret_*.json ~/.config/google-mcp/credentials.json
```

## Account Setup

Add each Google account you want to use. The name is your own label.

```sh
# Add accounts
google-mcp auth add personal
google-mcp auth add work

# List configured accounts
google-mcp auth list

# Remove an account
google-mcp auth remove work
```

When you run `auth add`, a browser window opens for Google's OAuth consent flow. After authorizing, the token is saved locally.

> **Important:** Each account you add must be listed as a test user in the [OAuth consent screen](https://console.cloud.google.com/auth/audience) (see step 3.6 above).

## Usage

Each service runs as a separate MCP server over stdio:

```sh
google-mcp gmail      # Start Gmail MCP server
google-mcp drive      # Start Google Drive MCP server
google-mcp calendar   # Start Google Calendar MCP server
```

### MCP Client Configuration

#### Claude Code

Add to `~/.claude.json` or your project's `.mcp.json`:

```json
{
  "mcpServers": {
    "gmail": {
      "command": "google-mcp",
      "args": ["gmail"]
    },
    "drive": {
      "command": "google-mcp",
      "args": ["drive"]
    },
    "calendar": {
      "command": "google-mcp",
      "args": ["calendar"]
    }
  }
}
```

To use read-only mode or tool filtering, add flags to the `args` array:

```json
{
  "mcpServers": {
    "gmail": {
      "command": "google-mcp",
      "args": ["gmail", "--read-only"]
    },
    "drive": {
      "command": "google-mcp",
      "args": ["drive", "--disable", "delete_file,empty_trash"]
    }
  }
}
```

#### OpenCode

Add to `~/.config/opencode/opencode.json`:

```json
{
  "mcp": {
    "gmail": {
      "type": "local",
      "command": ["google-mcp", "gmail"]
    },
    "drive": {
      "type": "local",
      "command": ["google-mcp", "drive"]
    },
    "calendar": {
      "type": "local",
      "command": ["google-mcp", "calendar"]
    }
  }
}
```

### Flags

**Global flags** (all subcommands):

```
--config-dir     Override config directory (default: ~/.config/google-mcp)
--credentials    Override path to credentials.json
```

**Server flags** (gmail, drive, calendar):

```
--read-only      Only expose read-only tools (no mutations)
--enable         Whitelist of tool names to expose (comma-separated)
--disable        Blacklist of tool names to hide (comma-separated)
```

`--enable` and `--disable` are mutually exclusive. When `--read-only` is set, `--enable`/`--disable` operate on the read-only subset only.

**Examples:**

```sh
# Read-only Gmail server (no send, modify, delete, etc.)
google-mcp gmail --read-only

# Only expose search and read tools
google-mcp gmail --enable search_messages,read_message,read_thread,list_labels

# Everything except delete
google-mcp drive --disable delete_file,empty_trash

# Read-only drive, but exclude shared drive tools
google-mcp drive --read-only --disable list_shared_drives,get_shared_drive
```

## Available Tools

### Gmail (26 tools)

| Tool | Description |
|------|-------------|
| `list_accounts` | List configured accounts |
| `get_profile` | Get email address, message/thread counts |
| `search_messages` | Search messages using Gmail query syntax |
| `read_message` | Read full message content by ID |
| `list_threads` | List threads (thread-based browsing) |
| `read_thread` | Read all messages in a thread |
| `modify_thread` | Add/remove labels on an entire thread |
| `trash_thread` | Move a thread to trash |
| `untrash_thread` | Restore a thread from trash |
| `send_message` | Send an email with attachments (inline base64 or from Google Drive) |
| `list_labels` | List all labels |
| `get_label` | Get label details (unread/total counts) |
| `create_label` | Create a custom label |
| `delete_label` | Delete a custom label |
| `modify_messages` | Batch add/remove labels on messages |
| `delete_message` | Permanently delete a message |
| `get_attachment` | Download an attachment |
| `get_vacation` | Get vacation/auto-reply settings |
| `update_vacation` | Update vacation/auto-reply settings |
| `create_draft` | Create a draft (with attachments) |
| `list_drafts` | List drafts |
| `get_draft` | Get a draft by ID |
| `update_draft` | Update a draft (with attachments) |
| `delete_draft` | Delete a draft |
| `send_draft` | Send an existing draft |
| `save_attachment_to_drive` | Save a Gmail attachment directly to Google Drive (server-side) |

### Google Drive (20 tools)

| Tool | Description |
|------|-------------|
| `list_accounts` | List configured accounts |
| `search_files` | Search files using Drive query syntax |
| `list_files` | List files, optionally in a folder |
| `get_file` | Get file metadata |
| `read_file` | Read/download file content |
| `upload_file` | Upload a new file |
| `update_file` | Update file metadata (rename, description) |
| `delete_file` | Delete a file (trash or permanent) |
| `create_folder` | Create a folder |
| `move_file` | Move a file to a different folder |
| `copy_file` | Copy a file |
| `share_file` | Share a file (user, group, domain, anyone) |
| `list_permissions` | List who has access to a file |
| `get_permission` | Inspect a specific permission |
| `update_permission` | Change access level for a permission |
| `delete_permission` | Revoke access (unshare) |
| `empty_trash` | Permanently delete all trashed files |
| `get_about` | Get storage quota, user info, export formats |
| `list_shared_drives` | List shared drives |
| `get_shared_drive` | Get shared drive details |

### Google Calendar (8 tools)

| Tool | Description |
|------|-------------|
| `list_accounts` | List configured accounts |
| `list_calendars` | List all accessible calendars |
| `list_events` | List events in a time range |
| `get_event` | Get event details |
| `create_event` | Create a new event |
| `update_event` | Update an existing event |
| `delete_event` | Delete an event |
| `respond_event` | Respond to an invitation (accept/decline/tentative) |

### Multi-Account Queries

All read-only tools support `account="all"` to fan out queries across every configured account:

```
search(account="all", query="from:boss subject:urgent")       # on gmail server
search(account="all", query="name contains 'report'")         # on drive server
list_events(account="all")                                     # on calendar server
```

## Configuration

| File | Purpose |
|------|---------|
| `~/.config/google-mcp/credentials.json` | OAuth client credentials from Google Cloud Console |
| `~/.config/google-mcp/tokens.json` | Stored account tokens (created by `auth add`) |

The config directory defaults to `$XDG_CONFIG_HOME/google-mcp` or `~/.config/google-mcp`. Override with `--config-dir`.

## License

MIT
