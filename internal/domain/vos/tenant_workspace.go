// Hand-written: declared in the spec as `kind: manual`. What puts it beyond a
// regex is the reserved list — the handles the platform keeps for itself.
//
// DisplayName and Description are shared because every rule they carry is about
// human-typed text. This one carries a rule about THIS SERVICE: the routes it
// serves. It stays specific.
//
// It used to also derive the tenant's public key (a UUIDv5 over the handle).
// That key was removed on 2026-08-24 — the row id is the tenant's only
// identifier now — and the derivation went with it.

package vos

import (
	"regexp"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

const (
	tenantWorkspaceMinRunes = 3
	// 63 is the DNS label ceiling, which is what every product surveyed for
	// this model converges on — the handle reaches a subdomain eventually.
	tenantWorkspaceMaxRunes = 63

	tenantWorkspaceMaxIdenticalRun = 4
	tenantWorkspaceMinDistinct     = 3
)

// tenantWorkspacePattern is RFC 1123 relaxed: lowercase alphanumerics in
// hyphen-separated groups. A handle MAY lead with a digit — "3m9", "3m-brasil"
// are real companies — which is why RFC 1035's leading-letter rule is not used.
// Hyphens never lead, trail or double.
var tenantWorkspacePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// reservedTenantWorkspaces are the handles the platform keeps for itself.
//
// Two populations, both deliberate: routes this service actually serves today
// (docs, graphql, openapi, livez, readyz, and the tenants collection segment),
// and words whose registration by a customer would be phishing-adjacent (admin,
// billing, login, security, support). Editing this list is cheap and expected —
// it is one slice and one test.
var reservedTenantWorkspaces = map[string]struct{}{
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

// TenantWorkspace is the tenant's immutable, human-facing handle: the value
// that reaches URLs, logs and support conversations, and the input the public
// tenant_id is derived from.
type TenantWorkspace string

func (v TenantWorkspace) Value() string { return string(v) }

// IsReserved reports whether the handle is one the platform keeps for itself.
// Exported so the reserved list can be asserted directly by a test and read by
// a future "is this handle available?" endpoint without duplicating the set.
func (v TenantWorkspace) IsReserved() bool {
	_, reserved := reservedTenantWorkspaces[string(v)]
	return reserved
}

// IsValid enforces the handle's shape and its reservation.
//
// Unlike the two text types, this one can raise TWO different answers, so both
// are evaluated: a malformed handle and a reserved handle are different
// problems with different fixes, and a caller is entitled to be told which one
// they hit.
//
// NOTHING IS NORMALIZED. " Acme " and "ACME-CORP" are refused, never quietly
// repaired. For a handle that is immutable, reserved forever and feeds a
// derived public key, storing something the caller did not send is worse than a
// 422: they would believe they own "ACME-CORP", every log line would say
// "acme-corp", the derived tenant_id would come from a value they never typed,
// and there is no second chance to correct it.
func (v TenantWorkspace) IsValid(fieldName string, ctx *domain.NotificationContext) bool {
	s := string(v)
	if s == "" {
		ctx.AddNotificationNamed(fieldName, domain.RequiredFieldNotification{})
		return false
	}

	valid := true

	length := runeLen(s)
	wellFormed := length >= tenantWorkspaceMinRunes &&
		length <= tenantWorkspaceMaxRunes &&
		tenantWorkspacePattern.MatchString(s) &&
		distinctRunes(s) >= tenantWorkspaceMinDistinct &&
		!hasRunOfIdenticalRunes(s, tenantWorkspaceMaxIdenticalRun)

	if !wellFormed {
		ctx.AddNotificationNamed(fieldName, InvalidTenantWorkspaceNotification{}, s)
		valid = false
	}

	if v.IsReserved() {
		ctx.AddNotificationNamed(fieldName, ReservedTenantWorkspaceNotification{}, s)
		valid = false
	}

	return valid
}
