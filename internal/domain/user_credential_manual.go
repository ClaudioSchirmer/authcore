// Hand-written, and not a hook: no generator declares this file.
//
// It is the domain half of the two credential operations. The spec language has
// no way to declare either — `authz.permissions` takes a closed set of verbs and
// neither "change the password" nor "reset it" is one of them — so the commands,
// the handlers and the routes are all by hand. What is NOT by hand is the
// judgement: the rules below live in the aggregate like every other rule,
// dispatched by the framework, reporting through the same notification context
// and answering in the same seven languages.
//
// THE ACTION NAME IS WHAT SEPARATES THEM FROM A PATCH, AND FROM EACH OTHER. All
// three dispatch ModeUpdate — the ordinary PATCH included — so a rule that keyed
// on the mode alone would run the password checks on a rename, and would have no
// way to tell the change from the reset. BuildRules receives the actionName the
// caller passed to domain.GetUpdatable, and that is the discriminator.
//
// THE TWO OPERATIONS ARE DISJOINT BY CONSTRUCTION: the change refuses any row
// but the caller's, the reset refuses the caller's own. Same id is always the
// change, a different id is always the reset. That is not symmetry for its own
// sake — see PasswordResetRequiresAnotherUserNotification for what the missing
// half would have cost.

package domain

import (
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// The action names the two operations dispatch under.
//
// They exist as constants because the ordinary PATCH and both of these dispatch
// ModeUpdate, so the action name is the only thing that tells the aggregate
// which of the three it is judging.
const (
	// ActionChangePassword is the self-service change: the caller proves the
	// password they currently hold and chooses the next one.
	ActionChangePassword = "ChangePassword"
	// ActionResetPassword is the helpdesk reset: somebody else sets the
	// password, proving nothing, and the next sign-in has to rotate it.
	ActionResetPassword = "ResetPassword"
)

// StageNewPassword puts a proposed password on the entity for the rules to
// judge. It writes NOTHING that survives a refusal.
//
// The plaintext lands on the same runtime fields the insert path uses — no
// column, no payload, no audit event, no response — and the hash is derived by
// the rules below only once every check has passed. That ordering is the point:
// a refused write must leave the stored credential exactly as it was.
func (e *User) StageNewPassword(plaintext, confirmation string) {
	e.Password = vos.Password(plaintext)
	e.PasswordConfirmation = confirmation
}

// StagePasswordChange is StageNewPassword plus the password the caller claims to
// hold today, which only the change carries.
//
// CurrentPassword is declared `source: manual` in the spec, which means the
// generator emits the field and fills it from nowhere: no write DTO, command or
// OpenAPI schema mentions it, so it cannot leak into the body of the ordinary
// PATCH. Putting the value there is this method's whole job.
func (e *User) StagePasswordChange(current, plaintext, confirmation string) {
	e.CurrentPassword = current
	e.StageNewPassword(plaintext, confirmation)
}

// rowIsTheCaller reports whether this row is the caller's own.
//
// It compares against RequestingUserID, which the spec declares `source:
// subject` — read off Identity.Subject, the canonical accessor, and NOT off
// Claims["sub"]. The distinction is load-bearing rather than stylistic: the
// framework builds identities that set Subject and carry no `sub` claim at all
// (web/grpc/posture.go, infra/integration/registry.go), and a comparison
// against the raw map would read "" for those callers and answer "not the
// owner" to everyone.
//
// It does NOT ask whether an identity was present — both callers below do that
// themselves, because the two want OPPOSITE answers when there is none.
func (e *User) rowIsTheCaller() bool {
	rowID := ""
	if id := e.GetID(); id != nil {
		rowID = id.Value()
	}
	return e.RequestingUserID != "" && e.RequestingUserID == rowID
}

// changePasswordRules is the branch the self-service change runs through.
//
// Called from customRules' IfUpdate gate, guarded by the action name, so an
// ordinary PATCH never reaches it — which matters in both directions: the
// password checks do not fire on a rename, and a rename's rules do not have to
// know a credential exists.
func (e *User) changePasswordRules(service UserService, r *domain.Rules) {
	// ── the row decision ──
	// The permission on the route says WHO may attempt the verb; this says
	// WHOSE row they reached. A caller pointing this endpoint at somebody else
	// meets a 403 — the reset is the operation for that, and it asks for a
	// different permission.
	//
	// With NO IDENTITY AT ALL the check stands down, the same posture
	// refuseForeignTenant takes: that is provably a development bench, since
	// the middleware is only bypassable with auth.mode disabled and the
	// framework refuses that mode outside APP_PROFILE=dev.
	if e.RequestingIdentityPresent && !e.rowIsTheCaller() {
		r.AddNotificationNamed("ID", PasswordChangeRequiresSelfNotification{})
		return
	}

	e.credentialValueRules(r)

	// The proof of possession is asked LAST, because it is the only check here
	// that costs a full Argon2id verification: there is no point spending
	// ~100 ms on a password the policy above already refused.
	if r.Context() != nil && r.Context().HasErrors() {
		return
	}

	// An EMPTY current password is answered without hashing anything — same
	// refusal, ~100 ms cheaper, and it says no more than the wrong-password
	// case does.
	if e.CurrentPassword == "" ||
		(service != nil && !service.PasswordIsUnchanged(e.CurrentPassword, e.PasswordHash)) {
		r.AddNotification(&e.CurrentPassword, InvalidCurrentPasswordNotification{}, false)
		return
	}

	// ── the new password must differ from the current one ──
	// Answered from the two PLAINTEXTS, for free. The line above already
	// established that CurrentPassword verifies against the stored hash, so a
	// second Argon2id call — which is what the reset has to make — would be
	// asking a question this branch can already answer.
	//
	// It is not password-reuse prevention and must not be described as one: the
	// row carries exactly one hash, so this can only see the CURRENT password.
	// Real reuse prevention needs a history table this model does not have.
	if e.Password.Value() == e.CurrentPassword {
		r.AddNotification(&e.Password, PasswordUnchangedNotification{}, false)
		return
	}

	// The caller chose this password and proved the one before it, so there is
	// nothing for the next sign-in to rotate. This is the operation that CLEARS
	// the flag; the insert and the reset are the two that set it.
	e.applyNewCredential(service, false)
}

// resetPasswordRules is the branch the helpdesk reset runs through.
func (e *User) resetPasswordRules(service UserService, r *domain.Rules) {
	// ── the row decision ──
	// The mirror of the change's, and it is what keeps a stolen token from
	// laundering itself: without it a holder of user:reset-password could point
	// this endpoint at their OWN row and replace the credential without proving
	// the previous one, defeating the change endpoint's currentPassword by
	// choosing the other URL.
	//
	// Stands down with no identity, for the reason given above.
	if e.RequestingIdentityPresent && e.rowIsTheCaller() {
		r.AddNotificationNamed("ID", PasswordResetRequiresAnotherUserNotification{})
		return
	}

	e.credentialValueRules(r)

	if r.Context() != nil && r.Context().HasErrors() {
		return
	}

	// The same question the change answers from two plaintexts, and here it
	// costs a full verification: this operation never learns the current
	// password, so the stored hash is the only thing to ask.
	if service != nil && service.PasswordIsUnchanged(e.Password.Value(), e.PasswordHash) {
		r.AddNotification(&e.Password, PasswordUnchangedNotification{}, false)
		return
	}

	// The password a reset leaves is somebody else's choice, so the next
	// sign-in has to replace it.
	e.applyNewCredential(service, true)
}

// credentialValueRules is what the two operations judge identically: the shape
// of the password being proposed, and nothing about who is proposing it.
//
// One method rather than two copies, so the change and the reset cannot drift
// apart about what a valid password is.
func (e *User) credentialValueRules(r *domain.Rules) {
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
	// declarative rule is scoped to a verb and these operations share ModeUpdate
	// with the ordinary patch.
	if e.PasswordConfirmation != e.Password.Value() {
		r.AddNotification(&e.PasswordConfirmation, PasswordConfirmationMismatchNotification{}, false)
	}

	// ── the context rule ──
	// Shared with the insert path, method and all, so the three entry points
	// cannot drift apart about what "echoes the identity" means.
	e.refusePasswordEchoingIdentity(r)
}

// applyNewCredential is the write half, reached only once every check has
// passed.
//
// Deriving earlier and letting the framework discard the entity would work
// today and would be one refactor away from writing a hash for a password the
// rules refused.
//
// mustChange is the ONE thing the two operations disagree about: a password the
// caller chose for themselves needs no rotation, and one somebody else chose
// for them does.
func (e *User) applyNewCredential(service UserService, mustChange bool) {
	if service != nil {
		e.PasswordHash = service.HashPassword(e.Password.Value())
	}
	e.PasswordChangedAt = time.Now().UTC()
	e.MustChangePassword = mustChange
}
