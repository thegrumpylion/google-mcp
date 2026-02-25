# google-mcp

MCP servers for Google services — Gmail, Google Drive, and Google Calendar.

Each service runs as a separate [Model Context Protocol](https://modelcontextprotocol.io/) server, designed for use with AI coding assistants and MCP-compatible clients.

Supports multiple Google accounts (e.g. "personal", "work") with a single binary.

## Features

- **Gmail** — search, read, send (with attachments), drafts, labels, filters, trash/untrash, history, send-as aliases, vacation settings, cross-service Drive integration
- **Google Drive** — search, list, read, upload, copy, move, share, permissions, shared drives, revisions, change tracking, trash
- **Google Calendar** — list, create, update, delete events, manage invitations, free/busy queries, calendar CRUD, sharing (ACL)
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
| Gmail    | `https://mail.google.com/` | Full mailbox access (read, send, delete, settings) |
| Gmail    | `https://www.googleapis.com/auth/gmail.settings.basic` | Manage filters and other basic settings |
| Gmail    | `https://www.googleapis.com/auth/drive` | Attach Drive files and save attachments to Drive |
| Drive    | `https://www.googleapis.com/auth/drive` | Full access to Google Drive |
| Calendar | `https://www.googleapis.com/auth/calendar` | Full access to Google Calendar (events, calendars, sharing) |

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

To use read-only mode, tool filtering, or local file access, add flags to the `args` array:

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

To enable local file attachments and uploads:

```json
{
  "mcpServers": {
    "gmail": {
      "command": "google-mcp",
      "args": ["gmail", "--allow-read-dir", "/home/user/documents", "--allow-write-dir", "/home/user/downloads"]
    },
    "drive": {
      "command": "google-mcp",
      "args": ["drive", "--allow-read-dir", "/home/user/documents", "--allow-write-dir", "/home/user/downloads"]
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
--read-only        Only expose read-only tools (no mutations)
--enable           Whitelist of tool names to expose (comma-separated)
--disable          Blacklist of tool names to hide (comma-separated)
--allow-read-dir   Local directories to allow reading from (repeatable)
--allow-write-dir  Local directories to allow reading and writing (repeatable)
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

# Enable local file access for email attachments
google-mcp gmail --allow-read-dir /home/user/documents --allow-read-dir /home/user/exports

# Enable local file upload to Drive
google-mcp drive --allow-read-dir /home/user/documents
```

## Cross-Service Integration

The Gmail server includes built-in Google Drive integration. All data flows server-side — file content never enters the LLM context window.

### Email Attachments

`send_message`, `create_draft`, and `update_draft` support three kinds of attachments:

- **Inline attachments** — base64-encoded content provided directly in the `attachments` field
- **Drive attachments** — Google Drive file IDs in the `drive_attachments` field, resolved server-side
- **Local attachments** — local file paths in the `local_attachments` field, read from allowed directories

Drive attachments can reference files from **any configured account**, not just the sending account. For example, you can send an email from your personal Gmail with a file attached from your work Drive — something even Gmail's web UI can't do.

```
send_message(
  account="personal",
  to="colleague@example.com",
  subject="Q4 Report",
  body="See attached.",
  drive_attachments=[{drive_account: "work", file_id: "abc123"}]
)
```

The server fetches the file bytes from Drive in memory, encodes them as a MIME attachment, and sends via Gmail. The LLM only sees the file ID and a "Message sent" confirmation.

### Local File Access

Local file access is **opt-in only** and disabled by default. Use `--allow-read-dir` or `--allow-write-dir` to grant the MCP server access to specific directories. Path containment is enforced by `os.Root` (Go 1.25+) at the kernel level — `../` traversal and symlink escapes are blocked by the OS.

- `--allow-read-dir` grants read-only access (for uploading/attaching local files)
- `--allow-write-dir` grants read-write access (also enables saving files to disk)

When enabled, two convenience tools — `list_local_files` and `read_local_file` — are automatically added so the LLM can browse and read files in allowed directories. All tools that accept local file paths include the configured directory paths and access modes in their descriptions, so the LLM always knows which directories are available.

#### Uploading and Attaching Local Files

```
# Gmail: attach local files to emails
google-mcp gmail --allow-read-dir ~/documents

send_message(
  account="personal",
  to="colleague@example.com",
  subject="Report",
  body="See attached.",
  local_attachments=[{path: "reports/q4.pdf"}]
)

# Drive: upload local files
google-mcp drive --allow-read-dir ~/documents

upload_file(account="personal", local_path="reports/q4.pdf")
```

Local attachments are also supported on `create_draft` and `update_draft`.

#### Saving Files to Disk

With `--allow-write-dir`, the `read_file` (Drive) and `get_attachment` (Gmail) tools accept a `save_to` field. When set, the file is written to disk and **content never enters the conversation** — no size limits apply.

```
# Drive: download a file to disk
google-mcp drive --allow-write-dir ~/downloads

read_file(account="personal", file_id="...", save_to="report.pdf")

# Gmail: save an attachment to disk
google-mcp gmail --allow-write-dir ~/downloads

get_attachment(account="work", message_id="...", attachment_id="...", save_to="invoice.pdf")
```

#### Browsing Local Files

When any directory is configured, `list_local_files` and `read_local_file` tools appear automatically on all servers:

```
# List files in the allowed directory
list_local_files()                     # list root
list_local_files(path="subdir")        # list subdirectory

# Read a text file (512 KB limit, binary files rejected)
read_local_file(path="notes.txt")
```

### Save Attachment to Drive

The `save_attachment_to_drive` tool transfers a Gmail attachment directly to Google Drive. Like Drive attachments, it supports cross-account transfers — save an attachment from one account's inbox to a different account's Drive.

```
save_attachment_to_drive(
  account="work",
  message_id="...",
  attachment_id="...",
  drive_account="personal",
  file_name="invoice.pdf"
)
```

## Available Tools

### Gmail (36 tools)

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
| `delete_thread` | Permanently delete a thread (irreversible) |
| `send_message` | Send an email with attachments (inline base64 or from Google Drive) |
| `modify_messages` | Batch add/remove labels on messages |
| `trash_message` | Move a message to trash |
| `untrash_message` | Restore a message from trash |
| `delete_message` | Permanently delete a message (irreversible) |
| `batch_delete_messages` | Permanently delete multiple messages (irreversible) |
| `list_labels` | List all labels |
| `get_label` | Get label details (unread/total counts) |
| `create_label` | Create a custom label |
| `update_label` | Rename a label or change visibility |
| `delete_label` | Delete a custom label |
| `get_attachment` | Download an attachment (or save to local disk with `save_to`) |
| `list_history` | Track mailbox changes since a history ID |
| `list_filters` | List inbox filters (rules) |
| `create_filter` | Create an inbox filter |
| `delete_filter` | Delete an inbox filter |
| `list_send_as` | List send-as aliases |
| `get_vacation` | Get vacation/auto-reply settings |
| `update_vacation` | Update vacation/auto-reply settings |
| `create_draft` | Create a draft (with attachments) |
| `list_drafts` | List drafts |
| `get_draft` | Get a draft by ID |
| `update_draft` | Update a draft (with attachments) |
| `delete_draft` | Delete a draft |
| `send_draft` | Send an existing draft |
| `save_attachment_to_drive` | Save a Gmail attachment directly to Google Drive (server-side) |

### Google Drive (27 tools)

| Tool | Description |
|------|-------------|
| `list_accounts` | List configured accounts |
| `search_files` | Search files using Drive query syntax |
| `list_files` | List files, optionally in a folder |
| `get_file` | Get file metadata |
| `read_file` | Read/download file content (or save to local disk with `save_to`) |
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
| `create_shared_drive` | Create a new shared drive |
| `update_shared_drive` | Update a shared drive's name or settings |
| `delete_shared_drive` | Delete an empty shared drive |
| `list_revisions` | List file version history |
| `get_revision` | Get details of a specific file revision |
| `delete_revision` | Delete a specific file revision |
| `list_changes` | Track changes across Drive since a point in time |

### Google Calendar (16 tools)

| Tool | Description |
|------|-------------|
| `list_accounts` | List configured accounts |
| `list_calendars` | List all accessible calendars |
| `create_calendar` | Create a new calendar |
| `delete_calendar` | Delete a secondary calendar |
| `list_events` | List events in a time range |
| `get_event` | Get event details |
| `create_event` | Create a new event |
| `update_event` | Update an existing event |
| `delete_event` | Delete an event |
| `respond_event` | Respond to an invitation (accept/decline/tentative) |
| `quick_add_event` | Create event from natural language (e.g. "Lunch tomorrow at noon") |
| `list_event_instances` | List occurrences of a recurring event |
| `move_event` | Move an event to a different calendar |
| `query_free_busy` | Check availability for users/calendars in a time range |
| `share_calendar` | Share a calendar (user, group, domain, or public) |
| `list_calendar_sharing` | List sharing rules (ACL) for a calendar |

### Local File Tools (conditional)

These tools appear on **all servers** when `--allow-read-dir` or `--allow-write-dir` is set:

| Tool | Description |
|------|-------------|
| `list_local_files` | List files in an allowed local directory |
| `read_local_file` | Read a text file from an allowed local directory (512 KB limit) |

The `list_local_files` tool description includes the configured directory paths and access modes, so the LLM knows what's available without guessing.

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
