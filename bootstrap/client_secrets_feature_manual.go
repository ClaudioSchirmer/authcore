// Hand-written, and not a hook: no generator declares this file.
//
// It exists because the rotation route cannot live in the generated
// ClientsFeature — that file is owned and rewritten, and mounting anything by
// hand inside it would be an edit the next regeneration refuses.
//
// It carries no ordering constraint: `POST /clients/:id/secret` collides with
// nothing the generated feature mounts.

package main

import (
	appinfra "github.com/ClaudioSchirmer/authcore/internal/infra"
	appweb "github.com/ClaudioSchirmer/authcore/internal/web"
	"github.com/ClaudioSchirmer/omnicore/bootstrap"
	fwgraphql "github.com/ClaudioSchirmer/omnicore/web/graphql"
	"github.com/gofiber/fiber/v3"
)

// ClientSecretsFeature mounts the secret rotation.
//
// It builds its OWN repository and service rather than reaching into
// ClientsFeature: a feature that borrowed another feature's fields would couple
// two things the framework deliberately keeps independent, and the repositories
// are cheap — they share the engine, which is what actually costs anything. The
// same shape UserCredentialsFeature already has.
type ClientSecretsFeature struct {
	repo *appinfra.ClientRepository
	svc  *appinfra.ClientServiceImpl
}

func NewClientSecretsFeature(d bootstrap.Deps) *ClientSecretsFeature {
	repo := appinfra.NewClientRepository(d.DB)
	return &ClientSecretsFeature{repo: repo, svc: appinfra.NewClientServiceImpl(repo)}
}

// Mount delegates to the web layer, like every other feature here.
//
// It contributes NO read model: the route writes, and the credential it answers
// with is not a projection of anything — no column holds it.
func (f *ClientSecretsFeature) Mount(app *fiber.App, d bootstrap.Deps) {
	appweb.MountClientSecrets(app, f.repo, f.svc, d)
}

// MountGraphQL opts this feature into the GraphQL surface, the same way
// ClientsFeature does.
//
// The framework discovers the method by type assertion, so the rotation lands on
// the single shared schema beside the nine generated client fields — nothing
// about GraphQL is wired in the composition root.
func (f *ClientSecretsFeature) MountGraphQL(reg *fwgraphql.Registry, _ bootstrap.Deps) {
	appweb.MountClientSecretsGraphQL(reg, f.repo, f.svc)
}
