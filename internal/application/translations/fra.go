// FRA is the FRA translation catalog.
//
// This file is a registration site: omnicore-gen inserts the keys an entity
// needs and maintains the text IT wrote, so a spec whose message changed is
// followed here too. Improving a generated wording is still safe: the
// generator records a hash of what it wrote, reads yours as different, and
// leaves it — naming it in the report rather than reverting it.

package translations

import (
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/application/translation"
)

type fra struct{}

func FRA() translation.Module { return fra{} }

func (fra) Language() configuration.Language { return configuration.LangFR }

func (fra) Translations() map[string]string {
	return map[string]string{
		"TenantWorkspaceAlreadyExistsNotification":  "Ce workspace est déjà utilisé.",
		"TenantIDAlreadyExistsNotification":         "Un locataire avec cet ID existe déjà.",
		"UnknownTenantStatusNotification":           "Statut de locataire inconnu.",
		"InvalidDisplayNameNotification":            "Le nom n'est pas un nom d'affichage valide.",
		"InvalidDescriptionNotification":            "La description n'est pas valide.",
		"InvalidTenantWorkspaceNotification":        "Le workspace doit comporter de 3 à 63 caractères parmi lettres minuscules, chiffres et tirets.",
		"ReservedTenantWorkspaceNotification":       "Ce workspace est réservé par la plateforme.",
		"TenantWorkspaceIsImmutableNotification":    "Le workspace ne peut pas être modifié.",
		"TenantIDIsImmutableNotification":           "L'ID de locataire ne peut pas être modifié.",
		"TenantIDDerivationMismatchNotification":    "L'ID de locataire ne correspond pas à celui dérivé du workspace.",
		"TenantDescriptionMustDifferNotification":   "La description doit dire autre chose que le nom ou le workspace.",
		"InvalidTenantStatusTransitionNotification": "Le locataire ne peut pas passer à ce statut.",
		"Tenant":                 "Tenant",
		"TenantTenantIDField":    "ID de locataire",
		"TenantNameField":        "Nom",
		"TenantWorkspaceField":   "Workspace",
		"TenantDescriptionField": "Description",
		"TenantStatusField":      "Statut",
		"TenantCreatedAtField":   "Created At",
		"TenantUpdatedAtField":   "Updated At",
		"TenantStatus.trial":     "Essai",
		"TenantStatus.active":    "Actif",
		"TenantStatus.suspended": "Suspendu",
	}
}
