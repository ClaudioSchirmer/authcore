// The GraphQL half of the secret rotation, proven at the schema.
//
// Same posture as the user credential mount's test: a mount returns nothing and
// writes into a registry, so what is worth asking is whether the field reached
// the SDL with the shape a client will code against. The handler takes a nil
// store and a nil service on purpose — registering a field executes nothing.

package web

import (
	"strings"
	"testing"

	fwgraphql "github.com/ClaudioSchirmer/omnicore/web/graphql"
)

func mountedRotationSDL(t *testing.T) string {
	t.Helper()

	reg := fwgraphql.New(nil)
	MountClientSecretsGraphQL(reg, nil, nil)

	sdl, err := reg.SDL()
	if err != nil {
		t.Fatalf("the schema did not build: %v", err)
	}
	return sdl
}

// TestMountClientSecretsGraphQL_PublishesTheRotation is the point of the mount:
// until 2026-08-28 a GraphQL client could create a client and never obtain a
// usable secret for it, since the insert mints one and shows nothing.
func TestMountClientSecretsGraphQL_PublishesTheRotation(t *testing.T) {
	sdl := mountedRotationSDL(t)

	const want = "rotateClientSecret(id: ID!, input: RotateClientSecretInput!): RotateClientSecretPayload!"
	if !strings.Contains(sdl, want) {
		t.Errorf("the schema does not publish %q — the rotation is not reachable from GraphQL\n%s", want, sdl)
	}
	if !strings.Contains(sdl, "secret: String") {
		t.Errorf("the payload does not carry the plaintext — this response is the only place it can ever be read\n%s", sdl)
	}
}

// TestMountClientSecretsGraphQL_KeepsTheGraceWindowNullable guards a distinction
// a non-null Int would erase: ZERO means "retire the old secret now" — the
// leaked case — and ABSENT means "use the default day". Collapsed, an omitted
// argument would revoke a live credential immediately.
func TestMountClientSecretsGraphQL_KeepsTheGraceWindowNullable(t *testing.T) {
	sdl := mountedRotationSDL(t)

	if !strings.Contains(sdl, "gracePeriodSeconds: Int\n") {
		t.Errorf("gracePeriodSeconds is not a nullable Int — zero and absent must stay tellable apart\n%s", sdl)
	}
}
