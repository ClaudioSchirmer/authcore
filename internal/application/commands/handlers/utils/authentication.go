// Hand-written, and not a hook: no generator declares this file.
//
// WHAT THE TOKEN HANDLERS SHARE. The layout standard puts one hand-written
// handler per file, which leaves the things two or three of them reach for with
// nowhere else to live: the ports they take, the claim vocabulary they mint, the
// single Refusal they all answer with, and the two builders that keep a token and
// its response body from ever disagreeing.
//
// EVERYTHING HERE IS USED BY MORE THAN ONE HANDLER. Anything a single handler owns
// stays in that handler's file — the client-token helpers, the two row-scope feeds —
// so this file cannot quietly become the drawer everything lands in.

package utils

import (
	"maps"
	"strings"

	cmdutils "github.com/ClaudioSchirmer/authcore/internal/application/commands/utils"
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/authcore/internal/infra"
	"github.com/ClaudioSchirmer/authcore/internal/infra/schemas"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/application/exception"
	"github.com/ClaudioSchirmer/omnicore/domain"
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

// TokenTypeBearer is the scheme both operations answer with. It is what the
// caller must put in front of the token in an Authorization header, and the only
// scheme the framework's middleware parses.
const TokenTypeBearer = "Bearer"

// The claim names this service mints.
//
// Two of them are NOT ours to choose. `permissions` and `tenant_id` are what the
// framework's Identity reads back (application/configuration/authz_config.go), so
// every service in the mesh depends on those exact spellings; renaming either
// silently disables authorization everywhere rather than failing loudly. The rest
// are ours, and they are snake_case to match the two that are not.
const (
	ClaimPermissions        = "permissions"
	ClaimTenantID           = "tenant_id"
	ClaimTenantWorkspace    = "tenant_workspace"
	ClaimEmail              = "email"
	ClaimName               = "name"
	ClaimGroups             = "groups"
	ClaimRoles              = "roles"
	ClaimMustChangePassword = "must_change_password"

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
	ClaimIdentityKind = "identity_kind"
)

// ContextKeyClientIP is where the /auth middleware leaves the request's origin
// address for the handler to read.
//
// It rides the AppContext's generic bag because the framework's AppContext
// exposes no IP of its own, and a pipeline.Handler receives that context rather
// than the Fiber one. The alternative — MountRaw, to reach c.IP() directly —
// would have cost the canonical envelope and the seven catalogs for one string.
const ContextKeyClientIP = "authcore.request.ip"

// Refusal wraps one notification in the carrier the pipeline understands.
//
// An APPLICATION error, not a domain one: nothing in the domain decided this, and
// the two are distinguishable at a glance in a log. The pipeline treats either
// through domain.NotificationCarrier, so the choice costs nothing and records
// which layer refused.
//
// The context name and the field name are fixed HERE rather than at each call
// site, so no future branch can refuse under a field name that says which half of
// the credential was wrong.
func Refusal(n domain.Notification) error {
	return exception.NewApplicationErrorWith("Authentication", domain.NotificationMessage{
		FieldName:    "credentials",
		Notification: n,
	})
}

// ClientIPOf reads the origin address the /auth middleware left on the context.
//
// Absent is "" rather than an error: an attempt with no recorded origin is still
// an attempt worth counting, and refusing a sign-in because a forensic field was
// missing would trade the operation for its own log.
func ClientIPOf(ctx *configuration.AppContext) string {
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

// AccountIsUsable reports whether this account may hold a session at all.
//
// TWO conditions, and the second is the one that is easy to forget. The account
// itself must be active — a suspended user is refused — and its owning TENANT
// must not be commercially suspended, because a customer who stopped paying
// should not be issuing credentials. Trial and active tenants both pass: a trial
// is a live customer being onboarded, and signing in is the first thing they need.
//
// The archive stamp needs no check here: the loader's default scope already
// refuses archived rows, so an archived account never reaches this function.
func AccountIsUsable(account *schemas.SignInAccount) bool {
	if account.Status != vos.UserStatusActive.Value() {
		return false
	}
	// The joined column, filled on every load by the repository's InnerJoin into
	// Tenant. An empty value means the join found nothing, which for an INNER join
	// over a NOT NULL key cannot happen — but reading it as "unusable" is the
	// fail-closed direction if it ever does.
	if account.TenantStatus == "" || account.TenantStatus == vos.TenantStatusSuspended.Value() {
		return false
	}
	return true
}

// BuildClaims assembles the access token's claim set.
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
func BuildClaims(account *schemas.SignInAccount, bundle infra.SignInBundle, custom map[string]any) map[string]any {
	claims := make(map[string]any, len(custom)+9)
	maps.Copy(claims, custom)
	claims[ClaimIdentityKind] = IdentityKindUser
	claims[ClaimTenantID] = account.TenantID.Value()
	claims[ClaimTenantWorkspace] = account.TenantWorkspace
	claims[ClaimEmail] = account.Email
	claims[ClaimName] = strings.TrimSpace(account.GivenName + " " + account.FamilyName)
	claims[ClaimPermissions] = EffectivePermissions(account, bundle.Permissions)
	claims[ClaimGroups] = KeysOf(bundle.Groups)
	claims[ClaimRoles] = KeysOf(bundle.Roles)
	claims[ClaimMustChangePassword] = account.MustChangePassword
	return claims
}

// KeysOf is the token's half of a named grant. The display name is deliberately
// left in the response body: a claim set is read by every service in the mesh on
// every request, and a name authorizes nothing.
func KeysOf(grants []infra.NamedGrant) []string {
	out := make([]string, 0, len(grants))
	for _, g := range grants {
		out = append(out, g.Key)
	}
	return out
}

// EffectivePermissions is the ONE decision about what this user may attempt, and
// both the token and the response body read it.
//
// It exists as a function rather than as two call sites because the two MUST NOT
// disagree: a body advertising `*:*` beside a token that carries only
// user:change-password would have the client offering actions every request then
// refuses — and the first version of this file had exactly that bug, caught by
// the test that compares the two.
func EffectivePermissions(account *schemas.SignInAccount, permissions []vos.PermissionKey) []string {
	rendered := RenderPermissions(permissions)
	if account.MustChangePassword {
		return restrictToPasswordChange(rendered)
	}
	return rendered
}

// RenderPermissions turns resolved keys into the wire vocabulary the framework's
// Identity reads back: a []string of "resource:action".
//
// parsePermissionsClaim accepts several shapes; this is the one that survives a
// JSON round trip unambiguously. The result is always non-nil, so the claim is
// PRESENT and empty rather than absent — which is how a consumer tells "this user
// holds nothing" from "this token predates permissions".
func RenderPermissions(keys []vos.PermissionKey) []string {
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

// BuildProfile assembles the richer shape the response body carries.
//
// It is where the display names live — the half deliberately kept out of the
// token. Roles come from the RESOLVED grants rather than from the aggregate,
// because the aggregate carries only the DIRECT grants and a client showing "your
// roles" has to see the ones inherited through a group too. That is also why the
// role entries have no display name: a role reached through a group was never
// loaded as a row here.
// The custom map is PASSED IN rather than resolved here, and that is the whole
// anti-drift argument EffectivePermissions makes one function up, applied to the
// second thing this body and that token now both carry. One resolution runs per
// request and both readers consume it, so the body cannot advertise a claim the
// token omits — including the case that makes the two most likely to disagree,
// where a must-change-password session carries none at all.
func BuildProfile(account *schemas.SignInAccount, bundle infra.SignInBundle, custom map[string]any) cmdutils.AuthenticatedUserResult {
	return cmdutils.AuthenticatedUserResult{
		ID:                 account.ID.Value(),
		Name:               strings.TrimSpace(account.GivenName + " " + account.FamilyName),
		Email:              account.Email,
		Status:             account.Status,
		MustChangePassword: account.MustChangePassword,
		TenantID:           account.TenantID.Value(),
		TenantWorkspace:    account.TenantWorkspace,
		Groups:             NamedGrantsOf(bundle.Groups),
		Roles:              NamedGrantsOf(bundle.Roles),
		Permissions:        EffectivePermissions(account, bundle.Permissions),
		Claims:             custom,
	}
}

// NamedGrantsOf is the body's half of a grant: key AND name.
//
// EVERY ROLE CARRIES A NAME NOW, inherited ones included. The statement this
// replaced could only name the DIRECT grants — an inherited role was never loaded
// as a row — so the body used to show a blank name for half of them.
func NamedGrantsOf(grants []infra.NamedGrant) []cmdutils.NamedGrantResult {
	out := make([]cmdutils.NamedGrantResult, 0, len(grants))
	for _, g := range grants {
		out = append(out, cmdutils.NamedGrantResult{Key: g.Key, Name: g.Name})
	}
	return out
}
