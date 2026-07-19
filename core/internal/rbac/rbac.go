package rbac

import (
	"context"

	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

// Action constants for permission checks.
const (
	ActionRead   = "read"   // GET endpoints
	ActionWrite  = "write"  // POST/PUT endpoints (tasks, workers, teams)
	ActionAdmin  = "admin"  // token management, policy management, audit
	ActionDelete = "delete" // DELETE endpoints
)

// permissions maps role → allowed actions.
var permissions = map[string]map[string]bool{
	protocol.RoleOwner:  {ActionRead: true, ActionWrite: true, ActionAdmin: true, ActionDelete: true},
	protocol.RoleAdmin:  {ActionRead: true, ActionWrite: true, ActionDelete: true},
	protocol.RoleViewer: {ActionRead: true},
}

// Enforcer checks role-based access control.
type Enforcer struct {
	store store.Store
}

// New creates a new RBAC Enforcer.
func New(s store.Store) *Enforcer {
	return &Enforcer{store: s}
}

// Check returns true if the subject has permission to perform the action in the org.
// Returns true if no role bindings exist for the org (dev mode / open access).
func (e *Enforcer) Check(ctx context.Context, orgID, subject, action string) bool {
	bindings := e.store.ListRoleBindingsByOrg(ctx, orgID)
	if len(bindings) == 0 {
		return true // no RBAC configured → allow all (dev mode)
	}

	rb, err := e.store.FindRoleBinding(ctx, orgID, subject)
	if err != nil {
		return false
	}

	perms, ok := permissions[rb.Role]
	if !ok {
		return false
	}
	return perms[action]
}

// RoleFor returns the role for a subject in an org, or empty string if not found.
func (e *Enforcer) RoleFor(ctx context.Context, orgID, subject string) string {
	rb, err := e.store.FindRoleBinding(ctx, orgID, subject)
	if err != nil {
		return ""
	}
	return rb.Role
}

// HasRole checks if the given role allows the specified action.
func HasRole(role, action string) bool {
	perms, ok := permissions[role]
	if !ok {
		return false
	}
	return perms[action]
}
