// Hand-written, and not a hook: no generator declares this file.
//
// It exists because the token routes cannot live in any generated feature: the
// spec language gates operations by a closed set of lifecycle verbs, and "sign
// in" is not one of them. Mounting them by hand inside a generated file would be
// an edit the next regeneration refuses.
//
// THE ISSUER IS READ IN Mount, NOT IN THE CONSTRUCTOR, and that ordering is the
// one non-obvious thing here. Deps.Issuer is populated POST-WIRE — buildIssuer
// runs after Wire returns, because the boot guard "refresh tokens enabled
// requires a RefreshTokenStore" can only be checked once Wiring exists. So a
// feature that captured d.Issuer at construction time would capture nil, mount
// two routes over it, and fail on the first sign-in with a nil dereference on a
// service that booted perfectly green.

package main

import (
	appinfra "github.com/ClaudioSchirmer/authcore/internal/infra"
	appweb "github.com/ClaudioSchirmer/authcore/internal/web"
	"github.com/ClaudioSchirmer/omnicore/bootstrap"
	"github.com/ClaudioSchirmer/omnicore/infra/events"
	"github.com/gofiber/fiber/v3"
)

// AuthenticationFeature mounts the two user token routes and the machine one.
//
// It builds and OWNS all of its adapters, like every other feature here. The
// refresh store is one of them even though the framework also needs it: see
// RefreshTokenStore below for why that does not move ownership out of this file.
//
// THE CLIENT READER LIVES HERE AND NOT IN A FEATURE OF ITS OWN, which is the one
// placement decision worth stating. The origin-address middleware is registered on
// the `/auth` group object inside MountAuthentication, and Fiber binds a group
// middleware in registration order — so a client-token route mounted from a second
// feature would run that middleware or not depending on which feature Wire happened
// to list first. On the client route the origin address is an AUTHORIZATION input
// (Client.allowedCIDRs is compared against it), so "sometimes empty" is not a
// forensic annoyance, it is a client with an allow-list being refused. One owner of
// the group removes the ordering question entirely.
type AuthenticationFeature struct {
	reader   *appinfra.AuthenticationReader
	clients  *appinfra.ClientAuthenticationReader
	store    *appinfra.RefreshTokenStore
	attempts *appinfra.AuthenticationAttemptStore
}

// NewAuthenticationFeature builds both adapters over the service's engine.
//
// The reader assembles its permission-resolution statement HERE, at construction,
// from the TableSchema declarations — so a renamed column aborts this boot with
// the field named, rather than shipping a SELECT that silently matches nothing.
func NewAuthenticationFeature(d bootstrap.Deps) *AuthenticationFeature {
	return &AuthenticationFeature{
		reader:   appinfra.NewAuthenticationReader(d.DB),
		clients:  appinfra.NewClientAuthenticationReader(d.DB),
		store:    appinfra.NewRefreshTokenStore(d.DB, d.Logger),
		attempts: appinfra.NewAuthenticationAttemptStore(d.DB),
	}
}

// RefreshTokenStore exposes the store the framework's Issuer persists rotation
// state through.
//
// IT IS A GETTER RATHER THAN A CONSTRUCTOR CALL IN Wire FOR A REASON. The
// framework reads this through Wiring.RefreshTokenStore, which is a field of the
// struct Wire returns — a feature cannot write into Wiring, so the value has to
// be REACHABLE from there. Reachable is not the same as owned: building it in
// Wire would have put one feature's adapter in the composition root, where the
// next person to touch it has no reason to look. This way the feature stays the
// single owner and Wire only forwards the one thing the framework asks for.
//
// The instance matters, not just the type: the Issuer writes rows through it and
// the rotation route reads them back, so two instances would be two halves of one
// story.
func (f *AuthenticationFeature) RefreshTokenStore() *appinfra.RefreshTokenStore {
	return f.store
}

// Mount delegates to the web layer, like every other feature here.
//
// It contributes NO read model: both routes answer with a token pair, which is
// not a projection of anything.
func (f *AuthenticationFeature) Mount(app *fiber.App, d bootstrap.Deps) {
	// See the file header: this is the value that is nil at construction time and
	// filled by the time Mount runs.
	// The publisher is the framework's own, built here rather than held as a
	// field: it is stateless, it is the composition root's to choose, and passing
	// d.Logger is what puts the sign-in's records on the SAME stdout channel and
	// in the same vocabulary as every audit echo — so one log query reaches both.
	//
	// No adapter and no wrapper type: *events.SlogPublisher satisfies the
	// application's narrow port structurally, which is exactly why that port was
	// spelled with the framework's own signature.
	//
	// d.Issuer is handed in TWICE and that is not a slip: the user routes take it
	// through commands.TokenIssuer (IssueWithRefresh + RedeemRefreshToken) and the
	// client route through commands.AccessTokenIssuer (Issue alone). Two narrow
	// ports over one instance, each naming exactly what its consumer calls — which
	// is what lets the client handler's tests run without a refresh store and
	// documents that this route mints no refresh token.
	appweb.MountAuthentication(app, f.reader, f.clients, f.store, f.attempts,
		events.NewSlogPublisher(d.Logger), d.Issuer, d.Issuer, d)
}
