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

package handlers

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
// because a utils.Refusal that comes back in one millisecond instead of a hundred
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
// IT IS THE ONE DELIBERATE EXCEPTION to the generic utils.Refusal above, and the
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

// InvalidClientCredentialsNotification reaches the caller as 401. The struct NAME
// is the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
//
// IT IS A SECOND TYPE RATHER THAN A REUSE OF THE ONE ABOVE, and the reason is the
// words. InvalidCredentialsNotification renders as "Invalid e-mail or password" in
// seven languages, and a machine has neither — an integration told to check its
// e-mail is being sent to look for something that does not exist. The two routes
// authenticate different kinds of subject with different credentials, so they name
// them differently.
//
// SPLITTING THEM BUILDS NO ORACLE, which is the question worth asking given how
// hard the type above works to be one answer to five questions. That argument is
// about telling apart the branches WITHIN one endpoint: which of "no such row",
// "wrong credential" and "account not usable" happened. Nothing here does that —
// this is still exactly one answer to every utils.Refusal the machine route can reach.
// The caller already chose the endpoint, so learning that /auth/client/token speaks
// about clients tells them what the path already said.
//
// The five branches it answers for, all identically:
//
//	the id matches no client row;
//	the id matches and the secret does not — including a retiring secret whose
//	  grace window has closed;
//	the client is suspended, or archived;
//	the owning tenant is archived, or commercially suspended;
//	the origin address is outside the client's allowed ranges.
//
// The last one is the one somebody will want to split out, and it must not be:
// telling the holder of a stolen secret that only the NETWORK was wrong confirms
// that the secret itself is good. The operator whose egress address changed gets
// their answer from the log stream, which carries the reason and the address.
//
// The FIELD NAME is the same neutral "credentials" the user route uses, so the
// envelope does not say which half was wrong either.
type InvalidClientCredentialsNotification struct {
	domain.ApplicationNotificationBase
}

func (InvalidClientCredentialsNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticUnauthorized
}
