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
| Gmail    |    36 |                  34 |                80 |      43% |
| Drive    |    27 |                  28 |                58 |      48% |
| Calendar |    27 |                  28 |                38 |      74% |
| **Total**| **90**|              **90** |           **176** |  **~51%**|

Additionally, 2 **local file tools** (`list_local_files`, `read_local_file`) are conditionally registered on all servers when `--allow-read-dir` or `--allow-write-dir` is set. These are not counted above as they don't map to Google API methods.

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
| `get_attachment` | `Messages.Attachments.Get` (+ optional `save_to` local file) | Read |
| `get_vacation` | `Settings.GetVacation` | Read |
| `update_vacation` | `Settings.UpdateVacation` | Mutation |
| `create_draft` | `Drafts.Create` | Mutation |
| `list_drafts` | `Drafts.List` | Read |
| `get_draft` | `Drafts.Get` | Read |
| `update_draft` | `Drafts.Update` | Mutation |
| `delete_draft` | `Drafts.Delete` | Mutation |
| `send_draft` | `Drafts.Send` | Mutation |
| `save_attachment_to_drive` | `Messages.Attachments.Get` + Drive `Files.Create` | Mutation (cross-service) |
| `update_label` | `Labels.Patch` | Mutation |
| `list_history` | `History.List` | Read |
| `trash_message` | `Messages.Trash` | Mutation |
| `untrash_message` | `Messages.Untrash` | Mutation |
| `batch_delete_messages` | `Messages.BatchDelete` | Mutation |
| `delete_thread` | `Threads.Delete` | Mutation |
| `list_filters` | `Settings.Filters.List` | Read |
| `create_filter` | `Settings.Filters.Create` | Mutation |
| `delete_filter` | `Settings.Filters.Delete` | Mutation |
| `list_send_as` | `Settings.SendAs.List` | Read |

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

- [x] **Update/Patch label** -- `Labels.Patch` (mutation) -- rename labels, change visibility
- [x] **List history** -- `History.List` (read) -- track mailbox changes since a point in time
- [x] **Batch delete messages** -- `Messages.BatchDelete` (mutation) -- permanently delete multiple messages (irreversible)
- [x] **Trash/Untrash message** -- `Messages.Trash` / `Messages.Untrash` (mutation) -- dedicated endpoints
- [x] **Delete thread** -- `Threads.Delete` (mutation) -- permanently delete a thread (irreversible)
- [x] **List filters** -- `Settings.Filters.List` (read) -- see inbox rules
- [x] **Create/Delete filter** -- `Settings.Filters.Create` / `Settings.Filters.Delete` (mutation) -- manage inbox rules
- [x] **List send-as aliases** -- `Settings.SendAs.List` (read) -- discover send-as addresses

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
| `read_file` | `Files.Get` (download) + `Files.Export` (+ optional `save_to` local file) | Read |
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
| `create_shared_drive` | `Drives.Create` | Mutation |
| `update_shared_drive` | `Drives.Update` | Mutation |
| `delete_shared_drive` | `Drives.Delete` | Mutation |
| `list_revisions` | `Revisions.List` | Read |
| `get_revision` | `Revisions.Get` | Read |
| `delete_revision` | `Revisions.Delete` | Mutation |
| `list_changes` | `Changes.List` + `Changes.GetStartPageToken` | Read |

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
- [x] **List revisions** -- `Revisions.List` (read) -- view file version history
- [x] **Get revision** -- `Revisions.Get` (read) -- inspect a specific version
- [x] **Delete revision** -- `Revisions.Delete` (mutation) -- remove a version
- [x] **List changes** -- `Changes.List` (read) -- track what changed across Drive
- [x] **Create shared drive** -- `Drives.Create` (mutation) -- create collaboration spaces
- [x] **Update shared drive** -- `Drives.Update` (mutation) -- manage shared drive settings
- [x] **Delete shared drive** -- `Drives.Delete` (mutation) -- remove a shared drive

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
| `create_calendar` | `Calendars.Insert` | Mutation |
| `delete_calendar` | `Calendars.Delete` | Mutation |
| `list_events` | `Events.List` | Read |
| `get_event` | `Events.Get` | Read |
| `create_event` | `Events.Insert` | Mutation |
| `update_event` | `Events.Get` + `Events.Update` | Mutation |
| `delete_event` | `Events.Delete` | Mutation |
| `respond_event` | `Events.Get` + `Events.Patch` | Mutation |
| `quick_add_event` | `Events.QuickAdd` | Mutation |
| `list_event_instances` | `Events.Instances` | Read |
| `move_event` | `Events.Move` | Mutation |
| `query_free_busy` | `Freebusy.Query` | Read |
| `get_calendar` | `Calendars.Get` | Read |
| `update_calendar` | `Calendars.Get` + `Calendars.Update` | Mutation |
| `clear_calendar` | `Calendars.Clear` | Mutation |
| `get_calendar_list_entry` | `CalendarList.Get` | Read |
| `subscribe_calendar` | `CalendarList.Insert` | Mutation |
| `unsubscribe_calendar` | `CalendarList.Delete` | Mutation |
| `update_calendar_list_entry` | `CalendarList.Get` + `CalendarList.Update` | Mutation |
| `share_calendar` | `Acl.Insert` | Mutation |
| `list_calendar_sharing` | `Acl.List` | Read |
| `get_acl_rule` | `Acl.Get` | Read |
| `update_acl_rule` | `Acl.Get` + `Acl.Update` | Mutation |
| `delete_acl_rule` | `Acl.Delete` | Mutation |
| `get_colors` | `Colors.Get` | Read |

### Gaps

#### High Value

- [x] **Quick add event** -- `Events.QuickAdd` (mutation) -- create event from natural language (e.g. "Lunch with Bob tomorrow at noon")
- [x] **Query free/busy** -- `Freebusy.Query` (read) -- check availability for users/groups before scheduling
- [x] **List recurring instances** -- `Events.Instances` (read) -- list individual occurrences of a recurring event
- [x] **Move event** -- `Events.Move` (mutation) -- move event to a different calendar
- [x] **Share calendar** -- `Acl.Insert` (mutation) -- share a calendar with another user
- [x] **List calendar sharing** -- `Acl.List` (read) -- see who has access to a calendar
- [x] **Create calendar** -- `Calendars.Insert` (mutation) -- create a new calendar
- [x] **Delete calendar** -- `Calendars.Delete` (mutation) -- remove a calendar

#### Medium Value

- [x] **Get calendar details** -- `Calendars.Get` (read) -- timezone, description, metadata
- [x] **Update calendar** -- `Calendars.Update` / `Calendars.Patch` (mutation) -- change name, description, timezone
- [x] **Clear calendar** -- `Calendars.Clear` (mutation) -- remove all events from a calendar
- [x] **Get calendar list entry** -- `CalendarList.Get` (read) -- detailed info about a specific calendar
- [x] **Subscribe to calendar** -- `CalendarList.Insert` (mutation) -- add an existing calendar to user's list
- [x] **Unsubscribe from calendar** -- `CalendarList.Delete` (mutation) -- remove a calendar from user's list
- [x] **Update calendar list entry** -- `CalendarList.Update` / `CalendarList.Patch` (mutation) -- color, notifications, visibility
- [x] **Get/Update/Delete ACL rule** -- `Acl.Get` / `Acl.Update` / `Acl.Delete` (both) -- manage calendar sharing
- [x] **Get colors** -- `Colors.Get` (read) -- available color palette for calendars/events

#### Low Value

- [ ] Import event -- preserves UID; for migration/sync
- [ ] Watch events/calendars/ACL/settings -- requires webhook infrastructure
- [ ] Channels.Stop
- [ ] Settings List/Get/Watch

---

## Notes

- **Gmail scope:** Uses `MailGoogleComScope` (`https://mail.google.com/`) which is the full-access scope. Required for permanent deletion (`Messages.Delete`, `Threads.Delete`, `Messages.BatchDelete`). It is a superset of `gmail.modify`, `gmail.send`, and `gmail.settings.basic`. Existing users will need to re-authorize after upgrading.
- **Watch/push notification methods** exist across all three APIs but require webhook infrastructure. Not practical for MCP tools. Deprioritize.
- **Calendar scope:** Uses `CalendarScope` (`https://www.googleapis.com/auth/calendar`) which is the full-access scope. Required for ACL operations and calendar CRUD. It is a superset of `calendar.readonly` and `calendar.events`. Existing users will need to re-authorize after upgrading.
- **Sharing/permissions is a cross-cutting gap.** Drive now has full permission CRUD (list, get, create, update, delete). Calendar has ACL insert + list. Gmail has no delegation.
- **Settings/admin methods** are consistently low-value for an MCP assistant context.
- **Deprecated services** (e.g. Teamdrives) should be skipped entirely.
- **Attachments:** `send_message`, `create_draft`, and `update_draft` support both inline base64 attachments and Google Drive file references (`drive_attachments`). Drive attachments are resolved server-side — file bytes never enter the LLM context window.
- **Cross-service bridge:** The `internal/bridge` package provides `SaveAttachmentToDrive` and `ReadDriveFile` functions that transfer data between Gmail and Drive server-side. The gmail server includes Drive scope for this purpose. The `save_attachment_to_drive` tool and the `drive_attachments` compose field use this package.
