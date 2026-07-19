package rbac_test

import (
	"context"
	"testing"
	"time"

	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/rbac"
	"github.com/kienbui1995/magic/core/internal/store"
)

func setup(t *testing.T) (*rbac.Enforcer, store.Store) {
	s := store.NewMemoryStore()
	return rbac.New(s), s
}

func TestEnforcer_DevMode_NoBindings(t *testing.T) {
	e, _ := setup(t)
	// No role bindings → allow all (dev mode)
	if !e.Check(context.Background(), "org1", "anyone", rbac.ActionAdmin) {
		t.Error("dev mode should allow all actions")
	}
}

func TestEnforcer_Owner(t *testing.T) {
	e, s := setup(t)
	s.AddRoleBinding(context.Background(), &protocol.RoleBinding{
		ID: "rb1", OrgID: "org1", Subject: "user_alice", Role: protocol.RoleOwner, CreatedAt: time.Now(),
	})

	for _, action := range []string{rbac.ActionRead, rbac.ActionWrite, rbac.ActionAdmin, rbac.ActionDelete} {
		if !e.Check(context.Background(), "org1", "user_alice", action) {
			t.Errorf("owner should have %s permission", action)
		}
	}
}

func TestEnforcer_Admin(t *testing.T) {
	e, s := setup(t)
	s.AddRoleBinding(context.Background(), &protocol.RoleBinding{
		ID: "rb1", OrgID: "org1", Subject: "user_bob", Role: protocol.RoleAdmin, CreatedAt: time.Now(),
	})

	if !e.Check(context.Background(), "org1", "user_bob", rbac.ActionWrite) {
		t.Error("admin should have write permission")
	}
	if e.Check(context.Background(), "org1", "user_bob", rbac.ActionAdmin) {
		t.Error("admin should NOT have admin permission")
	}
}

func TestEnforcer_Viewer(t *testing.T) {
	e, s := setup(t)
	s.AddRoleBinding(context.Background(), &protocol.RoleBinding{
		ID: "rb1", OrgID: "org1", Subject: "user_carol", Role: protocol.RoleViewer, CreatedAt: time.Now(),
	})

	if !e.Check(context.Background(), "org1", "user_carol", rbac.ActionRead) {
		t.Error("viewer should have read permission")
	}
	if e.Check(context.Background(), "org1", "user_carol", rbac.ActionWrite) {
		t.Error("viewer should NOT have write permission")
	}
}

func TestEnforcer_UnknownSubject(t *testing.T) {
	e, s := setup(t)
	// Add a binding so org is not in dev mode
	s.AddRoleBinding(context.Background(), &protocol.RoleBinding{
		ID: "rb1", OrgID: "org1", Subject: "user_alice", Role: protocol.RoleOwner, CreatedAt: time.Now(),
	})

	if e.Check(context.Background(), "org1", "unknown_user", rbac.ActionRead) {
		t.Error("unknown subject should be denied")
	}
}

func TestEnforcer_RoleFor(t *testing.T) {
	e, s := setup(t)
	s.AddRoleBinding(context.Background(), &protocol.RoleBinding{
		ID: "rb1", OrgID: "org1", Subject: "user_alice", Role: protocol.RoleOwner, CreatedAt: time.Now(),
	})

	if role := e.RoleFor(context.Background(), "org1", "user_alice"); role != protocol.RoleOwner {
		t.Errorf("expected owner, got %q", role)
	}
	if role := e.RoleFor(context.Background(), "org1", "nobody"); role != "" {
		t.Errorf("expected empty, got %q", role)
	}
}
