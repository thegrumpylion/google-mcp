# google-mcp

MCP servers for Google services — Gmail, Google Drive, and Google Calendar.

Each service runs as a separate [Model Context Protocol](https://modelcontextprotocol.io/) server, designed for use with AI coding assistants and MCP-compatible clients.

Supports multiple Google accounts (e.g. "personal", "work") with a single binary.

## Features

- **Gmail** — search, read, send, draft, label management, attachments
- **Google Drive** — search, list, read, upload, copy, move, share, folder management
- **Google Calendar** — list, create, update, delete events, manage invitations
- **Multi-account** — use `account="all"` to query across all accounts at once
- **Per-service servers** — run only what you need

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

```
--config-dir     Override config directory (default: ~/.config/google-mcp)
--credentials    Override path to credentials.json
```

## Available Tools

### Gmail (11 tools)

| Tool | Description |
|------|-------------|
| `list_accounts` | List configured accounts |
| `search` | Search messages using Gmail query syntax |
| `read` | Read full message content by ID |
| `read_thread` | Read all messages in a thread |
| `send` | Send an email |
| `list_labels` | List all labels |
| `modify` | Add/remove labels (archive, trash, star, read/unread) |
| `get_attachment` | Download an attachment |
| `draft_create` | Create a draft |
| `draft_list` | List drafts |
| `draft_send` | Send an existing draft |

### Google Drive (12 tools)

| Tool | Description |
|------|-------------|
| `list_accounts` | List configured accounts |
| `search` | Search files using Drive query syntax |
| `list` | List files, optionally in a folder |
| `get` | Get file metadata |
| `read` | Read/download file content |
| `upload` | Upload a new file |
| `update` | Update file metadata (rename, description) |
| `delete` | Delete a file (trash or permanent) |
| `create_folder` | Create a folder |
| `move` | Move a file to a different folder |
| `copy` | Copy a file |
| `share` | Share a file (user, group, domain, anyone) |

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
