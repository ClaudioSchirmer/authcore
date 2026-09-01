// Hand-written, and not a hook: no generator declares this file.
//
// THE NIL-GUARDS EVERY TOKEN RESPONSE SHARES, and the one mapper between the
// application's grant shape and the wire's.
//
// They are FUNCTIONS shared by more than one operation, so they live in utils/ while
// the shape they return lives in dtos/ next door.
//
// What they promise is one thing: a caller ranging over a collection or reading a key
// out of an object never has to guard first. An absent value renders as `[]` or `{}`
// and never as `null`, because "this integration holds no roles" is an empty list and
// not a missing one.

package utils

import (
	cmddtos "github.com/ClaudioSchirmer/authcore/internal/application/commands/dtos"
	"github.com/ClaudioSchirmer/authcore/internal/web/requests/dtos"
)

// nonNilClaims guarantees `{}` over `null`, for the reason nonNilStrings below
// guarantees `[]`: a client reading a key out of this object should not have to
// guard first, and "this user carries no custom claims" is an empty object, not
// a missing one.
func NonNilClaims(claims map[string]any) map[string]any {
	if claims == nil {
		return map[string]any{}
	}
	return claims
}

// namedGrants maps the application shape onto the wire one.
//
// Always a non-nil slice, so the field renders as `[]` rather than `null`: a
// client iterating it should not have to guard, and "this user is in no groups"
// is an empty list, not a missing one.
func NamedGrants(items []cmddtos.NamedGrantResult) []dtos.NamedGrantResponse {
	out := make([]dtos.NamedGrantResponse, 0, len(items))
	for _, item := range items {
		out = append(out, dtos.NamedGrantResponse{Key: item.Key, Name: item.Name})
	}
	return out
}

// nonNilStrings guarantees `[]` over `null` for the same reason.
func NonNilStrings(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}
