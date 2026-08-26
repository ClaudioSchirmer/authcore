// Hand-written, and not a hook: no generator declares this file.
//
// It is the domain half of the two credential operations. The spec language has
// no way to declare a custom operation — `authz.permissions` takes a closed set
// of verbs and none of them is "change the password" — so the commands, the
// handlers and the routes are all by hand. What is NOT by hand is the judgement:
// the rules below live in the aggregate like every other rule, dispatched by the
// framework, reporting through the same notification context and answering in
// the same seven languages.
//
// THE ACTION NAME IS WHAT SEPARATES THEM FROM A PATCH. Both operations dispatch
// ModeUpdate, which is also what PATCH /users/:id dispatches, so a rule that
// keyed on the mode alone would run the password checks on a rename. BuildRules
// receives the actionName the caller passed to domain.GetUpdatable, and that is
// the discriminator.

package domain

import (
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

const (
	// ActionChangePassword is the PUBLIC operation: the caller proves possession
	// of the current password instead of carrying a token.
	ActionChangePassword = "ChangePassword"

	// ActionResetPassword is the AUTHENTICATED one: the caller proves who they
	// are instead, and does not know the current password. It is what a helpdesk
	// and a self-service "I am logged in and want a new password" both use.
	ActionResetPassword = "ResetPassword"
)

// IsCredentialAction reports whether an actionName is one of the two.
//
// Exported so the web layer can assert it rather than repeating the strings —
// a third caller spelling "changePassword" would silently get the ordinary
// patch rules and no password validation at all.
func IsCredentialAction(actionName string) bool {
	return actionName == ActionChangePassword || actionName == ActionResetPassword
}

// StageNewPassword puts a proposed password on the entity for the rules to
// judge. It writes NOTHING that survives a refusal.
//
// The plaintext lands on the same runtime fields the insert path uses — no
// column, no payload, no audit event, no response — and the hash is derived by
// the rule below only once every check has passed. That ordering is the point: a
// refused change must leave the stored credential exactly as it was.
func (e *User) StageNewPassword(plaintext, confirmation string) {
	e.Password = vos.Password(plaintext)
	e.PasswordConfirmation = confirmation
}

// RegisterFailedCredentialAttempt records one failed attempt and locks the
// account when the threshold is reached.
//
// IT IS NOT A RULE, and it cannot be: it must persist on a request the domain
// REJECTED, and a rejected aggregate write persists nothing. The handler calls
// it on its failure branch and writes the row itself.
//
// The counter is reset rather than left at the threshold, so a lock that expires
// gives the account a fresh budget instead of locking again on the next mistake.
func (e *User) RegisterFailedCredentialAttempt(now time.Time) {
	e.FailedLoginAttempts++
	if e.FailedLoginAttempts >= credentialFailureThreshold {
		e.LockedUntil = now.Add(credentialLockWindow)
		e.FailedLoginAttempts = 0
	}
}

// IsLockedAt reports whether the account is locked at the given instant.
//
// The zero instant means "never locked", which is what a server-assigned column
// can express where NULL is not available. An expired lock is not a lock: the
// window passing is what releases it, with no sweeper and no second write.
func (e *User) IsLockedAt(now time.Time) bool {
	return !e.LockedUntil.IsZero() && e.LockedUntil.After(now)
}

const (
	// Five consecutive failures buy fifteen minutes. Both are calibrations
	// rather than invariants — one constant each, changeable without a
	// migration, because neither reaches the schema.
	credentialFailureThreshold = 5
	credentialLockWindow       = 15 * time.Minute
)

// credentialRules is the update-side branch the two operations run through, and
// nothing else does.
//
// It is called from customRules' IfUpdate gate, guarded by the action name, so
// an ordinary PATCH never reaches it — which matters in both directions: the
// password checks do not fire on a rename, and a rename's rules do not have to
// know a credential exists.
func (e *User) credentialRules(actionName string, service UserService, r *domain.Rules) {
	// THE VALUE OBJECT IS ASKED DIRECTLY, and the two lines this is NOT are the
	// whole reason for this comment.
	//
	// The Password field declares `modes: [insert]`, so the generated BuildRules
	// calls r.IgnoreValueObject("Password") in every update gate — correctly,
	// because a PATCH that renames somebody carries no password to judge. These
	// two operations DO carry one, and the automatic pass is not coming back for
	// it: without something here, the whole policy (length, the four classes,
	// whitespace) is simply not enforced on a password change, while every test
	// of the INSERT path stays green.
	//
	// The obvious repair — r.ValidateValueObject("Password", e.Password) — DOES
	// NOT WORK, and it fails silently. The framework collects the ignored and
	// the forced lists separately and combines them only at the end of the pass:
	//
	//     for _, entry := range forced {
	//         if ignoredSet[entry.name] || seen[entry.name] { continue }
	//
	// so the generated Ignore wins over a Force of the same name whatever order
	// they were registered in. It was written that way here first, and the
	// handler test caught it: a five-character password was accepted by both
	// credential operations.
	//
	// Calling IsValid directly is the escape that actually escapes. It IS the
	// framework's entry point — the same method the automatic pass invokes —
	// and it reports into the context itself, so nothing about the answer, the
	// field name or the seven catalogs differs from a value object validated any
	// other way.
	e.Password.IsValid("Password", r.Context())

	// ── the confirmation ──
	// The same rule the insert path gets declaratively, restated here because a
	// declarative rule is scoped to a verb and both operations share ModeUpdate
	// with the ordinary patch.
	if e.PasswordConfirmation != e.Password.Value() {
		r.AddNotification("PasswordConfirmation", PasswordConfirmationMismatchNotification{})
	}

	// ── the context rule ──
	// Shared with the insert path, method and all, so the two entry points
	// cannot drift apart about what "echoes the identity" means.
	e.refusePasswordEchoingIdentity(r)

	// ── the new password must differ from the current one ──
	// Asked LAST of the value checks, because it is the only one that costs a
	// full hash verification: there is no point spending ~100 ms of Argon2id on
	// a password that is already too short.
	//
	// It is not password-reuse prevention and must not be described as one: the
	// row carries exactly one hash, so this can only see the CURRENT password.
	// Real reuse prevention needs a history table this model does not have.
	if service != nil && service.PasswordIsUnchanged(e.Password.Value(), e.PasswordHash) {
		r.AddNotification("Password", PasswordUnchangedNotification{})
	}

	// The derivation runs only if nothing above rejected the write. Deriving
	// first and letting the framework discard the entity would work today and
	// would be one refactor away from writing a hash for a password the rules
	// refused.
	if r.Context() != nil && r.Context().HasErrors() {
		return
	}

	if service != nil {
		e.PasswordHash = service.HashPassword(e.Password.Value())
	}
	e.PasswordChangedAt = time.Now().UTC()

	// A successful credential change clears the lockout, whichever door it came
	// through: somebody who proved possession of the current password, or an
	// operator who reset it, has demonstrated exactly what the lock exists to
	// wait for.
	e.FailedLoginAttempts = 0
	e.LockedUntil = time.Time{}

	// The two operations differ on ONE field, and this is it. A caller who
	// chose the password themselves is done; a password somebody else set is a
	// temporary one, and the next sign-in has to replace it.
	e.MustChangePassword = actionName == ActionResetPassword
}
