// Hand-written, and not a hook: no generator declares this file.
//
// The wire shapes for the machine sign-in. They live here, beside every generated
// request, for the reason the layering already implies: the command is an
// APPLICATION value and the body is a WEB one, and the `example:` tags below are
// presentation.
//
// THE RESPONSE IS DELIBERATELY RICHER THAN THE TOKEN, exactly as the user one is:
// the access token's claims carry identity and authorization only, because they ride
// in a header on every request to every service. The display names travel here, in a
// body an integration reads once at sign-in and never re-sends.

package requests

import (
	"github.com/ClaudioSchirmer/authcore/internal/application/commands"
	"github.com/ClaudioSchirmer/authcore/internal/web/requests/dtos"
)

// IssueClientTokenRequest is the body of the machine sign-in.
//
// Two fields and no third. No tenant and no workspace: the client id is this
// service's own row id, unique across the platform, so it identifies the
// integration on its own.
//
// NO `grantType` FIELD, and that absence is the same call this service made when it
// split the two credential routes: the subject type is the URL segment, so a body
// whose required fields change with the value of one of them never exists here. The
// market standard (Auth0, Okta, Keycloak: one endpoint plus grant_type) is RFC 6749
// — form-encoded in, flat snake_case out — and this API is neither, so the
// compatibility it would buy is not on the table.
type IssueClientTokenRequest struct {
	ClientID string `json:"clientId" example:"018f2c7e-9a41-7b3d-8c52-2f7a1d9e4b60"`
	// The plaintext secret, as it was handed back by the operation that minted it —
	// the create, or a rotation. Nothing stores it, so nothing can hand it out
	// again.
	ClientSecret string `json:"clientSecret" example:"acs_8xQvR2mK9dLpN4wZ7tYcB1hJ6sF3gA5eU0iO8rTvXyM"`
}

// ToCommand maps the wire shape onto the application command.
func (r IssueClientTokenRequest) ToCommand() *commands.IssueClientTokenCommand {
	return &commands.IssueClientTokenCommand{ClientID: r.ClientID, ClientSecret: r.ClientSecret}
}

// ClientTokenResponse is what the machine sign-in answers with.
//
// A SHAPE OF ITS OWN, not TokenResponse. It carries NO refreshToken and no
// refreshExpiresAt, because a client-credentials grant issues none: the secret is
// already the long-lived credential, and an integration renews by presenting it
// again. Reusing the user shape would have put two always-empty credential fields
// on the wire, which is an invitation for a client library to read one.
type ClientTokenResponse struct {
	AccessToken string `json:"accessToken"`
	// The scheme to put in front of the token in an Authorization header. Always
	// "Bearer"; sent explicitly so a client never has to hardcode it.
	TokenType string `json:"tokenType"`
	// Unix seconds, not a formatted timestamp: a client computing "is this still
	// good" should never have to parse a date to do it. When it passes, sign in
	// again — there is nothing else to do and nothing else to store.
	ExpiresAt int64 `json:"expiresAt"`

	Client AuthenticatedClientResponse `json:"client"`
}

// AuthenticatedClientResponse is the profile an integration can log after signing
// in.
type AuthenticatedClientResponse struct {
	ID string `json:"id"`
	// The client's label, and the same value the token's `name` claim carries.
	Name string `json:"name"`
	// The integration's own lifecycle state. Always "active" here — a suspended
	// client cannot reach this response — and present so a caller need not infer it
	// from the absence of a refusal.
	Status          string `json:"status"`
	TenantID        string `json:"tenantId"`
	TenantWorkspace string `json:"tenantWorkspace"`

	// Every role granted to this client. There is one grant path — a client has no
	// groups — so unlike the user response every entry here was loaded as a row and
	// every one carries its display name.
	Roles []dtos.NamedGrantResponse `json:"roles"`
	// The effective permission set, as "resource:action". It mirrors the token's
	// `permissions` claim exactly, so a caller never has to decode the token to know
	// what it may attempt.
	Permissions []string `json:"permissions"`
	// The tenant-defined claims this token carries, by the name a consuming service
	// reads them under — the reserved `x_` namespace, always. Each value is rendered
	// in the JSON type its definition declares.
	//
	// It mirrors the token's custom claims exactly, from one resolution per request.
	// Only definitions whose `appliesTo` admits a client — `client` or `both` — are
	// resolved here; a `user`-scoped definition mints nothing on this route.
	Claims map[string]any `json:"claims"`
}

// FromResult is the response projection the route wires in.
//
// It is the single seat every surface shares, so REST and any future GraphQL or gRPC
// mirror of this operation render the same shape from the same result.
func (ClientTokenResponse) FromResult(result commands.ClientTokenResult) ClientTokenResponse {
	return ClientTokenResponse{
		AccessToken: result.AccessToken,
		TokenType:   result.TokenType,
		ExpiresAt:   result.ExpiresAt,
		Client: AuthenticatedClientResponse{
			ID:              result.Client.ID,
			Name:            result.Client.Name,
			Status:          result.Client.Status,
			TenantID:        result.Client.TenantID,
			TenantWorkspace: result.Client.TenantWorkspace,
			Roles:           dtos.NamedGrants(result.Client.Roles),
			Permissions:     dtos.NonNilStrings(result.Client.Permissions),
			Claims:          dtos.NonNilClaims(result.Client.Claims),
		},
	}
}
