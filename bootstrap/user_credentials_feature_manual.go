// Hand-written, and not a hook: no generator declares this file.
//
// It exists because the credential route cannot live in the generated
// UsersFeature — that file is owned and rewritten, and mounting anything by hand
// inside it would be an edit the next regeneration refuses.
//
// It no longer has an ordering constraint. It used to mount a PUBLIC
// `PATCH /users/password` beside the generated `PATCH /users/:id`, which are the
// same shape to a router, so registration order decided which one answered. That
// route was removed on 2026-08-26 and the hazard went with it; the remaining
// route carries a distinct suffix and collides with nothing.

package main

import (
	appinfra "github.com/ClaudioSchirmer/authcore/internal/infra"
	appweb "github.com/ClaudioSchirmer/authcore/internal/web"
	"github.com/ClaudioSchirmer/omnicore/bootstrap"
	"github.com/gofiber/fiber/v3"
)

// UserCredentialsFeature mounts the change-password and reset-password routes.
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
// It contributes NO read model: the route writes and answers 204. A credential
// operation that returned the user's row would turn a password reset into a
// profile read.
func (f *UserCredentialsFeature) Mount(app *fiber.App, d bootstrap.Deps) {
	appweb.MountUserCredentials(app, f.repo, f.svc, d)
}
