// Hand-written, and not a hook: no generator declares this file.
//
// It exists because the two credential routes cannot live in the generated
// UsersFeature — that file is owned and rewritten, and mounting anything by hand
// inside it would be an edit the next regeneration refuses.
//
// ⚠️ IT MUST BE REGISTERED BEFORE UsersFeature in wire.go. `PATCH
// /users/password` and `PATCH /users/:id` are the same shape to a router, so
// whichever is registered first wins — and the one that has to win is the public
// one. The by-id route also parses its segment as a UUID, so an inverted order
// cannot silently swallow the change; it would 404 loudly instead. Two locks,
// because the order is the kind of thing a refactor moves without noticing.

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
	repo   *appinfra.UserRepository
	svc    *appinfra.UserServiceImpl
	hasher *appinfra.Argon2idHasher
}

func NewUserCredentialsFeature(d bootstrap.Deps) *UserCredentialsFeature {
	repo := appinfra.NewUserRepository(d.DB)
	return &UserCredentialsFeature{
		repo:   repo,
		svc:    appinfra.NewUserServiceImpl(repo),
		hasher: appinfra.NewArgon2idHasher(),
	}
}

// Mount delegates to the web layer, like every other feature here.
//
// It contributes NO read model: these two routes write and answer 204. A
// credential operation that returned the user's row would turn a password change
// into a profile read — and the public one would do it without a token.
func (f *UserCredentialsFeature) Mount(app *fiber.App, d bootstrap.Deps) {
	appweb.MountUserCredentials(app, f.repo, f.svc, f.hasher, d)
}
