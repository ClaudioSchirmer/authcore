// Hand-written, and not a hook: no generator declares this file.
//
// It is the application half of the secret rotation. The spec language has no
// way to declare it — `authz.permissions` takes a closed set of verbs and
// "rotate a credential" is not one of them — so the command, the handler and the
// route are all by hand. What is NOT by hand is the judgement: the rules live in
// the aggregate like every other rule, dispatched by the framework, reporting
// through the same notification context and answering in the same seven
// languages.
//
// THE ACTION NAME IS WHAT SEPARATES IT FROM A PATCH. Both dispatch ModeUpdate,
// so a rule that keyed on the mode alone would rotate a live credential every
// time somebody fixed a typo in a description. BuildRules receives the
// actionName passed to domain.GetUpdatable, and that is the discriminator.
//
// IT IS THE ONE OPERATION IN THIS ENTITY THAT ANSWERS WITH A SECRET, and the
// only one that ever will: the plaintext exists for the length of this request
// and is never stored, so this response is the only place it can be read. That
// is also why the result below is a type of its own rather than the client's
// row — an operation that answered with the whole aggregate would be a profile
// read wearing a credential operation's clothes.

package handlers

import (
	"github.com/ClaudioSchirmer/authcore/internal/application/commands"
	"github.com/ClaudioSchirmer/authcore/internal/application/commands/handlers/dtos"

	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/application/persistence"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// RotateClientSecretHandler mints a new secret and retires the current one.
type RotateClientSecretHandler struct {
	Store   dtos.ClientSecretStore
	Service domain.Service
}

// Handle applies the rotation.
//
// THE ROW DECISIONS ARE NOT HERE. That the row belongs to the caller's tenant,
// and that a client-subject caller may only reach its own row, both live in the
// aggregate beside every other rule — they are authorization over a row rather
// than admission to a route, and the permission on the route already answered
// who may attempt the verb. Putting either here would also put it out of reach
// of the notification context, so the caller would get a bare status instead of
// a translated answer.
//
// The entity pointer is captured from the mutator and read back AFTER the write
// returns. That ordering is the point: the values below are what the rules
// produced and the writer accepted, so a utils.Refusal cannot leave a caller holding a
// secret the database never stored.
func (h *RotateClientSecretHandler) Handle(ctx *configuration.AppContext, cmd *commands.RotateClientSecretCommand) (commands.RotateClientSecretResult, error) {
	client, err := h.Store.ScopedReader(ctx).FindByID(domain.NewID(cmd.PathID()))
	if err != nil {
		return commands.RotateClientSecretResult{}, err
	}

	var rotated *appdomain.Client
	updatable, err := domain.GetUpdatable(client, func(e *appdomain.Client) error {
		// Absent means the default. Resolving it HERE and not in the rule keeps
		// the aggregate free of a pointer it would have to nil-check, and keeps
		// "what a missing field means" where the wire contract is.
		e.GracePeriodSeconds = appdomain.ClientSecretGraceDefaultSeconds
		if cmd.GracePeriodSeconds != nil {
			e.GracePeriodSeconds = *cmd.GracePeriodSeconds
		}
		feedClientRowScope(ctx, e)
		rotated = e
		return nil
	}, persistence.ScopeService(h.Service, ctx), appdomain.ActionRotateSecret)
	if err != nil {
		return commands.RotateClientSecretResult{}, err
	}

	if err := h.Store.Scope(ctx).Update(updatable); err != nil {
		return commands.RotateClientSecretResult{}, err
	}

	return commands.RotateClientSecretResult{
		ID:                      *rotated.GetID(),
		Secret:                  rotated.Secret,
		SecretChangedAt:         rotated.SecretChangedAt,
		PreviousSecretExpiresAt: rotated.PreviousSecretExpiresAt,
	}, nil
}

// feedClientRowScope fills the identity-derived fields the aggregate's row rules
// read. It is the same handful of lines every generated command mapper writes,
// and it is here for the same reason: a hand-written command that skips them
// leaves those guards standing down, because each asks whether an identity was
// PRESENT before it compares anything.
//
// The consequence of forgetting it is not theoretical and is invisible in a
// green build: a holder of client:rotate-secret in tenant A could mint a fresh
// credential for an integration in tenant B. The permission on the route says
// WHO may attempt the verb; it says nothing about WHOSE row.
func feedClientRowScope(ctx *configuration.AppContext, e *appdomain.Client) {
	id := ctx.Identity()
	if id == nil {
		return
	}
	e.RequestingIdentityPresent = true
	e.RequestingTenant = id.TenantID()
	// Identity.Subject and not Claims["sub"] — the canonical accessor, which is
	// what the generated mappers use for this field and what the aggregate's
	// comparison is written against. The framework builds identities that set
	// Subject and carry no sub claim at all.
	e.RequestingClientID = id.Subject
	// A super-admin crosses the scope. Not asked through HasPermission, which
	// panics on the *:* the claim carries — the wildcard has its own question.
	e.RequestingMayCrossScope = id.IsSuperAdmin()
	// Which KIND of subject is asking. Nothing mints this claim yet, so it reads
	// "" today and the rule consulting it stands down — see identity_kind.go.
	if kind, ok := id.Claims[clientIdentityKindClaim].(string); ok {
		e.RequestingIdentityKind = kind
	}
}

// The claim the subject kind travels in. It is spelled here rather than imported
// from the framework because the framework holds no opinion about custom
// claims — it is this platform's vocabulary, and identity_kind.go says why it is
// this word.
const clientIdentityKindClaim = "identity_kind"
