# Google MCP API Coverage

Tracking document for SDK method coverage across all three MCP servers.

Last updated: 2026-02-25

## Conventions (all new tools must follow)

1. **Naming:** `action_resource` pattern (e.g. `list_events`, `create_draft`, `get_profile`)
2. **Annotations:**
   - Read-only tools: `ReadOnlyHint: true`
   - Non-destructive mutations (create, untrash, restore): `DestructiveHint: server.BoolPtr(false)`
   - Idempotent mutations (update, modify labels): `IdempotentHint: true`
   - Destructive mutations (delete, trash): use defaults (no explicit hint needed)
3. **Account field descriptions:** `"Account name"` for single-account tools, `"Account name or 'all' for all accounts"` for multi-account tools
4. **Response format:** qualified IDs (e.g. `"Message ID: %s"`), newline-separated key-value pairs, no trailing `!`
5. **Input validation:** validate required fields before making API calls
6. **Helper usage:** `server.BoolPtr(bool)` for `*bool` annotation fields; `buildMessage()` in compose.go for RFC 2822 messages
7. **Testing:** every new tool must be added to `TestToolNames` and `TestToolAnnotations` in the corresponding `tools_test.go`

## Summary

| Server   | Tools | SDK Methods Covered | Total SDK Methods | Coverage |
|----------|-------|--------------------:|------------------:|---------:|
| Gmail    |    25 |                  23 |                80 |      29% |
| Drive    |    20 |                  21 |                58 |      36% |
| Calendar |     8 |                   8 |                38 |      21% |
| **Total**| **53**|              **52** |           **176** |  **~30%**|

---

## Gmail

### Implemented

| Tool | SDK Method(s) | Type |
|------|--------------|------|
| `list_accounts` | Internal auth manager | Read |
| `get_profile` | `Users.GetProfile` | Read |
| `search_messages` | `Messages.List` + `Messages.Get` | Read |
| `read_message` | `Messages.Get` (full) | Read |
| `modify_messages` | `Messages.BatchModify` | Mutation |
| `delete_message` | `Messages.Delete` | Mutation |
| `send_message` | `Messages.Send` | Mutation |
| `list_threads` | `Threads.List` | Read |
| `read_thread` | `Threads.Get` (full) | Read |
| `modify_thread` | `Threads.Modify` | Mutation |
| `trash_thread` | `Threads.Trash` | Mutation |
| `untrash_thread` | `Threads.Untrash` | Mutation |
| `list_labels` | `Labels.List` | Read |
| `get_label` | `Labels.Get` | Read |
| `create_label` | `Labels.Create` | Mutation |
| `delete_label` | `Labels.Delete` | Mutation |
| `get_attachment` | `Messages.Attachments.Get` | Read |
| `get_vacation` | `Settings.GetVacation` | Read |
| `update_vacation` | `Settings.UpdateVacation` | Mutation |
| `create_draft` | `Drafts.Create` | Mutation |
| `list_drafts` | `Drafts.List` | Read |
| `get_draft` | `Drafts.Get` | Read |
| `update_draft` | `Drafts.Update` | Mutation |
| `delete_draft` | `Drafts.Delete` | Mutation |
| `send_draft` | `Drafts.Send` | Mutation |

### Gaps

#### High Value

- [x] **Get user profile** -- `Users.GetProfile` (read) -- email address, message/thread counts, history ID
- [x] **List threads** -- `Threads.List` (read) -- thread-based browsing, distinct from message search
- [x] **Trash/Untrash thread** -- `Threads.Trash` / `Threads.Untrash` (mutation) -- direct trash operations on threads
- [x] **Create label** -- `Labels.Create` (mutation) -- create custom labels for organizing email
- [x] **Delete label** -- `Labels.Delete` (mutation) -- remove custom labels
- [x] **Get label details** -- `Labels.Get` (read) -- unread/total counts per label
- [x] **Delete message** -- `Messages.Delete` (mutation) -- permanently remove a message (bypasses trash, irreversible)
- [x] **Get/Update vacation** -- `Settings.GetVacation` / `Settings.UpdateVacation` (both) -- out-of-office auto-reply

#### Medium Value

- [ ] **Update/Patch label** -- `Labels.Update` / `Labels.Patch` (mutation) -- rename labels, change visibility
- [ ] **List history** -- `History.List` (read) -- track mailbox changes since a point in time
- [ ] **Batch delete messages** -- `Messages.BatchDelete` (mutation) -- permanently delete multiple messages (irreversible)
- [ ] **Trash/Untrash message** -- `Messages.Trash` / `Messages.Untrash` (mutation) -- dedicated endpoints (currently done via modify + labels)
- [ ] **Delete thread** -- `Threads.Delete` (mutation) -- permanently delete a thread (irreversible)
- [ ] **List filters** -- `Settings.Filters.List` (read) -- see inbox rules
- [ ] **Create/Delete filter** -- `Settings.Filters.Create` / `Settings.Filters.Delete` (mutation) -- manage inbox rules
- [ ] **List send-as aliases** -- `Settings.SendAs.List` (read) -- discover send-as addresses

#### Low Value

- [ ] Watch/Stop push notifications -- requires webhook infrastructure
- [ ] Import/Insert message -- migration/automation use cases
- [ ] Settings: AutoForwarding, IMAP, POP, Language
- [ ] Forwarding addresses CRUD
- [ ] Send-as CRUD + Verify
- [ ] S/MIME info CRUD
- [ ] Delegates CRUD
- [ ] CSE Identities/Keypairs CRUD

---

## Drive

### Implemented

| Tool | SDK Method(s) | Type |
|------|--------------|------|
| `list_accounts` | Internal auth manager | Read |
| `search_files` | `Files.List` (with Q) | Read |
| `list_files` | `Files.List` (with folder filter) | Read |
| `get_file` | `Files.Get` | Read |
| `read_file` | `Files.Get` (download) + `Files.Export` | Read |
| `upload_file` | `Files.Create` (with media) | Mutation |
| `update_file` | `Files.Update` (metadata) | Mutation |
| `delete_file` | `Files.Delete` + `Files.Update` (trash) | Mutation |
| `create_folder` | `Files.Create` (folder) | Mutation |
| `move_file` | `Files.Update` (parents) | Mutation |
| `copy_file` | `Files.Copy` | Mutation |
| `share_file` | `Permissions.Create` | Mutation |
| `list_permissions` | `Permissions.List` | Read |
| `get_permission` | `Permissions.Get` | Read |
| `update_permission` | `Permissions.Update` | Mutation |
| `delete_permission` | `Permissions.Delete` | Mutation |
| `empty_trash` | `Files.EmptyTrash` | Mutation |
| `get_about` | `About.Get` | Read |
| `list_shared_drives` | `Drives.List` | Read |
| `get_shared_drive` | `Drives.Get` | Read |

### Gaps

#### High Value

- [x] **List permissions** -- `Permissions.List` (read) -- see who has access to a file
- [x] **Get permission** -- `Permissions.Get` (read) -- inspect a specific permission
- [x] **Update permission** -- `Permissions.Update` (mutation) -- change access level (e.g. writer to reader)
- [x] **Delete permission (unshare)** -- `Permissions.Delete` (mutation) -- revoke access
- [x] **Empty trash** -- `Files.EmptyTrash` (mutation) -- clear all trashed files
- [x] **Get about/quota** -- `About.Get` (read) -- storage usage, user info, supported export formats
- [x] **List shared drives** -- `Drives.List` (read) -- discover shared drives
- [x] **Get shared drive** -- `Drives.Get` (read) -- shared drive details

#### Medium Value

- [ ] **List comments** -- `Comments.List` (read) -- view comments on a file
- [ ] **Create comment** -- `Comments.Create` (mutation) -- add feedback to a file
- [ ] **Delete comment** -- `Comments.Delete` (mutation) -- remove a comment
- [ ] **Update comment** -- `Comments.Update` (mutation) -- edit a comment
- [ ] **List replies** -- `Replies.List` (read) -- view replies to a comment
- [ ] **Create reply** -- `Replies.Create` (mutation) -- reply to a comment
- [ ] **List revisions** -- `Revisions.List` (read) -- view file version history
- [ ] **Get revision** -- `Revisions.Get` (read) -- inspect a specific version
- [ ] **Delete revision** -- `Revisions.Delete` (mutation) -- remove a version
- [ ] **List changes** -- `Changes.List` (read) -- track what changed across Drive
- [ ] **Create shared drive** -- `Drives.Create` (mutation) -- create collaboration spaces
- [ ] **Update shared drive** -- `Drives.Update` (mutation) -- manage shared drive settings
- [ ] **Delete shared drive** -- `Drives.Delete` (mutation) -- remove a shared drive

#### Low Value

- [ ] Files.Download -- newer endpoint; current Get+download works fine
- [ ] GenerateIds -- pre-generate file IDs; niche
- [ ] ListLabels / ModifyLabels -- Drive labels (admin-oriented)
- [ ] Watch (files/changes) -- requires webhook infrastructure
- [ ] Access proposals CRUD
- [ ] Approvals CRUD
- [ ] Apps list/get
- [ ] Operations.Get
- [ ] Drives.Hide/Unhide
- [ ] Teamdrives (all deprecated)

---

## Calendar

### Implemented

| Tool | SDK Method(s) | Type |
|------|--------------|------|
| `list_accounts` | Internal auth manager | Read |
| `list_calendars` | `CalendarList.List` | Read |
| `list_events` | `Events.List` | Read |
| `get_event` | `Events.Get` | Read |
| `create_event` | `Events.Insert` | Mutation |
| `update_event` | `Events.Get` + `Events.Update` | Mutation |
| `delete_event` | `Events.Delete` | Mutation |
| `respond_event` | `Events.Get` + `Events.Patch` | Mutation |

### Gaps

#### High Value

- [ ] **Quick add event** -- `Events.QuickAdd` (mutation) -- create event from natural language (e.g. "Lunch with Bob tomorrow at noon")
- [ ] **Query free/busy** -- `Freebusy.Query` (read) -- check availability for users/groups before scheduling
- [ ] **List recurring instances** -- `Events.Instances` (read) -- list individual occurrences of a recurring event
- [ ] **Move event** -- `Events.Move` (mutation) -- move event to a different calendar
- [ ] **Share calendar** -- `Acl.Insert` (mutation) -- share a calendar with another user
- [ ] **List calendar sharing** -- `Acl.List` (read) -- see who has access to a calendar
- [ ] **Create calendar** -- `Calendars.Insert` (mutation) -- create a new calendar
- [ ] **Delete calendar** -- `Calendars.Delete` (mutation) -- remove a calendar

#### Medium Value

- [ ] **Get calendar details** -- `Calendars.Get` (read) -- timezone, description, metadata
- [ ] **Update calendar** -- `Calendars.Update` / `Calendars.Patch` (mutation) -- change name, description, timezone
- [ ] **Clear calendar** -- `Calendars.Clear` (mutation) -- remove all events from a calendar
- [ ] **Get calendar list entry** -- `CalendarList.Get` (read) -- detailed info about a specific calendar
- [ ] **Subscribe to calendar** -- `CalendarList.Insert` (mutation) -- add an existing calendar to user's list
- [ ] **Unsubscribe from calendar** -- `CalendarList.Delete` (mutation) -- remove a calendar from user's list
- [ ] **Update calendar list entry** -- `CalendarList.Update` / `CalendarList.Patch` (mutation) -- color, notifications, visibility
- [ ] **Get/Update/Delete ACL rule** -- `Acl.Get` / `Acl.Update` / `Acl.Delete` (both) -- manage calendar sharing
- [ ] **Get colors** -- `Colors.Get` (read) -- available color palette for calendars/events

#### Low Value

- [ ] Import event -- preserves UID; for migration/sync
- [ ] Watch events/calendars/ACL/settings -- requires webhook infrastructure
- [ ] Channels.Stop
- [ ] Settings List/Get/Watch

---

## Notes

- **Gmail scope:** Uses `MailGoogleComScope` (`https://mail.google.com/`) which is the full-access scope. Required for permanent deletion (`Messages.Delete`, `Threads.Delete`, `Messages.BatchDelete`). It is a superset of `gmail.modify`, `gmail.send`, and `gmail.settings.basic`. Existing users will need to re-authorize after upgrading.
- **Watch/push notification methods** exist across all three APIs but require webhook infrastructure. Not practical for MCP tools. Deprioritize.
- **Sharing/permissions is a cross-cutting gap.** Drive now has full permission CRUD (list, get, create, update, delete). Calendar has no ACL tools, Gmail has no delegation.
- **Settings/admin methods** are consistently low-value for an MCP assistant context.
- **Deprecated services** (e.g. Teamdrives) should be skipped entirely.
