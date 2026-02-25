# Cross-Service Integration Ideas

Tracking document for potential bridge tools and cross-service features across Gmail, Drive, and Calendar.

Review after Phase 2 API gaps are complete.

Last updated: 2026-02-25

---

## Implemented

| Feature | Tool / Field | Direction | Location |
|---------|-------------|-----------|----------|
| Drive file as email attachment | `drive_attachments` on compose tools | Drive → Gmail | `internal/gmail/bridge.go` |
| Save email attachment to Drive | `save_attachment_to_drive` | Gmail → Drive | `internal/gmail/bridge.go` + `internal/bridge/bridge.go` |

---

## Tier 1 — High Value

### Drive attachments on calendar events

Attach Drive files to calendar events (meeting agendas, decks, notes). Same pattern as `drive_attachments` on compose — the Calendar API supports event attachments natively.

- **Direction:** Drive read → Calendar write
- **Scope:** Extend `create_event` / `update_event` with a `drive_attachments` field
- **Complexity:** Low — follows proven pattern from email attachments
- **Notes:** Requires `supportsAttachments=true` on the API call. Calendar API uses `Event.Attachments` field.

### Save email/thread to Drive

Export an email message or thread as a text file to Drive — for archival, sharing with people outside the inbox, or reference.

- **Direction:** Gmail read → Drive write
- **Tool:** `save_message_to_drive` or `save_thread_to_drive`
- **Complexity:** Low — similar to `save_attachment_to_drive`, just formatting the message body instead of downloading an attachment
- **Notes:** Could support multiple formats (plain text, HTML). Cross-account: read from one inbox, save to another account's Drive.

### Create event from email

Extract participants from an email's From/To/Cc headers and pre-populate a calendar event with attendees, subject as title, and a link back to the email.

- **Direction:** Gmail read → Calendar write
- **Tool:** `create_event_from_message`
- **Complexity:** Medium — header parsing, attendee dedup, sensible defaults for time
- **Notes:** The LLM can already do this as a multi-step workflow (read_message → create_event). A dedicated tool adds convenience but may not be necessary. Revisit based on usage patterns.

### Send event materials to attendees

Grab an event's attendee list from Calendar, then send an email with a Drive file attached (e.g. pre-read, agenda, slides).

- **Direction:** Calendar read + Drive read → Gmail send
- **Tool:** `send_to_attendees`
- **Complexity:** Medium — combines 3 services in one call
- **Notes:** The LLM can orchestrate this today with get_event → send_message + drive_attachments. A dedicated tool saves round-trips but adds complexity. Revisit based on usage patterns.

---

## Tier 2 — Medium Value

### Create meeting notes document

Create a Google Doc in Drive pre-populated with event title, date, attendee list — ready for meeting notes. Optionally attach the doc back to the event.

- **Direction:** Calendar read → Drive write (→ Calendar write if linking back)
- **Tool:** `create_meeting_notes`
- **Complexity:** Medium
- **Notes:** Pairs well with "Drive attachments on calendar events" (Tier 1). Could place doc in a configurable folder.

### Share file with custom notification

When sharing a Drive file, also send a custom email explaining context — richer than Google's generic "X shared a file with you".

- **Direction:** Drive write + Gmail send
- **Tool:** Combined `share_and_notify` or keep as LLM two-step (`share_file` + `send_message`)
- **Complexity:** Low
- **Notes:** Already achievable as a multi-step workflow. A combined tool adds marginal value. Likely skip unless there's demand.

### Bulk save attachments to Drive

Scan emails matching a query, save all attachments to a Drive folder. E.g. "save all PDFs from invoices@vendor.com to my Invoices folder".

- **Direction:** Gmail read → Drive write (batch)
- **Tool:** `bulk_save_attachments`
- **Complexity:** Medium — search + iterate + batch upload, needs progress reporting
- **Notes:** `save_attachment_to_drive` handles one at a time today. A batch tool would be significantly faster for large mailboxes.

---

## Tier 3 — Nice to Have

### Send availability by email

Query free/busy for a set of users, format as a readable table, send via email.

- **Direction:** Calendar read → Gmail send
- **Prerequisite:** `query_free_busy` tool (not yet implemented)
- **Notes:** Low priority. The LLM can format free/busy output itself once the tool exists.

### Cross-service unified search

Search Gmail, Drive, and Calendar in parallel for a topic, return unified results.

- **Direction:** All three services (read)
- **Notes:** The LLM already has access to all servers and can call them in parallel. A unified tool would save a round-trip but adds complexity for marginal gain. Likely not worth building.
