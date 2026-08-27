// Hand-written, and not a hook: no generator declares this file.
//
// The two refusals the token endpoints raise — and they are declared HERE, not in
// internal/domain/notifications.go, because THESE ENDPOINTS DO NOT TOUCH THE
// DOMAIN. A sign-in loads a user, compares a hash and mints a token: no
// GetUpdatable, no BuildRules, no aggregate write anywhere in the path. Nothing in
// the domain raises either of these and nothing in the domain could.
//
// The domain's own notifications file states the rule these follow: a notification
// lives beside the type that raises it. The credential routes' notifications are
// in the domain because the AGGREGATE raises them from inside its rules; these are
// in the application because a HANDLER raises them, and putting them next to the
// aggregate would have been filing them by resemblance rather than by origin.
//
// So they embed ApplicationNotificationBase, which is what the framework's own
// auth failures use, and they travel in an *exception.ApplicationError rather than
// a *domain.DomainError. The pipeline handles either through domain.NotificationCarrier
// without knowing which — but the one that is chosen says which layer decided, and
// that is worth keeping honest.

package commands

import (
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// InvalidCredentialsNotification reaches the caller as 401. The struct NAME is
// the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
//
// IT IS DELIBERATELY THE ONLY ANSWER TO FIVE DIFFERENT QUESTIONS, and that is the
// whole design rather than a shortcut. The sign-in refuses with this one
// notification when:
//
//	the e-mail matches no row;
//	the e-mail matches and the password does not;
//	the account is suspended, or archived;
//	the owning tenant is archived, or commercially suspended;
//	a refresh token is replayed after it was already redeemed.
//
// Splitting any of them apart builds an oracle. "No such user" tells an attacker
// which addresses have accounts here; "wrong password" tells them the address is
// worth attacking; "tenant suspended" discloses a competitor's commercial state to
// anyone who can guess an address. One message answers all five, and the FIELD
// NAME is a neutral "credentials" — never "email" and never "password" — so the
// envelope itself does not say which half was wrong.
//
// The message is generic, but the ENFORCEMENT is not sloppy: the handler pays a
// full Argon2id verification on the paths where there is no stored hash to check,
// because a refusal that comes back in one millisecond instead of a hundred
// answers the same question the shared message refuses to answer.
type InvalidCredentialsNotification struct {
	domain.ApplicationNotificationBase
}

func (InvalidCredentialsNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticUnauthorized
}

// AccountTemporarilyLockedNotification reaches the caller as 429. The struct NAME
// is the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated. It interpolates minutes into the
// message.
//
// IT IS THE ONE DELIBERATE EXCEPTION to the generic refusal above, and the
// maintainer took the decision knowingly: a user refused with the same message
// they get for a typo has no way to learn that WAITING is the fix, and every one
// of them becomes a support ticket.
//
// IT DISCLOSES NOTHING, and that is not luck — it is why the lockout counter does
// not live on the `users` row. The counter is keyed by the ATTEMPTED IDENTITY, so
// an address that names no account locks on the sixth attempt exactly as a real
// one does and receives exactly this message. Had the counter lived on the user
// row it could only ever have existed for real accounts, and this message would
// have been an existence oracle — which is why that design was abandoned rather
// than reworded.
//
// 429 and not 423 (Locked): 423 says the RESOURCE is locked, and there may be no
// resource here at all. 429 says "you have made too many requests", which is true
// regardless of whether the identity names anything, and it is the status every
// client library already knows how to back off from.
type AccountTemporarilyLockedNotification struct {
	domain.ApplicationNotificationBase
	Minutes string `tvar:"minutes"`
}

func (AccountTemporarilyLockedNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticTooManyRequests
}
