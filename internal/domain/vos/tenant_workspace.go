package vos

import (
	"regexp"

	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/google/uuid"
)

// TenantWorkspace is the tenant's public handle — the value that reaches URLs,
// logs, support conversations and other services' parsers.
//
// It stays entity-specific while DisplayName and Description are shared,
// because two of its rules are about THIS platform and nothing else: the
// reserved list is this service's own routes, and DeriveTenantID is the
// function that turns the handle into the tenant's public key.
//
// # Why the shape is not a taste decision
//
// The value lands in a hostname or a URL path, so it is a DNS label: letters,
// digits and hyphens, never leading, trailing or doubled, at most 63 octets.
// Every product that puts a tenant in a subdomain converges on the same shape
// — Auth0 is exactly 3–63. RFC 1035 additionally required a leading letter and
// RFC 1123 relaxed that, which is why "3m" is accepted here.
//
// # Why it is immutable and never reused
//
// TenantID is a pure function of this value, so re-issuing a workspace
// re-issues the IDENTICAL TenantID. A recycled handle would mint a tenant
// whose public key is byte-identical to the archived one's, and every token
// ever issued for the old tenant would authorize against the new one's data:
// valid signature, correct claim, wrong tenant, no anomaly to detect. That is
// why the uniqueness of this field is scoped to ALL rows, archived included,
// and why an update may not touch it.
type TenantWorkspace string

// TenantIDNamespace is the UUIDv5 namespace every tenant's public key is
// derived under. It was generated once, for this service.
//
// IT MUST NEVER CHANGE. Changing it re-derives every TenantID in existence,
// which invalidates every issued token and every foreign key pointing at one,
// with no migration path short of reissuing the whole platform's tokens. It is
// deliberately a project constant and not configuration, so that it cannot be
// changed by a deployment.
//
// A service-specific namespace is used, rather than a standard DNS or URL one,
// so that no other system deriving from the same handle can produce a
// colliding value.
const TenantIDNamespace = "e2937874-80cb-4b5f-b113-21741931ac1a"

var tenantIDNamespace = uuid.MustParse(TenantIDNamespace)

// workspacePattern is the DNS-label shape: lowercase alphanumerics in groups
// separated by single hyphens, so a hyphen can never lead, trail or double.
var workspacePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// reservedWorkspaces are the handles the platform keeps for itself.
//
// Two reasons, and the first is not hypothetical: docs, graphql, livez, readyz
// and openapi are routes this service serves TODAY, and tenant/tenants is the
// REST collection segment. The second is that grabbing "admin", "login" or
// "security" is a cheap phishing surface.
var reservedWorkspaces = map[string]struct{}{
	"admin": {}, "administrator": {}, "api": {}, "app": {}, "apps": {},
	"assets": {}, "auth": {}, "billing": {}, "cdn": {}, "console": {},
	"dashboard": {}, "dev": {}, "docs": {}, "files": {}, "ftp": {},
	"graphql": {}, "help": {}, "host": {}, "id": {}, "internal": {},
	"livez": {}, "login": {}, "logout": {}, "mail": {}, "media": {},
	"metrics": {}, "new": {}, "oauth": {}, "openapi": {}, "platform": {},
	"public": {}, "readyz": {}, "register": {}, "root": {}, "security": {},
	"settings": {}, "signin": {}, "signup": {}, "smtp": {}, "sso": {},
	"staging": {}, "static": {}, "status": {}, "support": {}, "system": {},
	"tenant": {}, "tenants": {}, "test": {}, "user": {}, "users": {},
	"www": {},
}

// Value is the underlying primitive the persister binds and the mappers read.
func (v TenantWorkspace) Value() string { return string(v) }

// DeriveTenantID computes the tenant's public key: UUIDv5 of the handle under
// TenantIDNamespace.
//
// It is a PURE FUNCTION, and that is the whole point of deriving rather than
// minting a second random id: any service that knows the handle reaches the
// same value offline, with no lookup and no call back to authcore.
//
// The value it produces is NOT a secret and must never be treated as one —
// UUIDv5 is SHA-1 over the namespace and the name, so it is brute-forceable
// back to the handle over a small dictionary. That costs nothing here, because
// the handle is public by design. What the derivation hides is the creation
// timestamp the UUIDv7 primary key would have leaked, and that it hides
// completely.
func (v TenantWorkspace) DeriveTenantID() domain.ID {
	return domain.NewIDFromUUID(uuid.NewSHA1(tenantIDNamespace, []byte(v)))
}

// IsValid is the framework's entry point, run on every write. It reports every
// problem it finds rather than returning at the first.
//
// Nothing here normalizes. " acme " and "ACME-CORP" are REFUSED, never quietly
// repaired: for a handle that is immutable, reserved forever and feeds a
// derived public key, storing something the caller did not send is worse than
// a 422 — the caller would believe they own ACME-CORP while every log line
// said acme-corp, and the public key would be derived from a value they never
// typed, with no second chance to correct it.
func (v TenantWorkspace) IsValid(fieldName string, ctx *domain.NotificationContext) bool {
	s := string(v)

	if s == "" {
		ctx.AddNotification(fieldName, domain.RequiredFieldNotification{})
		return false
	}

	valid := true
	invalid := func() {
		if valid {
			ctx.AddNotification(fieldName, InvalidTenantWorkspaceNotification{}, s)
			valid = false
		}
	}

	if n := runeLen(s); n < 3 || n > 63 {
		invalid()
	}
	if !workspacePattern.MatchString(s) {
		invalid()
	}
	if distinctRunes(s) < 3 {
		invalid()
	}
	if hasRunOfIdenticalRunes(s, 4) {
		invalid()
	}

	// Reported on its own notification: "this workspace is reserved" and "this
	// workspace is malformed" are different problems, and a caller that reads
	// the wrong one edits the wrong thing.
	if _, reserved := reservedWorkspaces[s]; reserved {
		ctx.AddNotification(fieldName, ReservedTenantWorkspaceNotification{}, s)
		valid = false
	}

	return valid
}
