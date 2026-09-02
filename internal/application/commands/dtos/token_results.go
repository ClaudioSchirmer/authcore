// Hand-written, and not a hook: no generator declares this file.
//
// THE RESULT BOTH TOKEN VERBS ANSWER WITH, and the one shape a client renders
// after either. It is here rather than beside a command because it belongs to
// NEITHER: the layout standard co-locates a Result with its Command, and this one
// answers the sign-in and the rotation alike — co-locating it with one would make
// the other's file the odd import.
//
// NamedGrantResult is shared one step further still: the user profile and the
// client profile both carry it.
//
// THE RESULT CARRIES MORE THAN THE TOKEN, deliberately. The access token's claims
// are kept to identity and authorization — they ride in a header on every request
// to every service, and a group description is exactly the field that grows
// unnoticed until a proxy truncates the header — so the richer profile travels
// here, in a body read once at sign-in and never re-sent.

package dtos

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
