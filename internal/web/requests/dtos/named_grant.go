// Hand-written, and not a hook: no generator declares this file.
//
// THE WIRE SHAPES AND GUARDS BOTH TOKEN RESPONSES SHARE. A named grant renders
// identically for a person and for a machine, and the three nil-guards below are
// the same promise on both: a caller ranging over a collection or reading a key
// out of an object should never have to guard first, so an absent value renders as
// `[]` or `{}` and never as `null`.
//
// They are here rather than in one of the two operation files because the layout
// standard makes that unit the OPERATION — a helper living in the sign-in's file
// and called from the machine sign-in's would make one operation's file the other's
// dependency.

package dtos

import cmdutils "github.com/ClaudioSchirmer/authcore/internal/application/commands/utils"

// NamedGrantResponse is one group or role.
type NamedGrantResponse struct {
	Key  string `json:"key"`
	Name string `json:"name,omitempty"`
}

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
func NamedGrants(items []cmdutils.NamedGrantResult) []NamedGrantResponse {
	out := make([]NamedGrantResponse, 0, len(items))
	for _, item := range items {
		out = append(out, NamedGrantResponse{Key: item.Key, Name: item.Name})
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
