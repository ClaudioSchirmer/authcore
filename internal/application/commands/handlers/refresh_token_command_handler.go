// Hand-written, and not a hook: no generator declares this file.
//
// The ROTATION — an unused refresh token exchanged for a fresh pair. It is one
// file of its own beside the sign-in because the layout standard puts one
// hand-written handler per file; what the two share — the ports, the utils.Refusal, the
// claim set — lives in authentication_shared.go next door.

package handlers

import (
	"errors"
	cmdutils "github.com/ClaudioSchirmer/authcore/internal/application/commands/utils"
	"strings"

	"github.com/ClaudioSchirmer/authcore/internal/application/commands/handlers/utils"

	"github.com/ClaudioSchirmer/authcore/internal/application/commands"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/domain"
	"github.com/ClaudioSchirmer/omnicore/web/authcore"
)

// RefreshTokenHandler rotates a refresh token into a new pair.
type RefreshTokenHandler struct {
	Store  utils.AuthenticationStore
	Lookup utils.RefreshTokenLookup
	Issuer utils.TokenIssuer
}

// Handle redeems and re-mints.
//
// THE CLAIMS ARE REBUILT FROM THE DATABASE, never replayed from the old token.
// RedeemRefreshToken takes them fresh from the caller at redemption time
// precisely so a permission revoked between logins reaches the mesh within
// minutes; handing back what the previous token carried would defeat the one
// mechanism the framework offers for that. It also means a must-change-password
// session cannot launder itself into a full one by refreshing — utils.BuildClaims
// applies the same restriction on this path as on the sign-in.
//
// THE SUBJECT IS LOOKED UP FIRST, and that ordering is forced by the framework
// rather than chosen: RedeemRefreshToken is handed the claim map by value and
// only then looks up the record, so this handler has to know whose claims to
// build before the framework would tell it. The lookup decides nothing — revoked,
// used and expired are all still the Issuer's call at the redemption below, which
// is what owns reuse detection and family revocation.
func (h *RefreshTokenHandler) Handle(ctx *configuration.AppContext, cmd *commands.RefreshTokenCommand) (cmdutils.TokenResult, error) {
	value := strings.TrimSpace(cmd.RefreshToken)
	if value == "" {
		// Answered without touching the store: an empty string redeems nothing,
		// and there is no timing signal to equalise here — this utils.Refusal does not
		// depend on whether any account exists.
		return cmdutils.TokenResult{}, utils.Refusal(InvalidCredentialsNotification{})
	}

	subject, err := h.Lookup.SubjectForRefreshToken(ctx, value)
	if err != nil {
		// A store failure is NOT a credential utils.Refusal: answering 401 would tell a
		// caller holding a perfectly good token that it was rejected, and would
		// bury an outage inside a login problem. It escapes as an exception → 500.
		return cmdutils.TokenResult{}, err
	}
	if subject == "" {
		// No record. Resolving grants before this point would have handed anyone
		// posting random strings a free database walk.
		return cmdutils.TokenResult{}, utils.Refusal(InvalidCredentialsNotification{})
	}

	account, err := h.Store.LoadAccountByID(ctx, domain.NewID(subject))
	if err != nil || account == nil || !utils.AccountIsUsable(account) {
		// The record exists but its subject no longer resolves to a usable
		// account — archived, suspended, or its tenant withdrawn since the last
		// rotation. The session is over, and it ends with the same utils.Refusal as
		// every other.
		return cmdutils.TokenResult{}, utils.Refusal(InvalidCredentialsNotification{})
	}

	// RE-READ ON EVERY ROTATION, never replayed from the token being redeemed —
	// which is the reason the framework takes claims fresh at redemption time. A
	// permission revoked or a claim value corrected while a session is live
	// therefore reaches the mesh at the next rotation rather than at the next full
	// sign-in, with no invalidation step anywhere.
	bundle, err := h.Store.ResolveSignIn(ctx, account)
	if err != nil {
		return cmdutils.TokenResult{}, err
	}
	customClaims := utils.ResolveCustomClaims(account, bundle)

	access, refresh, err := h.Issuer.RedeemRefreshToken(ctx, value, utils.BuildClaims(account, bundle, customClaims))
	switch {
	case err == nil:
	case errors.Is(err, authcore.ErrRefreshTokenNotFound),
		errors.Is(err, authcore.ErrRefreshTokenExpired),
		errors.Is(err, authcore.ErrRefreshTokenReused):
		// ALL THREE ANSWER THE SAME 401. Reuse in particular must not be
		// distinguishable: telling the holder of a stolen token that it had
		// already been redeemed confirms both that the token was real and that its
		// owner is active. The family has already been revoked by the Issuer
		// before this returns, and the WARN line the store logs is where an
		// operator sees it.
		return cmdutils.TokenResult{}, utils.Refusal(InvalidCredentialsNotification{})
	default:
		return cmdutils.TokenResult{}, err
	}

	return cmdutils.TokenResult{
		AccessToken:      access.Token,
		TokenType:        utils.TokenTypeBearer,
		ExpiresAt:        access.ExpiresAt.Unix(),
		RefreshToken:     refresh.Value,
		RefreshExpiresAt: refresh.ExpiresAt.Unix(),
		User:             utils.BuildProfile(account, bundle, customClaims),
	}, nil
}
