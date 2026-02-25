# Google MCP API Coverage

Tracking document for SDK method coverage across all three MCP servers.

**Prerequisite:** Complete all items in [breaking-changes.md](breaking-changes.md) before adding new features. The breaking changes establish naming conventions, annotations, and shared infrastructure that all new tools must follow.

Last updated: 2026-02-25

## Summary

| Server   | Tools | SDK Methods Covered | Total SDK Methods | Coverage |
|----------|-------|--------------------:|------------------:|---------:|
| Gmail    |    16 |                  15 |                80 |      19% |
| Drive    |    12 |                  13 |                58 |      22% |
| Calendar |     8 |                   8 |                38 |      21% |
| **Total**| **36**|              **36** |           **176** |  **~20%**|

---

## Gmail

### Implemented

| Tool | SDK Method(s) | Type |
|------|--------------|------|
| `accounts_list` | Internal auth manager | Read |
| `search` | `Messages.List` + `Messages.Get` | Read |
| `read` | `Messages.Get` (full) | Read |
| `read_thread` | `Threads.Get` (full) | Read |
| `thread_modify` | `Threads.Modify` | Mutation |
| `send` | `Messages.Send` | Mutation |
| `list_labels` | `Labels.List` | Read |
| `modify` | `Messages.Modify` + `Messages.BatchModify` | Mutation |
| `get_attachment` | `Messages.Attachments.Get` | Read |
| `draft_create` | `Drafts.Create` | Mutation |
| `draft_list` | `Drafts.List` | Read |
| `draft_get` | `Drafts.Get` | Read |
| `draft_update` | `Drafts.Update` | Mutation |
| `draft_delete` | `Drafts.Delete` | Mutation |
| `draft_send` | `Drafts.Send` | Mutation |

### Gaps

#### High Value

- [ ] **Get user profile** -- `Users.GetProfile` (read) -- email address, message/thread counts, history ID
- [ ] **List threads** -- `Threads.List` (read) -- thread-based browsing, distinct from message search
- [ ] **Trash/Untrash thread** -- `Threads.Trash` / `Threads.Untrash` (mutation) -- direct trash operations on threads
- [ ] **Create label** -- `Labels.Create` (mutation) -- create custom labels for organizing email
- [ ] **Delete label** -- `Labels.Delete` (mutation) -- remove custom labels
- [ ] **Get label details** -- `Labels.Get` (read) -- unread/total counts per label
- [ ] **Delete message** -- `Messages.Delete` (mutation) -- permanently remove a message (bypasses trash, irreversible)
- [ ] **Get/Update vacation** -- `Settings.GetVacation` / `Settings.UpdateVacation` (both) -- out-of-office auto-reply

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
| `accounts_list` | Internal auth manager | Read |
| `search` | `Files.List` (with Q) | Read |
| `list` | `Files.List` (with folder filter) | Read |
| `get` | `Files.Get` | Read |
| `read` | `Files.Get` (download) + `Files.Export` | Read |
| `upload` | `Files.Create` (with media) | Mutation |
| `update` | `Files.Update` (metadata) | Mutation |
| `delete` | `Files.Delete` + `Files.Update` (trash) | Mutation |
| `create_folder` | `Files.Create` (folder) | Mutation |
| `move` | `Files.Update` (parents) | Mutation |
| `copy` | `Files.Copy` | Mutation |
| `share` | `Permissions.Create` | Mutation |

### Gaps

#### High Value

- [ ] **List permissions** -- `Permissions.List` (read) -- see who has access to a file
- [ ] **Get permission** -- `Permissions.Get` (read) -- inspect a specific permission
- [ ] **Update permission** -- `Permissions.Update` (mutation) -- change access level (e.g. writer to reader)
- [ ] **Delete permission (unshare)** -- `Permissions.Delete` (mutation) -- revoke access
- [ ] **Empty trash** -- `Files.EmptyTrash` (mutation) -- clear all trashed files
- [ ] **Get about/quota** -- `About.Get` (read) -- storage usage, user info, supported export formats
- [ ] **List shared drives** -- `Drives.List` (read) -- discover shared drives
- [ ] **Get shared drive** -- `Drives.Get` (read) -- shared drive details

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
| `accounts_list` | Internal auth manager | Read |
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

- **Watch/push notification methods** exist across all three APIs but require webhook infrastructure. Not practical for MCP tools. Deprioritize.
- **Sharing/permissions is a cross-cutting gap.** Drive can share but not inspect/revoke, Calendar has no ACL tools, Gmail has no delegation.
- **Settings/admin methods** are consistently low-value for an MCP assistant context.
- **Deprecated services** (e.g. Teamdrives) should be skipped entirely.
