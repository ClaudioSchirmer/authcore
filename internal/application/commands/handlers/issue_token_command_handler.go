// Hand-written, and not a hook: no generator declares this file.
//
// THE SIGN-IN — an e-mail and a password exchanged for a token pair. One
// hand-written handler, one file, per the layout standard; the ports it takes, the
// utils.Refusal it answers with and the claim set it mints are shared with the rotation
// and live in authentication_shared.go next door.
//
// WHAT THE FRAMEWORK OWNS AND WHAT WE OWN. The framework's authcore.Issuer owns
// everything security-critical about the TOKEN: asymmetric signing, the reserved
// claims, key rotation, and the refresh algorithm. It authenticates NOBODY — its
// own documentation says so — and mints for a subject the caller has already
// decided is authentic. Deciding that is this file's entire job.

package handlers

import (
	"strconv"
	"strings"
	"time"

	cmddtos "github.com/ClaudioSchirmer/authcore/internal/application/commands/dtos"
	"github.com/ClaudioSchirmer/authcore/internal/application/commands/handlers/dtos"

	"github.com/ClaudioSchirmer/authcore/internal/application/commands/handlers/utils"

	"github.com/ClaudioSchirmer/authcore/internal/application/commands"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/web/authcore"
)

// IssueTokenHandler turns an e-mail and a password into a token pair.
type IssueTokenHandler struct {
	Store    dtos.AuthenticationStore
	Attempts dtos.AttemptRecorder
	Issuer   dtos.TokenIssuer

	// Events carries the per-attempt record that the rollup table stopped
	// keeping. A nil publisher disables the announcements and changes nothing
	// else — the same semantic the framework gives its own event port, and what
	// lets a test drive the utils.Refusal branches without one.
	Events dtos.AuthenticationEventPublisher
}

// journal is this route's view of the shared path to the auxiliary tables and
// the log stream, bound to the subject kind it authenticates.
//
// Built per call rather than held as a field: it is two interface copies and a
// string, and building it here keeps the handler's wiring exactly what the web
// layer already passes — the composition root never learns a new name.
func (h *IssueTokenHandler) journal() utils.Journal {
	return utils.NewJournal(h.Attempts, h.Events, utils.IdentityKindUser)
}

// Handle authenticates and mints.
//
// THE ORDER OF THE CHECKS IS THE SECURITY PROPERTY, not a style choice:
//
//  1. no row for this address → burn a verification, then refuse. Without the
//     burn the response time says "nobody here has that address".
//  2. a row exists → verify the password FIRST, before looking at status. Every
//     found-row path then costs one real Argon2id verification, so a suspended
//     account and a wrong password are indistinguishable from outside.
//  3. only then, the account and its tenant have to be usable.
//
// Every utils.Refusal is the same notification, the same field name and the same 401.
func (h *IssueTokenHandler) Handle(ctx *configuration.AppContext, cmd *commands.IssueTokenCommand) (cmddtos.TokenResult, error) {
	// Lowercased and trimmed before ANYTHING else, and that ordering matters
	// twice: the domain stores addresses lowercase (vos.Email refuses anything
	// else), and the attempt log counts by this exact string — two spellings of
	// one address counting toward two windows would mean neither ever locks.
	email := strings.ToLower(strings.TrimSpace(cmd.Email))
	ip := ctx.ClientIP()

	// ── the lockout, BEFORE the credential is even looked at ──
	//
	// Asked first because the point of a lockout is to stop paying for guesses:
	// past the threshold this costs one indexed read instead of a lookup plus a
	// ~100 ms verification. It answers identically whether or not the identity
	// names an account, which is what keeps the 429 from being an oracle.
	journal := h.journal()

	until, locked, knownToExist, err := journal.LockedUntilFor(ctx, email)
	if err != nil {
		// NOT a utils.Refusal. A lockout probe that cannot run has not established
		// anything, and answering 401 would tell a caller with a correct password
		// that it was wrong. It escapes as an exception → 500.
		return cmddtos.TokenResult{}, err
	}
	if locked {
		// Counted, so a reviewer sees somebody kept trying through the lock — and
		// counted on a lifetime column that moves neither the live count nor the
		// window anchor, so the trying cannot extend the lock.
		//
		// knownToExist comes from the failures that CAUSED this lock — the probe
		// read that row anyway. Nothing here looked the account up, and nothing
		// should: past the threshold the point is to stop paying for guesses. It
		// rides the announcement because "which locked identities are real
		// accounts under attack" is the question that separates a targeted attack
		// from credential-stuffing noise.
		if rerr := journal.RefusedWhileLocked(ctx, email, ip, until, knownToExist); rerr != nil {
			return cmddtos.TokenResult{}, rerr
		}
		return cmddtos.TokenResult{}, refuseLocked(until)
	}

	account, err := h.Store.LoadAccountByEmail(ctx, email)
	if err != nil || account == nil {
		// THE ANSWER IS THE SAME, THE RECORD IS NOT — and holding those two apart
		// is what lets this branch be both safe and useful.
		//
		// The caller is refused identically whether the address is unknown or the
		// store Failed: distinguishing them would mean 500 for one and 401 for the
		// other, and a caller could then learn an address exists by finding an
		// input that changes the status code.
		//
		// The LOG, which no attacker reads, keeps the difference. A store that
		// answered (nil, nil) established the address is not here — false. One
		// that answered an error established nothing — nil. Writing false in that
		// second case would put a claim in the table that nobody verified, and
		// this table exists to be trusted a year from now.
		var existed *bool
		if err == nil {
			absent := false
			existed = &absent
		}
		h.Store.BurnPasswordVerification()
		// The same split the counter makes, made once more on the stream: an
		// address that is provably absent and a lookup that never answered are one
		// utils.Refusal to the caller and two different lines to whoever reads this
		// later.
		reason := "sign-in Failed: no account for this identity"
		if err != nil {
			reason = "sign-in Failed: identity lookup could not be performed"
		}
		if rerr := journal.Failed(ctx, email, ip, reason, existed); rerr != nil {
			return cmddtos.TokenResult{}, rerr
		}
		return cmddtos.TokenResult{}, utils.Refusal(InvalidCredentialsNotification{})
	}

	// From here the identity provably exists, and every remaining branch says so.
	existed := true

	if !h.Store.PasswordMatches(cmd.Password, account.PasswordHash) {
		if rerr := journal.Failed(ctx, email, ip, "sign-in Failed: credential rejected", &existed); rerr != nil {
			return cmddtos.TokenResult{}, rerr
		}
		return cmddtos.TokenResult{}, utils.Refusal(InvalidCredentialsNotification{})
	}

	if !utils.AccountIsUsable(account) {
		// A suspended account or a withdrawn tenant is a FAILURE in the log, not a
		// success: nobody got in. It counts toward the lockout like any other,
		// which is correct — repeatedly presenting a valid credential for a
		// disabled account is exactly the pattern worth rate-limiting.
		// The one branch where the credential was RIGHT and the answer is still a
		// utils.Refusal. Indistinguishable to the caller by design; on the stream it is
		// the line that explains a support ticket in one read.
		if rerr := journal.Failed(ctx, email, ip,
			"sign-in Failed: credential valid but account or tenant not usable", &existed); rerr != nil {
			return cmddtos.TokenResult{}, rerr
		}
		return cmddtos.TokenResult{}, utils.Refusal(InvalidCredentialsNotification{})
	}

	// STEP TWO. Everything a token says, in one concurrent burst.
	//
	// NOT a credential utils.Refusal, and NOT an attempt worth counting: the caller
	// proved who they are and this service Failed them. Recording a failure here
	// would let a database problem lock out the very users it is already failing.
	// It escapes as an exception → 500.
	bundle, err := h.Store.ResolveSignIn(ctx, account)
	if err != nil {
		return cmddtos.TokenResult{}, err
	}

	// ONE resolution, two readers. The token below and the profile at the
	// bottom of this function both receive this exact map.
	customClaims := utils.ResolveCustomClaims(account, bundle)

	access, refresh, err := h.Issuer.IssueWithRefresh(ctx, authcore.TokenRequest{
		Subject: account.ID.Value(),
		Claims:  utils.BuildClaims(account, bundle, customClaims),
	})
	if err != nil {
		return cmddtos.TokenResult{}, err
	}

	// Recorded LAST, once the sign-in has actually Succeeded. It is what anchors
	// the window — every failure before this moment stops counting — so writing it
	// any earlier would clear a counter for a sign-in that had not happened yet.
	if rerr := journal.Succeeded(ctx, email, ip); rerr != nil {
		return cmddtos.TokenResult{}, rerr
	}

	return cmddtos.TokenResult{
		AccessToken:      access.Token,
		TokenType:        utils.TokenTypeBearer,
		ExpiresAt:        access.ExpiresAt.Unix(),
		RefreshToken:     refresh.Value,
		RefreshExpiresAt: refresh.ExpiresAt.Unix(),
		User:             utils.BuildProfile(account, bundle, customClaims),
	}, nil
}

// refuseLocked is the ONE utils.Refusal that is not the generic 401.
//
// It carries the remaining window so the caller learns that WAITING is the fix —
// the whole reason the maintainer asked for a distinct answer here. It discloses
// nothing, because the lockout is keyed by the attempted identity: an address
// that names no account reaches this same message on its sixth attempt.
//
// The window is rounded UP to the next whole minute. Rounding down would tell
// somebody to come back at a moment that is still inside the lock, and "0 minutes"
// is not an instruction.
func refuseLocked(until time.Time) error {
	remaining := time.Until(until)
	minutes := int(remaining / time.Minute)
	if remaining%time.Minute > 0 {
		minutes++
	}
	if minutes < 1 {
		minutes = 1
	}
	return utils.Refusal(AccountTemporarilyLockedNotification{Minutes: strconv.Itoa(minutes)})
}
