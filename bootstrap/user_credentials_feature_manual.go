// Hand-written, and not a hook: no generator declares this file.
//
// It exists because the credential route cannot live in the generated
// UsersFeature — that file is owned and rewritten, and mounting anything by hand
// inside it would be an edit the next regeneration refuses.
//
// It no longer has an ordering constraint. It used to mount a PUBLIC
// `PATCH /users/password` beside the generated `PATCH /users/:id`, which are the
// same shape to a router, so registration order decided which one answered. That
// route was removed on 2026-08-26 and the hazard went with it; both remaining
// routes carry a distinct suffix under `/users/:id` and collide with nothing.

package main

import (
	appinfra "github.com/ClaudioSchirmer/authcore/internal/infra"
	appweb "github.com/ClaudioSchirmer/authcore/internal/web"
	"github.com/ClaudioSchirmer/omnicore/bootstrap"
	fwgraphql "github.com/ClaudioSchirmer/omnicore/web/graphql"
	"github.com/gofiber/fiber/v3"
)

// UserCredentialsFeature mounts the two credential routes: the self-service
// change and the helpdesk reset.
//
// It builds its OWN repository and service rather than reaching into
// UsersFeature: a feature that borrowed another feature's fields would couple
// two things the framework deliberately keeps independent, and the repositories
// are cheap — they share the engine, which is what actually costs anything.
type UserCredentialsFeature struct {
	repo *appinfra.UserRepository
	svc  *appinfra.UserServiceImpl
}

func NewUserCredentialsFeature(d bootstrap.Deps) *UserCredentialsFeature {
	repo := appinfra.NewUserRepository(d.DB)
	return &UserCredentialsFeature{repo: repo, svc: appinfra.NewUserServiceImpl(repo)}
}

// Mount delegates to the web layer, like every other feature here.
//
// It contributes NO read model: both routes write and answer 204. A credential
// operation that returned the user's row would turn a password reset into a
// profile read.
func (f *UserCredentialsFeature) Mount(app *fiber.App, d bootstrap.Deps) {
	appweb.MountUserCredentials(app, f.repo, f.svc, d)
}

// MountGraphQL opts this feature into the GraphQL surface, the same way
// UsersFeature does.
//
// The framework discovers the method by type assertion and builds the single
// shared registry itself, so the two credential fields land on the same schema
// as the nine generated user fields — nothing about GraphQL is wired in the
// composition root.
func (f *UserCredentialsFeature) MountGraphQL(reg *fwgraphql.Registry, _ bootstrap.Deps) {
	appweb.MountUserCredentialsGraphQL(reg, f.repo, f.svc)
}
