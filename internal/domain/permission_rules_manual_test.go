// Tests for the hand-written rule in permission_rules_manual.go.
//
// It is hand-written because the rule is: the generator declared it as an
// invariant the spec language could not express, wrote a stub, and wrote no
// test. This is the only place it is proven.
//
// The harness — validPermission, stubPermissionService, permissionBlames,
// permissionRejectedFields — comes from the generated permission_test.go in
// this same package, so a fixture change lands in one place.

package domain

import (
	"errors"
	"reflect"
	"testing"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// permissionAnswers is the notification TYPE names a rejection carries.
//
// permissionBlames answers which FIELD was named; this answers which problem
// was reported, which is what separates "the description echoes the key" from
// "the description is junk" when both would blame the same field.
func permissionAnswers(err error) []string {
	var carrier domain.NotificationCarrier
	if !errors.As(err, &carrier) {
		return nil
	}
	var out []string
	for _, ctx := range carrier.NotificationContexts() {
		for _, msg := range ctx.Messages() {
			t := reflect.TypeOf(msg.Notification)
			for t != nil && t.Kind() == reflect.Ptr {
				t = t.Elem()
			}
			if t != nil {
				out = append(out, t.Name())
			}
		}
	}
	return out
}

// permissionEchoed returns the value the named notification carried back, or ""
// when it carried none. FieldValue is the only place a rejection says WHICH
// value it refused, and a composite reaches it through fmt.Stringer — so an
// empty answer here means either the notification did not fire or the echo was
// silenced, and the caller distinguishes them with permissionRaised.
func permissionEchoed(err error, notification string) string {
	var carrier domain.NotificationCarrier
	if !errors.As(err, &carrier) {
		return ""
	}
	for _, ctx := range carrier.NotificationContexts() {
		for _, msg := range ctx.Messages() {
			if domain.NotificationKey(msg.Notification) == notification {
				return msg.FieldValue
			}
		}
	}
	return ""
}

func permissionRaised(err error, notification string) bool {
	for _, a := range permissionAnswers(err) {
		if a == notification {
			return true
		}
	}
	return false
}

// persistedPermission is validPermission as it comes back OUT of the database:
// with an id. An update acts on a row that already exists, and the framework
// validates that id like any other field — so a fixture without one fails on
// the id before reaching the rule under test.
func persistedPermission() *Permission {
	e := validPermission()
	e.SetID(domain.NewID("0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410"))
	return e
}

// echoingKey renders to "permission:archive": 18 runes and two words, so it
// passes the Description value object on its own merits. That matters — a
// shorter key like "tenant:read" is refused for being too short before the
// echo rule is the reason for anything, and a test that cannot tell those
// apart proves nothing.
var echoingKey = vos.PermissionKey{Resource: "permission", Action: "archive"}

// ── description-does-not-echo-key ────────────────────────────────────────────

// The lazy paste: a description that merely restates the key teaches an
// operator nothing the listing did not already show them.
func TestPermissionDescriptionMayNotEchoTheKey(t *testing.T) {
	e := validPermission()
	e.Permission = echoingKey
	e.Description = vos.Description(echoingKey.String())

	_, err := domain.GetInsertable(e, &stubPermissionService{}, "GetInsertable")
	if err == nil {
		t.Fatal("a description echoing the key was accepted")
	}
	if !permissionBlames(err, "Description") {
		t.Errorf("the rejection should name Description, it named %v", permissionRejectedFields(err))
	}
	if !permissionRaised(err, "PermissionDescriptionEchoesKeyNotification") {
		t.Errorf("raised %v, want PermissionDescriptionEchoesKeyNotification", permissionAnswers(err))
	}
}

// The comparison is normalized, which is the whole point: it has to catch the
// paste however the caller dressed it up. Each of these reduces to the same
// letters and digits as `tenant:read`.
func TestPermissionDescriptionEchoIsDetectedThroughFormatting(t *testing.T) {
	for _, in := range []string{
		"permission:archive",
		"Permission:Archive",
		"PERMISSION ARCHIVE",
		"permission-archive",
		"  permission :: archive  ",
		"permission_archive.",
	} {
		e := validPermission()
		e.Permission = echoingKey
		e.Description = vos.Description(in)

		_, err := domain.GetInsertable(e, &stubPermissionService{}, "GetInsertable")
		if err == nil {
			t.Errorf("description %q was accepted — it is the key with different punctuation", in)
			continue
		}
		if !permissionRaised(err, "PermissionDescriptionEchoesKeyNotification") {
			t.Errorf("description %q raised %v, want PermissionDescriptionEchoesKeyNotification", in, permissionAnswers(err))
		}
	}
}

// It catches the lazy paste and NOTHING more: a real sentence that happens to
// mention the resource and the action is exactly what a good description does.
func TestPermissionDescriptionMayMentionTheKeyInProse(t *testing.T) {
	for _, in := range []string{
		"Read tenants: list the tenant registry and fetch a tenant by id.",
		"Lets the holder read tenant records, and nothing else.",
	} {
		e := validPermission()
		e.Permission = vos.PermissionKey{Resource: "tenant", Action: "read"}
		e.Description = vos.Description(in)

		if _, err := domain.GetInsertable(e, &stubPermissionService{}, "GetInsertable"); err != nil {
			t.Errorf("description %q was rejected: %v (fields: %v)", in, err, permissionRejectedFields(err))
		}
	}
}

// The rule is scoped IfInsertOrUpdate, and the update path is the one that
// matters most: description is the ONLY editable field, so an operator
// "improving" it into the key is the realistic way this happens.
func TestPermissionDescriptionEchoIsRefusedOnUpdateToo(t *testing.T) {
	e := persistedPermission()
	e.Permission = echoingKey

	_, err := domain.GetUpdatable(e, func(x *Permission) error {
		x.Description = vos.Description("Permission Archive")
		return nil
	}, &stubPermissionService{}, "GetUpdatable")
	if err == nil {
		t.Fatal("an update rewriting the description into the key was accepted")
	}
	if !permissionRaised(err, "PermissionDescriptionEchoesKeyNotification") {
		t.Errorf("raised %v, want PermissionDescriptionEchoesKeyNotification", permissionAnswers(err))
	}
}

// An empty description is the Description value object's complaint to make.
// The rule stands down rather than piling a second answer onto one field, so
// the caller reads one problem in one place.
func TestPermissionEmptyDescriptionIsAnsweredOnceByTheValueObject(t *testing.T) {
	e := validPermission()
	e.Description = vos.Description("")

	_, err := domain.GetInsertable(e, &stubPermissionService{}, "GetInsertable")
	if err == nil {
		t.Fatal("an empty description was accepted")
	}
	if permissionRaised(err, "PermissionDescriptionEchoesKeyNotification") {
		t.Errorf("the echo rule fired on an empty description: %v", permissionAnswers(err))
	}
	if !permissionRaised(err, "RequiredFieldNotification") {
		t.Errorf("raised %v, want the value object's RequiredFieldNotification", permissionAnswers(err))
	}
}

// ── the unique pre-check ─────────────────────────────────────────────────────

// takenPermissionService answers the one question the generated stub always
// answers "no" to. The generated suite cannot reach this branch — its stub is
// what lets every valid fixture through — so the 409 half of
// `unique.enforce: service-precheck+constraint` is proven here.
type takenPermissionService struct {
	domain.ServiceBase
	sawResource string
	sawAction   string
}

func (s *takenPermissionService) PermissionKeyTaken(resource string, action string, _ domain.ID) bool {
	s.sawResource, s.sawAction = resource, action
	return true
}

// The pre-check is what lets a duplicate be reported TOGETHER with the other
// validation problems instead of arriving alone as a 409 after everything else
// passed. The database's partial unique index is the backstop for the race.
func TestPermissionDuplicateKeyIsRefusedByThePreCheck(t *testing.T) {
	service := &takenPermissionService{}
	e := validPermission()

	_, err := domain.GetInsertable(e, service, "GetInsertable")
	if err == nil {
		t.Fatal("a permission whose pair is already taken was accepted")
	}
	if !permissionRaised(err, "PermissionAlreadyExistsNotification") {
		t.Errorf("raised %v, want PermissionAlreadyExistsNotification", permissionAnswers(err))
	}
	// The field names the COMPOSITE, and it names it `permission` — the same
	// word the read side answers with and the label renders. It was `key` until
	// the field was renamed, which named nothing any request or response carried.
	if !permissionBlames(err, "Permission") {
		t.Errorf("the rejection should name Permission, it named %v", permissionRejectedFields(err))
	}

	// And it hands the refused pair BACK. Saying a permission is taken without
	// saying which one is what `unique.echoValue: true` exists to end; the value
	// travels through PermissionKey.String(), which is why the rendering is
	// `tenant:read` and not a formatted Go struct.
	if got := permissionEchoed(err, "PermissionAlreadyExistsNotification"); got != "tenant:read" {
		t.Errorf("the conflict echoed %q, want the refused pair %q", got, "tenant:read")
	}

	// The question is asked about the PAIR, not about either half: a second
	// tenant:<something else> is perfectly legal, and a pre-check filtered by
	// the resource alone would refuse it.
	if service.sawResource != "tenant" || service.sawAction != "read" {
		t.Errorf("the service was asked about %q/%q, want both halves of the pair", service.sawResource, service.sawAction)
	}
}

// ── the pair is frozen, half by half ─────────────────────────────────────────

// The generated suite proves the rule fires when the WHOLE pair is replaced.
// The business rule the maintainer stated is stricter than that: neither half
// may move on its own.
//
// It holds because the rule compares the composite as a value — `old.Permission
// != e.Permission` is a struct comparison over two comparable fields — but "it follows
// from the implementation" is not the same as "it is pinned", and this is the
// invariant that quietly hands the old permission, free, to everyone who
// already holds it.
func TestPermissionNeitherHalfOfTheKeyMayChange(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Permission)
	}{
		{"the resource alone", func(x *Permission) { x.Permission.Resource = "billing" }},
		{"the action alone", func(x *Permission) { x.Permission.Action = "insert" }},
		{"both halves", func(x *Permission) { x.Permission = vos.PermissionKey{Resource: "billing", Action: "insert"} }},
		{"a case change on the resource", func(x *Permission) { x.Permission.Resource = "Tenant" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := persistedPermission()

			_, err := domain.GetUpdatable(e, func(x *Permission) error {
				tc.mutate(x)
				return nil
			}, &stubPermissionService{}, "GetUpdatable")
			if err == nil {
				t.Fatalf("changing %s was accepted — every holder of the old pair would keep it, meaning something else", tc.name)
			}
			if !permissionRaised(err, "PermissionKeyIsImmutableNotification") {
				t.Errorf("changing %s raised %v, want PermissionKeyIsImmutableNotification", tc.name, permissionAnswers(err))
			}
		})
	}
}

// The other half of the same rule: description IS editable. A frozen key that
// also froze the wording would make `update` a mode with nothing to do, and
// improving a description after operators read it is exactly why the mode is
// declared.
func TestPermissionDescriptionRemainsEditable(t *testing.T) {
	e := persistedPermission()

	if _, err := domain.GetUpdatable(e, func(x *Permission) error {
		x.Description = vos.Description("Read tenants, including the archived ones, through the registry listing.")
		return nil
	}, &stubPermissionService{}, "GetUpdatable"); err != nil {
		t.Fatalf("editing only the description was rejected: %v (fields: %v)", err, permissionRejectedFields(err))
	}
}
