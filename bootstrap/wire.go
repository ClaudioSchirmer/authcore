package main

import (
	apptrans "github.com/ClaudioSchirmer/authcore/internal/application/translations"
	"github.com/ClaudioSchirmer/omnicore/application/translation"
	"github.com/ClaudioSchirmer/omnicore/bootstrap"
	"github.com/ClaudioSchirmer/omnicore/web/openapi"
)

// Wire assembles the service. It is currently an empty shell: no features and no
// translations yet, which the framework accepts under APP_PROFILE=dev with a loud
// warning — the legitimate state of a freshly scaffolded service.
//
// The first aggregate is added with /omnicore:scaffold-entity, which appends its
// feature to Features and its seven catalogs to Translations. Translations become
// mandatory as soon as the first feature exists.
func Wire(d bootstrap.Deps) bootstrap.Wiring {
	// Built here only because Wiring is what the framework reads it through; the
	// feature OWNS it, exposes it, and is where anyone looking for it should end
	// up. See AuthenticationFeature.RefreshTokenStore.
	authentication := NewAuthenticationFeature(d)

	return bootstrap.Wiring{
		// The storage half of the framework's refresh-token algorithm. The Issuer
		// owns rotation and reuse detection and never sees a raw secret — only a
		// SHA-256 hash crosses this seam. Left nil with
		// auth.issuer.refreshTokenTtlSeconds > 0, the boot aborts rather than
		// starting an issuer that would fail at the first sign-in.
		RefreshTokenStore: authentication.RefreshTokenStore(),

		// The seven catalogs. The framework requires them as soon as a
		// feature exists, so they arrive with the first entity.
		Translations: []translation.Module{
			apptrans.PTBR(), apptrans.ENG(), apptrans.ESP(), apptrans.FRA(),
			apptrans.DEU(), apptrans.ITA(), apptrans.NLD(),
		},

		Features: []bootstrap.Feature{
			NewTenantsFeature(d),
			NewPermissionsFeature(d),
			NewRolesFeature(d),
			NewGroupsFeature(d),
			NewUserCredentialsFeature(d),
			NewUsersFeature(d),
			authentication,
		},

		OpenAPI: &openapi.Config{
			Title:       "authcore API",
			Version:     "0.1.0",
			Description: "Identity and authentication for a multi-tenant platform.",
			// The dropdown content is filled by bootstrap from Wiring.Translations.
			// While that slice is empty no selector is rendered; it starts showing
			// up the moment the first entity registers its catalogs.
			LanguageSelector: true,
		},
	}
}
