// The GraphQL half of the two credential operations, proven at the schema.
//
// A mount is not testable by calling it — it returns nothing and writes into a
// registry — but the registry publishes an SDL, and the SDL is the contract a
// client reads. So this asks the only question worth asking of wiring: did the
// two fields land on the schema, with the arguments and the payload the REST
// twins promise?
//
// The handlers take a nil store and a nil service on purpose: registering a
// field builds no resolver and executes nothing. What is under test here is the
// shape, and a fake store would only add a dependency the assertion never reads.

package web

import (
	"strings"
	"testing"

	fwgraphql "github.com/ClaudioSchirmer/omnicore/web/graphql"
)

func mountedCredentialSDL(t *testing.T) string {
	t.Helper()

	reg := fwgraphql.New(nil)
	MountUserCredentialsGraphQL(reg, nil, nil)

	sdl, err := reg.SDL()
	if err != nil {
		t.Fatalf("the schema did not build: %v", err)
	}
	return sdl
}

// TestMountUserCredentialsGraphQL_PublishesBothFields is the whole point of the
// mount: until 2026-08-28 these two operations answered on REST alone, and a
// client on the GraphQL surface had no way to set a password at all.
func TestMountUserCredentialsGraphQL_PublishesBothFields(t *testing.T) {
	sdl := mountedCredentialSDL(t)

	for _, want := range []string{
		"changeUserPassword(id: ID!, input: ChangeUserPasswordInput!): ChangeUserPasswordPayload!",
		"resetUserPassword(id: ID!, input: ResetUserPasswordInput!): ResetUserPasswordPayload!",
	} {
		if !strings.Contains(sdl, want) {
			t.Errorf("the schema does not publish %q — the field is not reachable from GraphQL\n%s", want, sdl)
		}
	}
}

// TestMountUserCredentialsGraphQL_KeepsTheTwoBodiesApart guards the distinction
// the two REST bodies exist to keep: the change proves the credential it
// replaces, the reset does not know it. One input carrying an optional
// currentPassword would document neither.
func TestMountUserCredentialsGraphQL_KeepsTheTwoBodiesApart(t *testing.T) {
	sdl := mountedCredentialSDL(t)

	change := inputBlock(t, sdl, "ChangeUserPasswordInput")
	if !strings.Contains(change, "currentPassword") {
		t.Errorf("the change input carries no currentPassword — the operation would prove nothing:\n%s", change)
	}

	reset := inputBlock(t, sdl, "ResetUserPasswordInput")
	if strings.Contains(reset, "currentPassword") {
		t.Errorf("the reset input grew a currentPassword — that is the change endpoint wearing the wrong name:\n%s", reset)
	}
}

// inputBlock returns the SDL body of one input type, so an assertion about that
// type cannot accidentally match a field of the other one.
func inputBlock(t *testing.T, sdl, name string) string {
	t.Helper()

	start := strings.Index(sdl, "input "+name+" {")
	if start < 0 {
		t.Fatalf("the schema declares no input %s:\n%s", name, sdl)
	}
	rest := sdl[start:]
	end := strings.Index(rest, "}")
	if end < 0 {
		t.Fatalf("input %s is not closed:\n%s", name, rest)
	}
	return rest[:end]
}
