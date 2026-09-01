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

// NamedGrantResponse is one group or role.
type NamedGrantResponse struct {
	Key  string `json:"key"`
	Name string `json:"name,omitempty"`
}
