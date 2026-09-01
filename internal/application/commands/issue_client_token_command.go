// Hand-written, and not a hook: no generator declares this file.
//
// The machine sign-in's COMMAND and RESULT, one level above the handler that
// consumes them, per the layout standard.
//
// THE RESULT IS ITS OWN TYPE RATHER THAN dtos.TokenResult, and not for tidiness: four
// of that type's fields have no meaning here — RefreshToken, RefreshExpiresAt,
// and inside the profile, Email, Groups and MustChangePassword. Sharing it would
// mean six fields that are always zero, which is six invitations for a client to
// read one and for the two shapes to drift over exactly the field somebody caches.

package commands

import (
	"github.com/ClaudioSchirmer/authcore/internal/application/commands/dtos"
	"github.com/ClaudioSchirmer/omnicore/application/pipeline"
)

// ClientTokenResult is what the machine sign-in answers with.
//
// A TYPE OF ITS OWN RATHER THAN dtos.TokenResult, and not for tidiness: four of that
// type's fields have no meaning here — RefreshToken, RefreshExpiresAt, and inside
// the profile, Email, Groups and MustChangePassword. Sharing it would mean six
// fields that are always zero, which is six invitations for a client to read one
// and for the two shapes to drift over exactly the field somebody caches.
type ClientTokenResult struct {
	AccessToken string
	TokenType   string
	ExpiresAt   int64
	Client      AuthenticatedClientResult
}

// AuthenticatedClientResult is the profile an integration can log after signing in.
//
// It carries MORE than the token deliberately, for the reason its user twin does:
// display names ride here, in a body read once, rather than in claims that ride in
// a header on every request to every service.
type AuthenticatedClientResult struct {
	ID              string
	Name            string
	Status          string
	TenantID        string
	TenantWorkspace string
	Roles           []dtos.NamedGrantResult
	Permissions     []string
	// The tenant-defined claims this token carries, resolved down the two-level
	// chain and typed per each definition's declared value type.
	//
	// IT MIRRORS THE TOKEN EXACTLY, from ONE resolution per request — the same
	// anti-drift rule the user path enforces on its permissions and claims. Neither
	// reader re-derives the other's answer.
	Claims map[string]any
}

// IssueClientTokenCommand is what the machine sign-in route binds.
type IssueClientTokenCommand struct {
	pipeline.CommandWithBodyBase

	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}
