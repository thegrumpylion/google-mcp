package calendar

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/thegrumpylion/google-mcp/internal/auth"
	"github.com/thegrumpylion/google-mcp/internal/server"
	"google.golang.org/api/calendar/v3"
)

// --- share_calendar ---

type shareCalendarInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar ID to share (default: 'primary')"`
	Type       string `json:"type" jsonschema:"Scope type: 'user', 'group', 'domain', or 'default' (public)"`
	Value      string `json:"value,omitempty" jsonschema:"Email address (for user/group) or domain name (for domain). Omit for 'default' (public)."`
	Role       string `json:"role" jsonschema:"Access role: 'freeBusyReader', 'reader', 'writer', or 'owner'"`
}

func registerShareCalendar(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name: "share_calendar",
		Description: `Share a calendar with a user, group, domain, or make it public.

Types:
  - "user" — Share with a specific user (requires value as email)
  - "group" — Share with a Google Group (requires value as email)
  - "domain" — Share with an entire domain (requires value as domain name)
  - "default" — Make calendar public (no value needed)

Roles:
  - "freeBusyReader" — See free/busy only
  - "reader" — See event details
  - "writer" — Edit events
  - "owner" — Full management access`,
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: server.BoolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input shareCalendarInput) (*mcp.CallToolResult, any, error) {
		// Validate type.
		switch input.Type {
		case "user", "group", "domain", "default":
		default:
			return nil, nil, fmt.Errorf("invalid type %q: must be 'user', 'group', 'domain', or 'default'", input.Type)
		}

		// Validate role.
		switch input.Role {
		case "freeBusyReader", "reader", "writer", "owner":
		default:
			return nil, nil, fmt.Errorf("invalid role %q: must be 'freeBusyReader', 'reader', 'writer', or 'owner'", input.Role)
		}

		// Validate value requirement.
		if input.Type != "default" && input.Value == "" {
			return nil, nil, fmt.Errorf("value is required for type %q (email or domain name)", input.Type)
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		calendarID := input.CalendarID
		if calendarID == "" {
			calendarID = "primary"
		}

		rule := &calendar.AclRule{
			Role: input.Role,
			Scope: &calendar.AclRuleScope{
				Type:  input.Type,
				Value: input.Value,
			},
		}

		created, err := svc.Acl.Insert(calendarID, rule).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("sharing calendar: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Calendar shared.\n\nRule ID: %s\nRole: %s\nScope: %s (%s)",
					created.Id, created.Role, created.Scope.Value, created.Scope.Type)},
			},
		}, nil, nil
	})
}

// --- list_calendar_sharing ---

type listCalendarSharingInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
}

func registerListCalendarSharing(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "list_calendar_sharing",
		Description: "List all sharing rules (ACL) for a calendar. Shows who has access and their role.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listCalendarSharingInput) (*mcp.CallToolResult, any, error) {
		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		calendarID := input.CalendarID
		if calendarID == "" {
			calendarID = "primary"
		}

		resp, err := svc.Acl.List(calendarID).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("listing calendar sharing: %w", err)
		}

		var sb strings.Builder
		if len(resp.Items) == 0 {
			sb.WriteString("No sharing rules found.")
		} else {
			fmt.Fprintf(&sb, "Found %d sharing rules:\n\n", len(resp.Items))
			for _, rule := range resp.Items {
				scope := rule.Scope.Value
				if scope == "" {
					scope = "(public)"
				}
				fmt.Fprintf(&sb, "- Rule ID: %s\n  Role: %s\n  Scope: %s (%s)\n\n",
					rule.Id, rule.Role, scope, rule.Scope.Type)
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- get_acl_rule ---

type getACLRuleInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
	RuleID     string `json:"rule_id" jsonschema:"ACL rule ID (from list_calendar_sharing)"`
}

func registerGetACLRule(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "get_acl_rule",
		Description: "Get details of a specific calendar sharing rule (ACL entry) by ID.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input getACLRuleInput) (*mcp.CallToolResult, any, error) {
		if input.RuleID == "" {
			return nil, nil, fmt.Errorf("rule_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		calendarID := input.CalendarID
		if calendarID == "" {
			calendarID = "primary"
		}

		rule, err := svc.Acl.Get(calendarID, input.RuleID).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting ACL rule: %w", err)
		}

		scope := rule.Scope.Value
		if scope == "" {
			scope = "(public)"
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Rule ID: %s\n", rule.Id)
		fmt.Fprintf(&sb, "Role: %s\n", rule.Role)
		fmt.Fprintf(&sb, "Scope type: %s\n", rule.Scope.Type)
		fmt.Fprintf(&sb, "Scope value: %s\n", scope)
		if rule.Etag != "" {
			fmt.Fprintf(&sb, "ETag: %s\n", rule.Etag)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	})
}

// --- update_acl_rule ---

type updateACLRuleInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
	RuleID     string `json:"rule_id" jsonschema:"ACL rule ID to update (from list_calendar_sharing)"`
	Role       string `json:"role" jsonschema:"New access role: 'freeBusyReader', 'reader', 'writer', or 'owner'"`
}

func registerUpdateACLRule(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "update_acl_rule",
		Description: "Update the role of an existing calendar sharing rule (ACL entry). Changes the access level for the user, group, or domain.",
		Annotations: &mcp.ToolAnnotations{
			IdempotentHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input updateACLRuleInput) (*mcp.CallToolResult, any, error) {
		if input.RuleID == "" {
			return nil, nil, fmt.Errorf("rule_id is required")
		}

		switch input.Role {
		case "freeBusyReader", "reader", "writer", "owner":
		default:
			return nil, nil, fmt.Errorf("invalid role %q: must be 'freeBusyReader', 'reader', 'writer', or 'owner'", input.Role)
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		calendarID := input.CalendarID
		if calendarID == "" {
			calendarID = "primary"
		}

		// Fetch current rule to preserve scope.
		current, err := svc.Acl.Get(calendarID, input.RuleID).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("getting ACL rule: %w", err)
		}

		current.Role = input.Role

		updated, err := svc.Acl.Update(calendarID, input.RuleID, current).Do()
		if err != nil {
			return nil, nil, fmt.Errorf("updating ACL rule: %w", err)
		}

		scope := updated.Scope.Value
		if scope == "" {
			scope = "(public)"
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("ACL rule updated.\n\nRule ID: %s\nRole: %s\nScope: %s (%s)",
					updated.Id, updated.Role, scope, updated.Scope.Type)},
			},
		}, nil, nil
	})
}

// --- delete_acl_rule ---

type deleteACLRuleInput struct {
	Account    string `json:"account" jsonschema:"Account name"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar ID (default: 'primary')"`
	RuleID     string `json:"rule_id" jsonschema:"ACL rule ID to delete (from list_calendar_sharing)"`
}

func registerDeleteACLRule(srv *server.Server, mgr *auth.Manager) {
	server.AddTool(srv, &mcp.Tool{
		Name:        "delete_acl_rule",
		Description: "Delete a calendar sharing rule (ACL entry), revoking access for the specified user, group, or domain.",
		Annotations: &mcp.ToolAnnotations{},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input deleteACLRuleInput) (*mcp.CallToolResult, any, error) {
		if input.RuleID == "" {
			return nil, nil, fmt.Errorf("rule_id is required")
		}

		svc, err := newService(ctx, mgr, input.Account)
		if err != nil {
			return nil, nil, fmt.Errorf("creating Calendar service: %w", err)
		}

		calendarID := input.CalendarID
		if calendarID == "" {
			calendarID = "primary"
		}

		if err := svc.Acl.Delete(calendarID, input.RuleID).Do(); err != nil {
			return nil, nil, fmt.Errorf("deleting ACL rule: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("ACL rule %s deleted.", input.RuleID)},
			},
		}, nil, nil
	})
}
