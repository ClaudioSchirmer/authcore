// Hand-written, and not a hook: no generator declares this file.
//
// The two operations that turn a credential into a token, and a token into a
// fresher one. Neither is a shape the spec language has — `authz.permissions`
// takes a closed set of lifecycle verbs and "sign in" is not one of them — so the
// commands, the handlers and the routes are all by hand.
//
// WHAT THE FRAMEWORK OWNS AND WHAT WE OWN. The framework's authcore.Issuer owns
// everything security-critical about the TOKEN: asymmetric signing, the reserved
// claims, key rotation, and the refresh algorithm (opaque values, single use,
// rotation on every redemption, family revocation on reuse). It authenticates
// NOBODY — its own documentation says so — and mints for a subject the caller has
// already decided is authentic. Deciding that is this file's entire job.
//
// THE REFUSAL IS ONE ANSWER TO FIVE QUESTIONS, and that is the design rather than
// laziness. See InvalidCredentialsNotification for the full argument; the short
// version is that any split builds an enumeration oracle, and the timing is part
// of the message — a refusal that comes back in a millisecond instead of a
// hundred has answered the question the words refused to answer.

package commands

import (
	"context"
	"errors"
	"maps"
	"strconv"
	"strings"
	"time"

	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/authcore/internal/domain/aggregatevos"
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/application/exception"
	"github.com/ClaudioSchirmer/omnicore/application/pipeline"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/web/authcore"
)

// PermissionChangeOwnPassword is the one permission a must-change-password
// session keeps.
//
// It has to be the same string the change route is gated with — that route spells
// it as a literal in internal/web/user_credential_routes_manual.go, which the
// approved impact map for this work does not touch. So the duplication stands for
// now and is deliberate rather than overlooked: pointing that route at this
// constant is a one-line follow-up, and until it happens the two spellings must
// be changed together or a must-change-password session silently stops being able
// to change its password.
const PermissionChangeOwnPassword = "user:change-password"

// tokenTypeBearer is the scheme both operations answer with. It is what the
// caller must put in front of the token in an Authorization header, and the only
// scheme the framework's middleware parses.
const tokenTypeBearer = "Bearer"

// The claim names this service mints.
//
// Two of them are NOT ours to choose. `permissions` and `tenant_id` are what the
// framework's Identity reads back (application/configuration/authz_config.go), so
// every service in the mesh depends on those exact spellings; renaming either
// silently disables authorization everywhere rather than failing loudly. The rest
// are ours, and they are snake_case to match the two that are not.
const (
	claimPermissions        = "permissions"
	claimTenantID           = "tenant_id"
	claimTenantWorkspace    = "tenant_workspace"
	claimEmail              = "email"
	claimName               = "name"
	claimGroups             = "groups"
	claimRoles              = "roles"
	claimMustChangePassword = "must_change_password"

	// WHICH KIND OF SUBJECT THE TOKEN SPEAKS FOR — `user` here, `client` when
	// POST /auth/client/token mints one. The spelling is not ours to choose
	// freely: specs/omnicore-gen/client.omnicore.yaml declares it, and Client's
	// row rules already read it back under exactly this name.
	//
	// MINTED EVEN THOUGH NOTHING REQUIRES IT YET, and that is the point. Client's
	// rules stand down on anything that is not `client`, so emitting `user`
	// changes no decision today — what it removes is the inference. Until now a
	// user token was recognised by the ABSENCE of this claim, which cannot
	// distinguish a user from an issuer that forgot, or from a token minted
	// before the claim existed.
	//
	// It also has to be minted BEFORE the client route arrives rather than with
	// it: tokens outlive a deploy by their TTL, so a rule written positively
	// (`== "user"`) on the day the client route lands would misjudge every token
	// still in flight. Minting it now means that by then every live token carries
	// it. Until a full TTL has passed, rules stay written NEGATIVELY — the two
	// that exist compare against `client`, and they should not be "improved".
	claimIdentityKind = "identity_kind"
)

// AuthenticationStore is what the sign-in needs from infra, and no more.
//
// Declared here rather than taken as *infra.AuthenticationReader so the
// application keeps depending on an interface IT owns — the same reason
// UserCredentialStore exists one file over.
type AuthenticationStore interface {
	// FindUserByEmail loads the account behind an address, active rows only.
	//
	// (nil, nil) means NO SUCH ACCOUNT; (nil, err) means the lookup could not be
	// performed. Both are refused identically, but they are LOGGED differently —
	// see the sign-in — so a reviewer can tell stuffing against addresses that do
	// not exist from an attack that landed during an outage.
	FindUserByEmail(ctx *configuration.AppContext, email string) (*appdomain.User, error)
	// FindUserByID reloads the account on a refresh, so a permission revoked
	// between logins reaches the mesh at the next rotation rather than at the
	// next full sign-in.
	FindUserByID(ctx *configuration.AppContext, id domain.ID) (*appdomain.User, error)
	// ResolveGrants returns the roles held by ANY path and the permissions they
	// confer, in one round trip. Two slices and no result struct: see the reader
	// for why a named type here would have been one invented to work around a
	// layer boundary rather than to model anything.
	ResolveGrants(ctx context.Context, userID domain.ID) (roleKeys []string, permissions []vos.PermissionKey, err error)
	// ClaimDefinitionsOfTenant returns the ACTIVE claim definitions of this
	// tenant that a user may hold a value for — level 2 of the two-level chain,
	// and the vocabulary level 1 is resolved against.
	//
	// It returns the aggregate and NOT a result type of its own, for the reason
	// ResolveGrants states one method up: a struct with a name, a type and a
	// default would have no identity, no rule and no validation, and would
	// exist only to give this port something to name. *appdomain.Claim already
	// is that shape, and FindUserByEmail already hands the application an
	// aggregate across this same seam.
	ClaimDefinitionsOfTenant(ctx *configuration.AppContext, tenantID domain.ID) ([]*appdomain.Claim, error)
	// PasswordMatches verifies a plaintext against a stored hash.
	PasswordMatches(plaintext, encoded string) bool
	// BurnPasswordVerification spends one verification and discards it, so a
	// refusal that never reached a real hash costs what a real rejection costs.
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

// ContextKeyClientIP is where the /auth middleware leaves the request's origin
// address for the handler to read.
//
// It rides the AppContext's generic bag because the framework's AppContext
// exposes no IP of its own, and a pipeline.Handler receives that context rather
// than the Fiber one. The alternative — MountRaw, to reach c.IP() directly —
// would have cost the canonical envelope and the seven catalogs for one string.
const ContextKeyClientIP = "authcore.request.ip"

// TokenIssuer is the slice of the framework's Issuer these handlers use.
//
// Narrowing it to two methods is not ceremony: it is what lets the handler tests
// drive the refusal branches without a signing key, and it documents that nothing
// here touches key material, TTL setters, or the JWKS document.
type TokenIssuer interface {
	IssueWithRefresh(ctx context.Context, req authcore.TokenRequest) (authcore.IssuedToken, authcore.RefreshToken, error)
	RedeemRefreshToken(ctx context.Context, value string, claims map[string]any) (authcore.IssuedToken, authcore.RefreshToken, error)
}

// ── the results ─────────────────────────────────────────────────────────────

// TokenResult is what both operations answer with.
//
// It carries MORE than the token deliberately. The access token's claims are kept
// to identity and authorization — they ride in a header on every request to every
// service, and a group description is exactly the field that grows unnoticed
// until a proxy truncates the header — so the richer profile travels here, in a
// body read once at sign-in and never re-sent.
type TokenResult struct {
	AccessToken      string
	TokenType        string
	ExpiresAt        int64
	RefreshToken     string
	RefreshExpiresAt int64
	User             AuthenticatedUserResult
}

// AuthenticatedUserResult is the profile a client renders after signing in.
type AuthenticatedUserResult struct {
	ID                 string
	Name               string
	Email              string
	Status             string
	MustChangePassword bool
	TenantID           string
	TenantWorkspace    string
	Groups             []NamedGrantResult
	Roles              []NamedGrantResult
	Permissions        []string
	// The tenant-defined claims this token carries, resolved down the two-level
	// chain and typed per each definition's declared value type.
	//
	// IT MIRRORS THE TOKEN EXACTLY, restriction included — the same reason
	// Permissions above is filled from effectivePermissions rather than from the
	// raw bundle. Both come from ONE resolution per request; neither re-derives
	// the other's answer.
	Claims map[string]any
}

// NamedGrantResult is one group or role, with the display name the token
// deliberately leaves out.
type NamedGrantResult struct {
	Key  string
	Name string
}

// ── the sign-in ─────────────────────────────────────────────────────────────

// IssueTokenCommand is what the sign-in route binds.
type IssueTokenCommand struct {
	pipeline.CommandWithBodyBase

	Email    string `json:"email"`
	Password string `json:"password"`
}

// IssueTokenHandler turns an e-mail and a password into a token pair.
type IssueTokenHandler struct {
	Store    AuthenticationStore
	Attempts AttemptRecorder
	Issuer   TokenIssuer

	// Events carries the per-attempt record that the rollup table stopped
	// keeping. A nil publisher disables the announcements and changes nothing
	// else — the same semantic the framework gives its own event port, and what
	// lets a test drive the refusal branches without one.
	Events AuthenticationEventPublisher
}

// journal is this route's view of the shared path to the auxiliary tables and
// the log stream, bound to the subject kind it authenticates.
//
// Built per call rather than held as a field: it is two interface copies and a
// string, and building it here keeps the handler's wiring exactly what the web
// layer already passes — the composition root never learns a new name.
func (h *IssueTokenHandler) journal() authenticationJournal {
	return newAuthenticationJournal(h.Attempts, h.Events, identityKindUser)
}

// Handle authenticates and mints.
//
// THE ORDER OF THE CHECKS IS THE SECURITY PROPERTY, not a style choice:
//
//  1. no row for this address → burn a verification, then refuse. Without the
//     burn the response time says "nobody here has that address".
//  2. a row exists → verify the password FIRST, before looking at status. Every
//     found-row path then costs one real Argon2id verification, so a suspended
//     account and a wrong password are indistinguishable from outside.
//  3. only then, the account and its tenant have to be usable.
//
// Every refusal is the same notification, the same field name and the same 401.
func (h *IssueTokenHandler) Handle(ctx *configuration.AppContext, cmd *IssueTokenCommand) (TokenResult, error) {
	// Lowercased and trimmed before ANYTHING else, and that ordering matters
	// twice: the domain stores addresses lowercase (vos.Email refuses anything
	// else), and the attempt log counts by this exact string — two spellings of
	// one address counting toward two windows would mean neither ever locks.
	email := strings.ToLower(strings.TrimSpace(cmd.Email))
	ip := clientIPOf(ctx)

	// ── the lockout, BEFORE the credential is even looked at ──
	//
	// Asked first because the point of a lockout is to stop paying for guesses:
	// past the threshold this costs one indexed read instead of a lookup plus a
	// ~100 ms verification. It answers identically whether or not the identity
	// names an account, which is what keeps the 429 from being an oracle.
	journal := h.journal()

	until, locked, knownToExist, err := journal.lockedUntil(ctx, email)
	if err != nil {
		// NOT a refusal. A lockout probe that cannot run has not established
		// anything, and answering 401 would tell a caller with a correct password
		// that it was wrong. It escapes as an exception → 500.
		return TokenResult{}, err
	}
	if locked {
		// Counted, so a reviewer sees somebody kept trying through the lock — and
		// counted on a lifetime column that moves neither the live count nor the
		// window anchor, so the trying cannot extend the lock.
		//
		// knownToExist comes from the failures that CAUSED this lock — the probe
		// read that row anyway. Nothing here looked the account up, and nothing
		// should: past the threshold the point is to stop paying for guesses. It
		// rides the announcement because "which locked identities are real
		// accounts under attack" is the question that separates a targeted attack
		// from credential-stuffing noise.
		if rerr := journal.refusedWhileLocked(ctx, email, ip, until, knownToExist); rerr != nil {
			return TokenResult{}, rerr
		}
		return TokenResult{}, refuseLocked(until)
	}

	user, err := h.Store.FindUserByEmail(ctx, email)
	if err != nil || user == nil {
		// THE ANSWER IS THE SAME, THE RECORD IS NOT — and holding those two apart
		// is what lets this branch be both safe and useful.
		//
		// The caller is refused identically whether the address is unknown or the
		// store failed: distinguishing them would mean 500 for one and 401 for the
		// other, and a caller could then learn an address exists by finding an
		// input that changes the status code.
		//
		// The LOG, which no attacker reads, keeps the difference. A store that
		// answered (nil, nil) established the address is not here — false. One
		// that answered an error established nothing — nil. Writing false in that
		// second case would put a claim in the table that nobody verified, and
		// this table exists to be trusted a year from now.
		var existed *bool
		if err == nil {
			absent := false
			existed = &absent
		}
		h.Store.BurnPasswordVerification()
		// The same split the counter makes, made once more on the stream: an
		// address that is provably absent and a lookup that never answered are one
		// refusal to the caller and two different lines to whoever reads this
		// later.
		reason := "sign-in failed: no account for this identity"
		if err != nil {
			reason = "sign-in failed: identity lookup could not be performed"
		}
		if rerr := journal.failed(ctx, email, ip, reason, existed); rerr != nil {
			return TokenResult{}, rerr
		}
		return TokenResult{}, refuseCredentials()
	}

	// From here the identity provably exists, and every remaining branch says so.
	existed := true

	if !h.Store.PasswordMatches(cmd.Password, user.PasswordHash) {
		if rerr := journal.failed(ctx, email, ip, "sign-in failed: credential rejected", &existed); rerr != nil {
			return TokenResult{}, rerr
		}
		return TokenResult{}, refuseCredentials()
	}

	if !accountIsUsable(user) {
		// A suspended account or a withdrawn tenant is a FAILURE in the log, not a
		// success: nobody got in. It counts toward the lockout like any other,
		// which is correct — repeatedly presenting a valid credential for a
		// disabled account is exactly the pattern worth rate-limiting.
		// The one branch where the credential was RIGHT and the answer is still a
		// refusal. Indistinguishable to the caller by design; on the stream it is
		// the line that explains a support ticket in one read.
		if rerr := journal.failed(ctx, email, ip,
			"sign-in failed: credential valid but account or tenant not usable", &existed); rerr != nil {
			return TokenResult{}, rerr
		}
		return TokenResult{}, refuseCredentials()
	}

	roleKeys, permissions, err := h.Store.ResolveGrants(ctx, idOf(user))
	if err != nil {
		// NOT a credential refusal, and NOT an attempt worth counting: the caller
		// proved who they are and this service failed them. Recording a failure
		// here would let a database problem lock out the very users it is already
		// failing. It escapes as an exception → 500.
		return TokenResult{}, err
	}

	// LEVEL 2 of the claim chain. Level 1 came along with the aggregate — the
	// repository's read join fills every entry — so this is the one extra read
	// the custom claims cost, and it is an indexed one.
	//
	// It fails the way ResolveGrants above fails, and for the same two reasons:
	// the caller proved who they are and this service failed them, so it is
	// neither a credential refusal nor an attempt worth counting — counting it
	// would let a database problem lock out the very users it is already
	// failing. Minting a token silently missing claims a consumer branches on
	// is the alternative, and a wrong answer is worse than no answer.
	catalog, err := h.Store.ClaimDefinitionsOfTenant(ctx, user.TenantID)
	if err != nil {
		return TokenResult{}, err
	}
	// ONE resolution, two readers. The token below and the profile at the
	// bottom of this function both receive this exact map.
	customClaims := resolveCustomClaims(user, catalog)

	access, refresh, err := h.Issuer.IssueWithRefresh(ctx, authcore.TokenRequest{
		Subject: idOf(user).Value(),
		Claims:  buildClaims(user, roleKeys, permissions, customClaims),
	})
	if err != nil {
		return TokenResult{}, err
	}

	// Recorded LAST, once the sign-in has actually succeeded. It is what anchors
	// the window — every failure before this moment stops counting — so writing it
	// any earlier would clear a counter for a sign-in that had not happened yet.
	if rerr := journal.succeeded(ctx, email, ip); rerr != nil {
		return TokenResult{}, rerr
	}

	return TokenResult{
		AccessToken:      access.Token,
		TokenType:        tokenTypeBearer,
		ExpiresAt:        access.ExpiresAt.Unix(),
		RefreshToken:     refresh.Value,
		RefreshExpiresAt: refresh.ExpiresAt.Unix(),
		User:             buildProfile(user, roleKeys, permissions, customClaims),
	}, nil
}

// ── the refresh ─────────────────────────────────────────────────────────────

// RefreshTokenCommand is what the rotation route binds.
type RefreshTokenCommand struct {
	pipeline.CommandWithBodyBase

	RefreshToken string `json:"refreshToken"`
}

// RefreshTokenHandler rotates a refresh token into a new pair.
type RefreshTokenHandler struct {
	Store  AuthenticationStore
	Lookup RefreshTokenLookup
	Issuer TokenIssuer
}

// Handle redeems and re-mints.
//
// THE CLAIMS ARE REBUILT FROM THE DATABASE, never replayed from the old token.
// RedeemRefreshToken takes them fresh from the caller at redemption time
// precisely so a permission revoked between logins reaches the mesh within
// minutes; handing back what the previous token carried would defeat the one
// mechanism the framework offers for that. It also means a must-change-password
// session cannot launder itself into a full one by refreshing — buildClaims
// applies the same restriction on this path as on the sign-in.
//
// THE SUBJECT IS LOOKED UP FIRST, and that ordering is forced by the framework
// rather than chosen: RedeemRefreshToken is handed the claim map by value and
// only then looks up the record, so this handler has to know whose claims to
// build before the framework would tell it. The lookup decides nothing — revoked,
// used and expired are all still the Issuer's call at the redemption below, which
// is what owns reuse detection and family revocation.
func (h *RefreshTokenHandler) Handle(ctx *configuration.AppContext, cmd *RefreshTokenCommand) (TokenResult, error) {
	value := strings.TrimSpace(cmd.RefreshToken)
	if value == "" {
		// Answered without touching the store: an empty string redeems nothing,
		// and there is no timing signal to equalise here — this refusal does not
		// depend on whether any account exists.
		return TokenResult{}, refuseCredentials()
	}

	subject, err := h.Lookup.SubjectForRefreshToken(ctx, value)
	if err != nil {
		// A store failure is NOT a credential refusal: answering 401 would tell a
		// caller holding a perfectly good token that it was rejected, and would
		// bury an outage inside a login problem. It escapes as an exception → 500.
		return TokenResult{}, err
	}
	if subject == "" {
		// No record. Resolving grants before this point would have handed anyone
		// posting random strings a free database walk.
		return TokenResult{}, refuseCredentials()
	}

	user, err := h.Store.FindUserByID(ctx, domain.NewID(subject))
	if err != nil || user == nil || !accountIsUsable(user) {
		// The record exists but its subject no longer resolves to a usable
		// account — archived, suspended, or its tenant withdrawn since the last
		// rotation. The session is over, and it ends with the same refusal as
		// every other.
		return TokenResult{}, refuseCredentials()
	}

	roleKeys, permissions, err := h.Store.ResolveGrants(ctx, idOf(user))
	if err != nil {
		return TokenResult{}, err
	}

	// RE-READ ON EVERY ROTATION, never replayed from the token being redeemed —
	// the same discipline the grants above already follow, and the reason the
	// framework takes claims fresh at redemption time. A claim value corrected
	// while a session is live therefore reaches the mesh at the next rotation
	// rather than at the next full sign-in, with no invalidation step anywhere.
	catalog, err := h.Store.ClaimDefinitionsOfTenant(ctx, user.TenantID)
	if err != nil {
		return TokenResult{}, err
	}
	customClaims := resolveCustomClaims(user, catalog)

	access, refresh, err := h.Issuer.RedeemRefreshToken(ctx, value, buildClaims(user, roleKeys, permissions, customClaims))
	switch {
	case err == nil:
	case errors.Is(err, authcore.ErrRefreshTokenNotFound),
		errors.Is(err, authcore.ErrRefreshTokenExpired),
		errors.Is(err, authcore.ErrRefreshTokenReused):
		// ALL THREE ANSWER THE SAME 401. Reuse in particular must not be
		// distinguishable: telling the holder of a stolen token that it had
		// already been redeemed confirms both that the token was real and that its
		// owner is active. The family has already been revoked by the Issuer
		// before this returns, and the WARN line the store logs is where an
		// operator sees it.
		return TokenResult{}, refuseCredentials()
	default:
		return TokenResult{}, err
	}

	return TokenResult{
		AccessToken:      access.Token,
		TokenType:        tokenTypeBearer,
		ExpiresAt:        access.ExpiresAt.Unix(),
		RefreshToken:     refresh.Value,
		RefreshExpiresAt: refresh.ExpiresAt.Unix(),
		User:             buildProfile(user, roleKeys, permissions, customClaims),
	}, nil
}

// ── the shared decisions ────────────────────────────────────────────────────

// refuseCredentials is THE refusal. Every branch that declines a sign-in returns
// exactly this — one notification, one neutral field name, one 401.
func refuseCredentials() error {
	return refusal(InvalidCredentialsNotification{})
}

// refuseLocked is the ONE refusal that is not the generic 401.
//
// It carries the remaining window so the caller learns that WAITING is the fix —
// the whole reason the maintainer asked for a distinct answer here. It discloses
// nothing, because the lockout is keyed by the attempted identity: an address
// that names no account reaches this same message on its sixth attempt.
//
// The window is rounded UP to the next whole minute. Rounding down would tell
// somebody to come back at a moment that is still inside the lock, and "0 minutes"
// is not an instruction.
func refuseLocked(until time.Time) error {
	remaining := time.Until(until)
	minutes := int(remaining / time.Minute)
	if remaining%time.Minute > 0 {
		minutes++
	}
	if minutes < 1 {
		minutes = 1
	}
	return refusal(AccountTemporarilyLockedNotification{Minutes: strconv.Itoa(minutes)})
}

// refusal wraps one notification in the carrier the pipeline understands.
//
// An APPLICATION error, not a domain one: nothing in the domain decided this, and
// the two are distinguishable at a glance in a log. The pipeline treats either
// through domain.NotificationCarrier, so the choice costs nothing and records
// which layer refused.
//
// The context name and the field name are fixed HERE rather than at each call
// site, so no future branch can refuse under a field name that says which half of
// the credential was wrong.
func refusal(n domain.Notification) error {
	return exception.NewApplicationErrorWith("Authentication", domain.NotificationMessage{
		FieldName:    "credentials",
		Notification: n,
	})
}

// clientIPOf reads the origin address the /auth middleware left on the context.
//
// Absent is "" rather than an error: an attempt with no recorded origin is still
// an attempt worth counting, and refusing a sign-in because a forensic field was
// missing would trade the operation for its own log.
func clientIPOf(ctx *configuration.AppContext) string {
	if ctx == nil {
		return ""
	}
	if raw, ok := ctx.Get(ContextKeyClientIP); ok {
		if ip, ok := raw.(string); ok {
			return ip
		}
	}
	return ""
}

// accountIsUsable reports whether this account may hold a session at all.
//
// TWO conditions, and the second is the one that is easy to forget. The account
// itself must be active — a suspended user is refused — and its owning TENANT
// must not be commercially suspended, because a customer who stopped paying
// should not be issuing credentials. Trial and active tenants both pass: a trial
// is a live customer being onboarded, and signing in is the first thing they need.
//
// The archive stamp needs no check here: the loader's default scope already
// refuses archived rows, so an archived account never reaches this function.
func accountIsUsable(user *appdomain.User) bool {
	if user.Status.Value() != vos.UserStatusActive.Value() {
		return false
	}
	// The joined column, filled on every load by the repository's InnerJoin into
	// Tenant. An empty value means the join found nothing, which for an INNER join
	// over a NOT NULL key cannot happen — but reading it as "unusable" is the
	// fail-closed direction if it ever does.
	if user.TenantStatus == "" || user.TenantStatus == vos.TenantStatusSuspended.Value() {
		return false
	}
	return true
}

// buildClaims assembles the access token's claim set.
//
// THE MUST-CHANGE-PASSWORD RESTRICTION LIVES HERE, and it is the reason this is a
// function rather than a literal at each call site. A user whose credential is
// expired still authenticates and still gets a token — otherwise they could never
// reach the endpoint that fixes it — but that token carries ONLY
// user:change-password, and only if their own bundle actually contains it.
// Everything else is dropped, `*:*` included.
//
// The consequence is deliberate: a session that exists to rotate an expired
// credential cannot read a user, cannot list a tenant, cannot do anything but the
// one thing it is for. If the bundle does not contain the permission at all, the
// claim is an EMPTY list and the account is a dead end until a helpdesk reset —
// which is the honest fail-closed reading, because inventing the permission would
// hand out a grant nobody issued.
// THE TENANT-DEFINED CLAIMS ARE MERGED UNDER THE FIXED SET, never over it. The
// resolution already refuses a definition carrying a platform name, so the
// order changes nothing today; it is written this way because the two guards
// fail in opposite directions. Assigning the fixed set last means that if the
// first guard is ever wrong — a name added to the platform's vocabulary that an
// operator had already seeded, a row written straight into the table — the
// consequence is a tenant claim that quietly does not appear, rather than
// `permissions` or `tenant_id` being replaced by a value the tenant chose.
// Those two are read by the framework across the whole mesh.
func buildClaims(user *appdomain.User, roleKeys []string, permissions []vos.PermissionKey, custom map[string]any) map[string]any {
	claims := make(map[string]any, len(custom)+9)
	maps.Copy(claims, custom)
	claims[claimIdentityKind] = identityKindUser
	claims[claimTenantID] = user.TenantID.Value()
	claims[claimTenantWorkspace] = user.TenantWorkspace
	claims[claimEmail] = user.Email.Value()
	claims[claimName] = user.Name.FullName()
	claims[claimPermissions] = effectivePermissions(user, permissions)
	claims[claimGroups] = groupKeysOf(user)
	claims[claimRoles] = roleKeys
	claims[claimMustChangePassword] = user.MustChangePassword
	return claims
}

// effectivePermissions is the ONE decision about what this user may attempt, and
// both the token and the response body read it.
//
// It exists as a function rather than as two call sites because the two MUST NOT
// disagree: a body advertising `*:*` beside a token that carries only
// user:change-password would have the client offering actions every request then
// refuses — and the first version of this file had exactly that bug, caught by
// the test that compares the two.
func effectivePermissions(user *appdomain.User, permissions []vos.PermissionKey) []string {
	rendered := renderPermissions(permissions)
	if user.MustChangePassword {
		return restrictToPasswordChange(rendered)
	}
	return rendered
}

// renderPermissions turns resolved keys into the wire vocabulary the framework's
// Identity reads back: a []string of "resource:action".
//
// parsePermissionsClaim accepts several shapes; this is the one that survives a
// JSON round trip unambiguously. The result is always non-nil, so the claim is
// PRESENT and empty rather than absent — which is how a consumer tells "this user
// holds nothing" from "this token predates permissions".
func renderPermissions(keys []vos.PermissionKey) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if key.Resource == "" || key.Action == "" {
			continue
		}
		out = append(out, key.Resource+":"+key.Action)
	}
	return out
}

// restrictToPasswordChange keeps at most the one permission an expired-credential
// session is allowed to carry.
func restrictToPasswordChange(permissions []string) []string {
	for _, p := range permissions {
		if p == PermissionChangeOwnPassword {
			return []string{PermissionChangeOwnPassword}
		}
	}
	// NOT a wildcard fallback, and not the permission granted anyway. A user whose
	// bundle lacks it is stuck until somebody with user:reset-password acts, and
	// that is the correct answer: the alternative is minting a grant that no role
	// in this tenant confers.
	return []string{}
}

// groupKeysOf reads the memberships off the loaded aggregate.
//
// The KEYS only. A group's display name and description are mutable and nothing
// decides on them, and the schema says outright that a key — not a display name —
// is what an API caller and an audit line reference.
func groupKeysOf(user *appdomain.User) []string {
	entries := domain.GetCurrentItemsOf[aggregatevos.UserGroup](user.GetAggregateRoot())
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.GroupKey != "" {
			out = append(out, entry.GroupKey)
		}
	}
	return out
}

// buildProfile assembles the richer shape the response body carries.
//
// It is where the display names live — the half deliberately kept out of the
// token. Roles come from the RESOLVED grants rather than from the aggregate,
// because the aggregate carries only the DIRECT grants and a client showing "your
// roles" has to see the ones inherited through a group too. That is also why the
// role entries have no display name: a role reached through a group was never
// loaded as a row here.
// The custom map is PASSED IN rather than resolved here, and that is the whole
// anti-drift argument effectivePermissions makes one function up, applied to the
// second thing this body and that token now both carry. One resolution runs per
// request and both readers consume it, so the body cannot advertise a claim the
// token omits — including the case that makes the two most likely to disagree,
// where a must-change-password session carries none at all.
func buildProfile(user *appdomain.User, roleKeys []string, permissions []vos.PermissionKey, custom map[string]any) AuthenticatedUserResult {
	groupEntries := domain.GetCurrentItemsOf[aggregatevos.UserGroup](user.GetAggregateRoot())
	groups := make([]NamedGrantResult, 0, len(groupEntries))
	for _, entry := range groupEntries {
		groups = append(groups, NamedGrantResult{Key: entry.GroupKey, Name: entry.GroupName})
	}

	// The direct grants carry a display name from the read join; the inherited
	// ones do not exist as rows on this aggregate. Naming the ones we can is
	// better than naming none, and a client that needs every name has the roles
	// endpoint.
	directNames := map[string]string{}
	for _, entry := range domain.GetCurrentItemsOf[aggregatevos.UserRole](user.GetAggregateRoot()) {
		directNames[entry.RoleKey] = entry.RoleName
	}
	roles := make([]NamedGrantResult, 0, len(roleKeys))
	for _, key := range roleKeys {
		roles = append(roles, NamedGrantResult{Key: key, Name: directNames[key]})
	}

	return AuthenticatedUserResult{
		ID:                 idOf(user).Value(),
		Name:               user.Name.FullName(),
		Email:              user.Email.Value(),
		Status:             user.Status.Value(),
		MustChangePassword: user.MustChangePassword,
		TenantID:           user.TenantID.Value(),
		TenantWorkspace:    user.TenantWorkspace,
		Groups:             groups,
		Roles:              roles,
		Permissions:        effectivePermissions(user, permissions),
		Claims:             custom,
	}
}

// idOf reads the loaded row's id.
//
// GetID answers a pointer that is nil for an entity that was never persisted. A
// loaded row always has one, so the nil branch is unreachable here — but reading
// it without checking would be a panic waiting for the first caller who passes an
// unsaved entity, and the zero ID it returns instead is refused by every consumer.
func idOf(user *appdomain.User) domain.ID {
	if id := user.GetID(); id != nil {
		return *id
	}
	return domain.ID{}
}
