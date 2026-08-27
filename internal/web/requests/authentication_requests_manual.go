// Hand-written, and not a hook: no generator declares this file.
//
// The wire shapes for the two token operations. They live here, beside every
// generated request, for the reason the layering already implies: the command is
// an APPLICATION value and the body is a WEB one, and the `example:` tags below
// are presentation.
//
// THE RESPONSE IS DELIBERATELY RICHER THAN THE TOKEN. The access token's claims
// carry identity and authorization only — they ride in a header on every request
// to every service in the mesh, and a group's description is exactly the field
// that grows unnoticed until a proxy truncates the header. The display names
// travel HERE instead, in a body a client reads once at sign-in and never
// re-sends. Nothing on the authorization path depends on them.

package requests

import "github.com/ClaudioSchirmer/authcore/internal/application/commands"

// IssueTokenRequest is the body of the sign-in.
//
// Two fields and no third. No tenant, no workspace, no "remember me": the e-mail
// is unique across the whole platform, so it identifies the account on its own,
// and every other knob would be a way for a caller to influence a decision that
// is the server's.
type IssueTokenRequest struct {
	Email    string `json:"email" example:"ada@acme.test"`
	Password string `json:"password" example:"Str0ng!Passphrase"`
}

// ToCommand maps the wire shape onto the application command.
func (r IssueTokenRequest) ToCommand() *commands.IssueTokenCommand {
	return &commands.IssueTokenCommand{Email: r.Email, Password: r.Password}
}

// RefreshTokenRequest is the body of the rotation.
//
// It carries the opaque refresh value and NOTHING else — no subject, no tenant,
// no claims. Everything about who this is and what they may do is re-read from
// the database at redemption time, which is the whole reason a rotation propagates
// a revoked permission within minutes instead of at the next full sign-in. A
// caller-supplied identity here would be a caller-supplied authorization.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refreshToken" example:"Qk9HVVMtUkVGUkVTSC1UT0tFTi1FWEFNUExF"`
}

// ToCommand maps the wire shape onto the application command.
func (r RefreshTokenRequest) ToCommand() *commands.RefreshTokenCommand {
	return &commands.RefreshTokenCommand{RefreshToken: r.RefreshToken}
}

// TokenResponse is what both operations answer with.
//
// ONE SHAPE FOR BOTH, on purpose: a client that can read a sign-in can read a
// rotation, and a second type would be an invitation for the two to drift apart
// over exactly the field a client caches.
type TokenResponse struct {
	AccessToken string `json:"accessToken"`
	// The scheme to put in front of the token in an Authorization header. Always
	// "Bearer"; sent explicitly so a client never has to hardcode it.
	TokenType string `json:"tokenType"`
	// Unix seconds, not a formatted timestamp: a client computing "is this still
	// good" should never have to parse a date to do it.
	ExpiresAt int64 `json:"expiresAt"`
	// The opaque rotation credential. NOT a JWT and not inspectable — the only
	// thing a client may do with it is send it back.
	RefreshToken     string `json:"refreshToken"`
	RefreshExpiresAt int64  `json:"refreshExpiresAt"`

	User AuthenticatedUserResponse `json:"user"`
}

// AuthenticatedUserResponse is the profile a client renders after signing in.
type AuthenticatedUserResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	// The account's own lifecycle state. Always "active" here — a suspended
	// account cannot reach this response — and present so a client need not infer
	// it from the absence of a refusal.
	Status string `json:"status"`
	// When true the client must route straight to the password change: the token
	// it just received carries user:change-password and nothing else.
	MustChangePassword bool   `json:"mustChangePassword"`
	TenantID           string `json:"tenantId"`
	TenantWorkspace    string `json:"tenantWorkspace"`

	Groups []NamedGrantResponse `json:"groups"`
	// Every role held by ANY path — granted directly, or inherited through a
	// group. A role reached through a group carries no display name: it was never
	// loaded as a row on this aggregate, and inventing one would be worse than
	// leaving it empty.
	Roles []NamedGrantResponse `json:"roles"`
	// The effective permission set, as "resource:action". It mirrors the token's
	// `permissions` claim exactly — including the restriction applied when
	// mustChangePassword is true — so a client never has to decode the token to
	// know what it may attempt.
	Permissions []string `json:"permissions"`
}

// NamedGrantResponse is one group or role.
type NamedGrantResponse struct {
	Key  string `json:"key"`
	Name string `json:"name,omitempty"`
}

// FromResult is the response projection the route wires in.
//
// It is the single seat every surface shares, so REST and any future GraphQL or
// gRPC mirror of this operation render the same shape from the same result.
func (TokenResponse) FromResult(result commands.TokenResult) TokenResponse {
	return TokenResponse{
		AccessToken:      result.AccessToken,
		TokenType:        result.TokenType,
		ExpiresAt:        result.ExpiresAt,
		RefreshToken:     result.RefreshToken,
		RefreshExpiresAt: result.RefreshExpiresAt,
		User: AuthenticatedUserResponse{
			ID:                 result.User.ID,
			Name:               result.User.Name,
			Email:              result.User.Email,
			Status:             result.User.Status,
			MustChangePassword: result.User.MustChangePassword,
			TenantID:           result.User.TenantID,
			TenantWorkspace:    result.User.TenantWorkspace,
			Groups:             namedGrants(result.User.Groups),
			Roles:              namedGrants(result.User.Roles),
			Permissions:        nonNilStrings(result.User.Permissions),
		},
	}
}

// namedGrants maps the application shape onto the wire one.
//
// Always a non-nil slice, so the field renders as `[]` rather than `null`: a
// client iterating it should not have to guard, and "this user is in no groups"
// is an empty list, not a missing one.
func namedGrants(items []commands.NamedGrantResult) []NamedGrantResponse {
	out := make([]NamedGrantResponse, 0, len(items))
	for _, item := range items {
		out = append(out, NamedGrantResponse{Key: item.Key, Name: item.Name})
	}
	return out
}

// nonNilStrings guarantees `[]` over `null` for the same reason.
func nonNilStrings(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}
