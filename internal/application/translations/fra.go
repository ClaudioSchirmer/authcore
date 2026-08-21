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
		"Tenant":                                     "Tenant",
		"TenantTenantIDField":                        "ID de locataire",
		"TenantNameField":                            "Nom",
		"TenantWorkspaceField":                       "Workspace",
		"TenantDescriptionField":                     "Description",
		"TenantStatusField":                          "Statut",
		"PermissionAlreadyExistsNotification":        "Cette permission existe déjà.",
		"PermissionKeyIsImmutableNotification":       "La ressource et l'action ne peuvent pas être modifiées.",
		"PermissionDescriptionEchoesKeyNotification": "La description doit expliquer la permission, pas la répéter.",
		"InvalidResourceNameNotification":            "La ressource doit être des slugs minuscules joints par des deux-points, ou exactement *.",
		"InvalidActionNameNotification":              "L'action doit être un seul slug minuscule, ou exactement *.",
		"UnmatchablePermissionKeyNotification":       "Une ressource joker exige une action joker.",
		"Permission":                                 "Permission",
		"PermissionResourceField":                    "Ressource",
		"PermissionActionField":                      "Action",
		"PermissionDescriptionField":                 "Description",
		"PermissionKeyField":                         "Key",
		"PermissionPermissionField":                  "Permission",
		"RoleKeyAlreadyExistsNotification":           "Ce locataire possède déjà un rôle avec cette clé.",
		"RoleKeyIsImmutableNotification":             "La clé du rôle ne peut pas être modifiée.",
		"RoleTenantIsImmutableNotification":          "Le locataire du rôle ne peut pas être modifié.",
		"RoleTenantDoesNotExistNotification":         "Le locataire n'existe pas ou n'est plus actif.",
		"PermissionNotInCatalogNotification":         "Cette permission n'est pas au catalogue, ou n'est plus active.",
		"RoleAlreadyGrantsPermissionNotification":    "Ce rôle accorde déjà cette permission.",
		"TooManyPermissionsInRoleNotification":       "Un rôle peut accorder au maximum {max} permissions.",
		"CannotGrantUnheldPermissionNotification":    "Vous ne pouvez pas accorder une permission que vous ne détenez pas.",
		"CannotGrantWildcardPermissionNotification":  "Une permission joker ne peut pas être accordée sur un rôle.",
		"InvalidRoleKeyNotification":                 "La clé du rôle doit être un seul slug minuscule de 2 à 64 caractères.",
		"Role":                                       "Role",
		"RoleTenantIDField":                          "ID de locataire",
		"RoleKeyField":                               "Clé",
		"RoleNameField":                              "Nom",
		"RoleDescriptionField":                       "Description",
		"RolePermissionPermissionIDField":            "Permission",
	}
}
