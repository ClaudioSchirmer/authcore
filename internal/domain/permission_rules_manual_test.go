package domain

import (
	"testing"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// description-differs-from-key is the only rule in permission_rules_manual.go,
// and the only logic in this entity with no generated test behind it — which is
// exactly why it is the one worth testing by hand.
//
// It reuses validPermission(), permissionBlames() and stubPermissionService
// from the generated permission_test.go: one fixture, so a change to the
// aggregate's valid shape cannot leave this file asserting against a stale one.

// permissionWithDescription is validPermission() with one thing changed, so a
// failure points at the rule under test rather than at unrelated invalid state.
func permissionWithDescription(description string) *Permission {
	e := validPermission()
	e.Description = vos.Description(description)
	return e
}

// The rule fires when the description says nothing the key does not already
// say. Every value below is a normalized match for "tenant:read" — that is the
// point of normalizing rather than comparing literally.
func TestPermissionDescriptionMayNotEchoTheKey(t *testing.T) {
	for _, tc := range []struct {
		description string
		why         string
	}{
		{"tenant:read", "the key pasted verbatim"},
		{"Tenant:Read", "case-folded"},
		{"TENANT:READ", "…in any case"},
		{"tenant read", "the colon written as a space"},
		{"tenant-read", "…or as a hyphen"},
		{"  tenant : read  ", "padded and spaced out"},
		{"tenantread", "run together"},
	} {
		e := permissionWithDescription(tc.description)
		_, err := domain.GetInsertable(e, &stubPermissionService{}, "GetInsertable")
		if err == nil {
			t.Errorf("description %q was accepted (%s) — it only echoes the key", tc.description, tc.why)
			continue
		}
		if !permissionBlames(err, "Description") {
			t.Errorf("description %q: the rejection should name Description, it named %v",
				tc.description, permissionRejectedFields(err))
		}
	}
}

// A description that actually explains the permission is what the rule exists
// to let through. The catch is deliberately narrow: it targets the lazy paste
// and nothing more.
func TestPermissionDescriptionThatExplainsIsAccepted(t *testing.T) {
	for _, description := range []string{
		"Read tenants: list the tenant registry and fetch a tenant by id.",
		"Allows the holder to list every tenant and fetch one by its identifier.",
		"Leitura de tenants: lista o registro e busca um tenant por id.",
		// It mentions the key and still says something — only an exact
		// normalized match is refused.
		"tenant:read — lets an operator browse the tenant registry.",
	} {
		e := permissionWithDescription(description)
		if _, err := domain.GetInsertable(e, &stubPermissionService{}, "GetInsertable"); err != nil {
			t.Errorf("description %q was rejected: %v (fields: %v)",
				description, err, permissionRejectedFields(err))
		}
	}
}

// The rule runs on UPDATE too, not only on insert — the description is the one
// editable field, so the echo it guards against is likelier there than anywhere.
func TestPermissionDescriptionEchoIsRefusedOnUpdateToo(t *testing.T) {
	e := validPermission()
	_, err := domain.GetUpdatable(e, func(x *Permission) error {
		x.Description = vos.Description("tenant read")
		return nil
	}, &stubPermissionService{}, "GetUpdatable")
	if err == nil {
		t.Fatal("an echoing description was accepted on update")
	}
	if !permissionBlames(err, "Description") {
		t.Errorf("the rejection should name Description, it named %v", permissionRejectedFields(err))
	}
}

// The rule compares against the key as the value object renders it, so a
// hierarchical resource is folded the same way the wire form is.
func TestPermissionDescriptionEchoFollowsTheRenderedKey(t *testing.T) {
	e := validPermission()
	e.Key = vos.PermissionKey{Resource: "user:profile", Action: "read"}
	e.Description = vos.Description("user profile read please")

	// Not an exact normalized match, so it passes.
	if _, err := domain.GetInsertable(e, &stubPermissionService{}, "GetInsertable"); err != nil {
		t.Fatalf("a non-echoing description was rejected: %v", err)
	}

	e = validPermission()
	e.Key = vos.PermissionKey{Resource: "user:profile", Action: "read"}
	e.Description = vos.Description("User Profile Read")
	if _, err := domain.GetInsertable(e, &stubPermissionService{}, "GetInsertable"); err == nil {
		t.Error("a description echoing the rendered hierarchical key was accepted")
	}
}

// The empty case belongs to the value object, not to this rule: vos.Description
// answers an empty value with RequiredFieldNotification on the same pass, and a
// second complaint about one field would be noise.
//
// The guard matters because normalizing "" yields "", which would otherwise
// equal the normalization of an empty key and fire a second, misleading
// complaint.
func TestPermissionEmptyDescriptionIsTheValueObjectsProblem(t *testing.T) {
	e := permissionWithDescription("")
	_, err := domain.GetInsertable(e, &stubPermissionService{}, "GetInsertable")
	if err == nil {
		t.Fatal("an empty description was accepted")
	}
	// The complaint about Description comes from the value object.
	if !permissionBlames(err, "Description") {
		t.Errorf("the rejection should name Description, it named %v", permissionRejectedFields(err))
	}
}

// normalizeKeyForComparison is a COMPARISON helper — nothing is ever persisted
// in this form. It drops colons on top of the shared normalization, which is
// what lets a rendered key and a prose paste of it collapse together.
func TestNormalizeKeyForComparison(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"tenant:read", "tenantread"},
		{"Tenant: Read", "tenantread"},
		{"tenant-read", "tenantread"},
		{"  tenant read  ", "tenantread"},
		{"user:profile:read", "userprofileread"},
		{"", ""},
	} {
		if got := normalizeKeyForComparison(tc.in); got != tc.want {
			t.Errorf("normalizeKeyForComparison(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
