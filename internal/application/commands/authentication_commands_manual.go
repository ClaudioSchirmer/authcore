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

// AttemptRecorder is the lockout, and the forensic log behind it.
//
// It is a SEPARATE port from AuthenticationStore for the same reason
// RefreshTokenLookup is: a different adapter implements it, over a different
// table, and folding them together would give whichever grew first methods it has
// no business owning.
//
// THREE RECORDING METHODS RATHER THAN ONE WITH AN OUTCOME PARAMETER. An earlier
// draft passed an outcome value and a struct describing the attempt, which forced
// both a shared enum and a shared type — and neither had a home, since a type both
// layers name can live only in the domain, where a row shape with an IP in it does
// not belong. Naming the outcome in the METHOD dissolves the problem: the storage
// vocabulary stays with the table that owns it, and nothing crosses this seam but
// strings the caller already had.
//
// NOTHING ABOUT THE CREDENTIAL CROSSES IT EITHER. There is no parameter a password
// could arrive in — see the store and the 0007 migration for why that is a rule
// rather than an oversight.
type AttemptRecorder interface {
	// LockedUntil reports whether an identity is currently refused outright,
	// until when, and what its counted failures already established about
	// whether it names a real account.
	//
	// It answers identically for an identity that names an account and one that
	// does not — that uniformity IS the feature. The existence flag it returns
	// never reaches the caller of the endpoint; it exists so the `locked` row
	// this handler writes can carry it, which is the whole reason that column is
	// on the table.
	LockedUntil(ctx context.Context, identity string) (until time.Time, locked bool, identityExisted *bool, err error)

	// RecordFailure logs a credential presented and rejected. The only outcome
	// that counts toward a lockout. identityExisted is nil when the lookup itself
	// failed and the answer is genuinely unknown.
	RecordFailure(ctx context.Context, identity, kind, ip string, identityExisted *bool) error

	// RecordSuccess logs a credential that verified. It also ANCHORS the window:
	// failures before it stop counting, which is how "a successful sign-in clears
	// the counter" works with no counter to clear.
	RecordSuccess(ctx context.Context, identity, kind, ip string) error

	// RecordLocked logs an attempt refused because the identity was already
	// locked — recorded so a reviewer sees the persistence, never counted so the
	// persistence cannot extend the lock.
	//
	// identityExisted is CARRIED IN from LockedUntil rather than looked up: this
	// path deliberately never asks. Stamping it here is what lets somebody filter
	// `WHERE outcome = 'locked'` on its own and still see which locks are real
	// accounts under attack and which are noise — the question that column was
	// added to answer.
	RecordLocked(ctx context.Context, identity, kind, ip string, identityExisted *bool) error
}

// identityKindUser is what this route's attempts are logged as. The client
// credentials route will pass its own; the store takes it as a string precisely
// so neither side has to import the other's vocabulary.
const identityKindUser = "user"

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
	until, locked, knownToExist, err := h.Attempts.LockedUntil(ctx, email)
	if err != nil {
		// NOT a refusal. A lockout probe that cannot run has not established
		// anything, and answering 401 would tell a caller with a correct password
		// that it was wrong. It escapes as an exception → 500.
		return TokenResult{}, err
	}
	if locked {
		// Recorded, so a reviewer sees somebody kept trying through the lock —
		// and NOT counted, so the trying does not extend it.
		// knownToExist comes from the failures that caused this lock — the probe
		// read them anyway. Nothing here looked the account up, and nothing should:
		// past the threshold the point is to stop paying for guesses.
		if rerr := h.Attempts.RecordLocked(ctx, email, identityKindUser, ip, knownToExist); rerr != nil {
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
		if rerr := h.Attempts.RecordFailure(ctx, email, identityKindUser, ip, existed); rerr != nil {
			return TokenResult{}, rerr
		}
		return TokenResult{}, refuseCredentials()
	}

	// From here the identity provably exists, and every remaining branch says so.
	existed := true

	if !h.Store.PasswordMatches(cmd.Password, user.PasswordHash) {
		if rerr := h.Attempts.RecordFailure(ctx, email, identityKindUser, ip, &existed); rerr != nil {
			return TokenResult{}, rerr
		}
		return TokenResult{}, refuseCredentials()
	}

	if !accountIsUsable(user) {
		// A suspended account or a withdrawn tenant is a FAILURE in the log, not a
		// success: nobody got in. It counts toward the lockout like any other,
		// which is correct — repeatedly presenting a valid credential for a
		// disabled account is exactly the pattern worth rate-limiting.
		if rerr := h.Attempts.RecordFailure(ctx, email, identityKindUser, ip, &existed); rerr != nil {
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

	access, refresh, err := h.Issuer.IssueWithRefresh(ctx, authcore.TokenRequest{
		Subject: idOf(user).Value(),
		Claims:  buildClaims(user, roleKeys, permissions),
	})
	if err != nil {
		return TokenResult{}, err
	}

	// Recorded LAST, once the sign-in has actually succeeded. It is what anchors
	// the window — every failure before this moment stops counting — so writing it
	// any earlier would clear a counter for a sign-in that had not happened yet.
	if rerr := h.Attempts.RecordSuccess(ctx, email, identityKindUser, ip); rerr != nil {
		return TokenResult{}, rerr
	}

	return TokenResult{
		AccessToken:      access.Token,
		TokenType:        tokenTypeBearer,
		ExpiresAt:        access.ExpiresAt.Unix(),
		RefreshToken:     refresh.Value,
		RefreshExpiresAt: refresh.ExpiresAt.Unix(),
		User:             buildProfile(user, roleKeys, permissions),
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

	access, refresh, err := h.Issuer.RedeemRefreshToken(ctx, value, buildClaims(user, roleKeys, permissions))
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
		User:             buildProfile(user, roleKeys, permissions),
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
func buildClaims(user *appdomain.User, roleKeys []string, permissions []vos.PermissionKey) map[string]any {
	return map[string]any{
		claimTenantID:           user.TenantID.Value(),
		claimTenantWorkspace:    user.TenantWorkspace,
		claimEmail:              user.Email.Value(),
		claimName:               user.Name.FullName(),
		claimPermissions:        effectivePermissions(user, permissions),
		claimGroups:             groupKeysOf(user),
		claimRoles:              roleKeys,
		claimMustChangePassword: user.MustChangePassword,
	}
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
func buildProfile(user *appdomain.User, roleKeys []string, permissions []vos.PermissionKey) AuthenticatedUserResult {
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
