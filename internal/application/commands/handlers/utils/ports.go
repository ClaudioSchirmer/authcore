// Hand-written, and not a hook: no generator declares this file.
//
// EVERY PORT THE TOKEN AND CREDENTIAL HANDLERS TAKE, in one place.
//
// They are declared HERE rather than as *infra.Something so the application keeps
// depending on interfaces IT owns, and they sit beside each other rather than
// inside the handler files because a handler file holds a handler — the layout
// standard is read by whoever opens the directory, and a reader scanning
// handlers/ should meet handlers.
//
// THEY ARE SEPARATE INTERFACES AND NOT ONE, deliberately. Different adapters
// implement them — the sign-in reader, the refresh store, the attempt counters,
// the aggregate repositories — and folding them together would give whichever grew
// first methods it has no business owning.
//
// NOTHING A CREDENTIAL COULD RIDE IN crosses the attempt seam: there is no
// parameter a password could arrive in and no column it could be written to.

package utils

import (
	"context"

	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/authcore/internal/infra"
	"github.com/ClaudioSchirmer/authcore/internal/infra/schemas"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/application/persistence"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/web/authcore"
)

// AuthenticationStore is what the sign-in needs from infra, and no more.
//
// Declared here rather than taken as *infra.AuthenticationReader so the
// application keeps depending on an interface IT owns — the same reason
// UserCredentialStore exists one file over.
type AuthenticationStore interface {
	// LoadAccountByEmail is the account lookup, and the FIRST of the two steps.
	// One indexed statement, active rows only, and a live tenant.
	//
	// (nil, nil) means NO SUCH ACCOUNT; (nil, err) means the lookup could not be
	// performed. Both are refused identically, but they are LOGGED differently —
	// see the sign-in — so a reviewer can tell stuffing against addresses that do
	// not exist from an attack that landed during an outage.
	//
	// IT RETURNS A ROW AND NOT AN AGGREGATE, deliberately. A sign-in has no
	// invariant to protect and no lifecycle to drive; loading the User aggregate
	// cost four sequential statements to hydrate three collections this endpoint
	// does not want. See the reader.
	LoadAccountByEmail(ctx context.Context, email string) (*schemas.SignInAccount, error)
	// LoadAccountByID reloads the account on a refresh, so a permission revoked
	// between logins reaches the mesh at the next rotation rather than at the
	// next full sign-in.
	LoadAccountByID(ctx context.Context, id domain.ID) (*schemas.SignInAccount, error)
	// ResolveSignIn reads everything a token says about this account, in one
	// concurrent burst of statements. It is the SECOND step on purpose: it hides
	// behind the account read, so a credential-stuffing attempt against an
	// address nobody holds costs one statement rather than five.
	//
	// THE GROUPS AND ROLES COME FROM HERE AND NOT FROM AN AGGREGATE. A child join
	// reaches a group's or a role's row whatever its archived state, so a token
	// built from a loaded aggregate would name things the operator retired. The
	// reader states every gate; an aggregate load cannot.
	ResolveSignIn(ctx context.Context, account *schemas.SignInAccount) (infra.SignInBundle, error)
	// PasswordMatches verifies a plaintext against a stored hash.
	PasswordMatches(plaintext, encoded string) bool
	// BurnPasswordVerification spends one verification and discards it, so a
	// Refusal that never reached a real hash costs what a real rejection costs.
	BurnPasswordVerification()
}

// RefreshTokenLookup is the one thing the rotation needs from the refresh store.
//
// It is a SEPARATE port from AuthenticationStore because a different adapter
// implements it — the token store, not the user reader — and folding both into
// one interface would force whichever adapter grew first to grow methods it has
// no business owning.
type RefreshTokenLookup interface {
	// SubjectForRefreshToken answers whose session a value belongs to, or "" when
	// it belongs to none. It decides nothing about validity: revoked, used and
	// expired stay the Issuer's call at redemption.
	SubjectForRefreshToken(ctx context.Context, value string) (string, error)
}

// TokenIssuer is the slice of the framework's Issuer these handlers use.
//
// Narrowing it to two methods is not ceremony: it is what lets the handler tests
// drive the Refusal branches without a signing key, and it documents that nothing
// here touches key material, TTL setters, or the JWKS document.
type TokenIssuer interface {
	IssueWithRefresh(ctx context.Context, req authcore.TokenRequest) (authcore.IssuedToken, authcore.RefreshToken, error)
	RedeemRefreshToken(ctx context.Context, value string, claims map[string]any) (authcore.IssuedToken, authcore.RefreshToken, error)
}

// ClientAuthenticationStore is what the machine sign-in needs from infra, and no
// more.
//
// Declared here rather than taken as *infra.ClientAuthenticationReader so the
// application keeps depending on an interface IT owns — the same reason
// AuthenticationStore exists one file over.
type ClientAuthenticationStore interface {
	// LoadClientByID is the account lookup, and the FIRST of the two steps. One
	// indexed statement, active rows only, and a live tenant.
	//
	// (nil, nil) means NO SUCH CLIENT; (nil, err) means the lookup could not be
	// performed. Both are refused identically, but they are LOGGED differently, so
	// a reviewer can tell attempts against ids that do not exist from an attack
	// that landed during an outage.
	LoadClientByID(ctx context.Context, id domain.ID) (*schemas.SignInClient, error)

	// ResolveClientSignIn reads everything a token says about this client, in one
	// concurrent burst. It is the SECOND step on purpose: it hides behind the
	// account read, so an attempt against an id nobody holds costs one statement
	// rather than five.
	//
	// THE ROLES COME FROM HERE AND NOT FROM AN AGGREGATE. A child join reaches a
	// role's row whatever its archived state, so a token built from a loaded
	// aggregate would name things the operator retired.
	ResolveClientSignIn(ctx context.Context, client *schemas.SignInClient) (infra.ClientSignInBundle, error)

	// SecretMatches verifies a presented secret against the client's live hash
	// and, on a miss, against the retiring one while its grace window is open.
	SecretMatches(presented string, client *schemas.SignInClient) bool

	// BurnSecretVerification spends one verification and discards it, so a Refusal
	// that never reached a real hash costs what a real rejection costs.
	BurnSecretVerification()
}

// AccessTokenIssuer is the ONE method this route uses of the framework's Issuer.
//
// It is a port of its own rather than a third method on TokenIssuer, because the
// user route's two handlers do not mint without a refresh and would gain a method
// they never call — and every test double of theirs would have to implement it.
// Narrowing to what the consumer uses is the rule; *authcore.Issuer satisfies both
// structurally, so the composition root hands the same value to each.
type AccessTokenIssuer interface {
	Issue(ctx context.Context, req authcore.TokenRequest) (authcore.IssuedToken, error)
}

// ClientSecretStore is what the handler needs from infra, and no more.
//
// Declared HERE rather than taken as *infra.ClientRepository so the application
// layer keeps depending on an interface it owns: the handler reads one client
// and writes one, and naming exactly those two is what keeps a later change to
// the repository from reaching in. Same shape as UserCredentialStore, one file
// over, and for the same reason.
type ClientSecretStore interface {
	// ScopedReader rather than the repository's own FindByID: the bound reader
	// carries the request context, so the read runs under the same deadline,
	// cancellation and trace as the write that follows it.
	ScopedReader(ctx *configuration.AppContext) domain.Reader[*appdomain.Client]
	Scope(ctx *configuration.AppContext, opts ...persistence.WriteOption[*appdomain.Client]) domain.Writer
}

// UserCredentialStore is what the handlers need from infra, and no more.
//
// It is declared HERE rather than taken as *infra.UserRepository so the
// application layer keeps depending on an interface it owns: the handlers need
// to read one user and write one, and naming exactly those two is what keeps a
// later change to the repository from reaching in.
type UserCredentialStore interface {
	// ScopedReader rather than the repository's own FindByID: the bound reader
	// carries the request context, so the read runs under the same deadline,
	// cancellation and trace as the write that follows it.
	ScopedReader(ctx *configuration.AppContext) domain.Reader[*appdomain.User]
	Scope(ctx *configuration.AppContext, opts ...persistence.WriteOption[*appdomain.User]) domain.Writer
}
