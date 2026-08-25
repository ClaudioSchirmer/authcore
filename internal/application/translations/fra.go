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
		"UnknownTenantStatusNotification":           "Statut de locataire inconnu.",
		"InvalidDisplayNameNotification":            "Le nom n'est pas un nom d'affichage valide.",
		"InvalidDescriptionNotification":            "La description n'est pas valide.",
		"InvalidTenantWorkspaceNotification":        "Le workspace doit comporter de 3 à 63 caractères parmi lettres minuscules, chiffres et tirets.",
		"ReservedTenantWorkspaceNotification":       "Ce workspace est réservé par la plateforme.",
		"TenantWorkspaceIsImmutableNotification":    "Le workspace ne peut pas être modifié.",
		"TenantDescriptionMustDifferNotification":   "La description doit dire autre chose que le nom ou le workspace.",
		"InvalidTenantStatusTransitionNotification": "Le locataire ne peut pas passer à ce statut.",
		"Tenant":                                     "Tenant",
		"TenantNameField":                            "Nom",
		"TenantWorkspaceField":                       "Workspace",
		"TenantDescriptionField":                     "Description",
		"TenantStatusField":                          "Statut",
		"TenantCreatedAtField":                       "Created At",
		"TenantUpdatedAtField":                       "Updated At",
		"TenantStatus.trial":                         "Essai",
		"TenantStatus.active":                        "Actif",
		"TenantStatus.suspended":                     "Suspendu",
		"PermissionAlreadyExistsNotification":        "Cette permission existe déjà.",
		"PermissionKeyIsImmutableNotification":       "La ressource et l'action ne peuvent pas être modifiées.",
		"PermissionDescriptionEchoesKeyNotification": "La description doit expliquer la permission, pas la répéter.",
		"InvalidResourceNameNotification":            "La ressource doit être des slugs en minuscules séparés par des deux-points, ou exactement « * ».",
		"InvalidActionNameNotification":              "L'action doit être un seul slug en minuscules, ou exactement « * ».",
		"UnmatchablePermissionKeyNotification":       "Une ressource joker impose que l'action soit également « * ».",
		"Permission":                                 "Permission",
		"PermissionResourceField":                    "Ressource",
		"PermissionActionField":                      "Action",
		"PermissionDescriptionField":                 "Description",
		"PermissionKeyField":                         "Permission",
		"PermissionPermissionField":                  "Permission",
		"PermissionCreatedAtField":                   "Created At",
		"PermissionUpdatedAtField":                   "Updated At",
		"RoleKeyAlreadyExistsNotification":           "Un rôle avec cette clé existe déjà dans ce locataire.",
		"RoleKeyIsImmutableNotification":             "La clé du rôle ne peut pas être modifiée.",
		"RoleTenantIsImmutableNotification":          "Le locataire propriétaire ne peut pas être modifié.",
		"RoleTenantDoesNotExistNotification":         "Le locataire propriétaire n'existe pas, est archivé ou est suspendu.",
		"PermissionNotInCatalogNotification":         "Cette permission n'est pas au catalogue ou n'est plus active.",
		"RoleAlreadyGrantsPermissionNotification":    "Ce rôle accorde déjà cette permission.",
		"TooManyPermissionsInRoleNotification":       "Un rôle peut accorder au maximum {max} permissions.",
		"CannotGrantUnheldPermissionNotification":    "Vous ne pouvez pas accorder une permission que vous ne détenez pas.",
		"CannotGrantWildcardPermissionNotification":  "Une permission joker ne peut pas être accordée à un rôle.",
		"Role":                                             "Role",
		"RoleTenantIDField":                                "Locataire",
		"RoleKeyField":                                     "Clé",
		"RoleNameField":                                    "Nom",
		"RoleDescriptionField":                             "Description",
		"RolePermissionPermissionIDField":                  "Permission",
		"RoleCreatedAtField":                               "Created At",
		"RoleUpdatedAtField":                               "Updated At",
		"RolePermissionResourceField":                      "Ressource",
		"RolePermissionActionField":                        "Action",
		"InvalidRoleKeyNotification":                       "La clé du rôle n'est pas valide.",
		"RolePermissionPermissionField":                    "Permission",
		"RoleTenantWorkspaceField":                         "Espace de travail",
		"RoleTenantStatusField":                            "Statut du locataire",
		"InvalidGroupKeyNotification":                      "La clé du groupe n'est pas valide.",
		"GroupKeyAlreadyExistsNotification":                "Un groupe avec cette clé existe déjà dans ce locataire.",
		"GroupKeyIsImmutableNotification":                  "La clé du groupe ne peut pas être modifiée.",
		"GroupTenantIsImmutableNotification":               "Le locataire propriétaire ne peut pas être modifié.",
		"GroupTenantDoesNotExistNotification":              "Le locataire propriétaire n'existe pas, est archivé ou est suspendu.",
		"RoleNotAvailableInTenantNotification":             "Ce rôle n'est pas disponible dans votre locataire, ou n'est plus actif.",
		"GroupAlreadyGrantsRoleNotification":               "Ce groupe confère déjà ce rôle.",
		"TooManyRolesInGroupNotification":                  "Un groupe peut conférer au maximum {max} rôles.",
		"CannotGrantRoleWithUnheldPermissionsNotification": "Vous ne pouvez pas conférer un rôle qui accorde des permissions que vous ne détenez pas.",
		"CannotGrantWildcardRoleNotification":              "Un rôle accordant une permission joker ne peut pas être conféré par un groupe.",
		"Group":                                            "Group",
		"GroupTenantIDField":                               "Locataire",
		"GroupKeyField":                                    "Clé",
		"GroupNameField":                                   "Nom",
		"GroupDescriptionField":                            "Description",
		"GroupRoleRoleIDField":                             "Rôle",
		"GroupCreatedAtField":                              "Created At",
		"GroupUpdatedAtField":                              "Updated At",
		"GroupTenantWorkspaceField":                        "Espace de travail",
		"GroupTenantStatusField":                           "Statut du locataire",
		"GroupRoleRoleKeyField":                            "Clé du rôle",
		"GroupRoleRoleNameField":                           "Nom du rôle",
	}
}
