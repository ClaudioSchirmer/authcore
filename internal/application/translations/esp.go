// ESP is the ESP translation catalog.
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

type esp struct{}

func ESP() translation.Module { return esp{} }

func (esp) Language() configuration.Language { return configuration.LangES }

func (esp) Translations() map[string]string {
	return map[string]string{
		"TenantWorkspaceAlreadyExistsNotification":  "Este workspace ya está en uso.",
		"UnknownTenantStatusNotification":           "Estado de tenant desconocido.",
		"InvalidDisplayNameNotification":            "El nombre no es un nombre visible válido.",
		"InvalidDescriptionNotification":            "La descripción no es válida.",
		"InvalidTenantWorkspaceNotification":        "El workspace debe tener de 3 a 63 caracteres entre letras minúsculas, dígitos y guiones.",
		"ReservedTenantWorkspaceNotification":       "Este workspace está reservado por la plataforma.",
		"TenantWorkspaceIsImmutableNotification":    "El workspace no se puede modificar.",
		"TenantDescriptionMustDifferNotification":   "La descripción debe decir algo más que el nombre o el workspace.",
		"InvalidTenantStatusTransitionNotification": "El tenant no puede pasar a ese estado.",
		"Tenant":                                     "Tenant",
		"TenantNameField":                            "Nombre",
		"TenantWorkspaceField":                       "Workspace",
		"TenantDescriptionField":                     "Descripción",
		"TenantStatusField":                          "Estado",
		"TenantCreatedAtField":                       "Created At",
		"TenantUpdatedAtField":                       "Updated At",
		"TenantStatus.trial":                         "Prueba",
		"TenantStatus.active":                        "Activo",
		"TenantStatus.suspended":                     "Suspendido",
		"PermissionAlreadyExistsNotification":        "Este permiso ya existe.",
		"PermissionKeyIsImmutableNotification":       "El recurso y la acción no se pueden modificar.",
		"PermissionDescriptionEchoesKeyNotification": "La descripción debe explicar el permiso, no repetirlo.",
		"InvalidResourceNameNotification":            "El recurso debe ser slugs en minúsculas separados por dos puntos, o exactamente \"*\".",
		"InvalidActionNameNotification":              "La acción debe ser un único slug en minúsculas, o exactamente \"*\".",
		"UnmatchablePermissionKeyNotification":       "Un recurso comodín exige que la acción también sea \"*\".",
		"Permission":                                 "Permission",
		"PermissionResourceField":                    "Recurso",
		"PermissionActionField":                      "Acción",
		"PermissionDescriptionField":                 "Descripción",
		"PermissionKeyField":                         "Permiso",
		"PermissionPermissionField":                  "Permiso",
		"PermissionCreatedAtField":                   "Created At",
		"PermissionUpdatedAtField":                   "Updated At",
		"RoleKeyAlreadyExistsNotification":           "Ya existe un rol con esta clave en este inquilino.",
		"RoleKeyIsImmutableNotification":             "La clave del rol no se puede modificar.",
		"RoleTenantIsImmutableNotification":          "El inquilino propietario no se puede modificar.",
		"RoleTenantDoesNotExistNotification":         "El inquilino propietario no existe, está archivado o está suspendido.",
		"PermissionNotInCatalogNotification":         "Este permiso no está en el catálogo o ya no está activo.",
		"RoleAlreadyGrantsPermissionNotification":    "Este rol ya concede este permiso.",
		"TooManyPermissionsInRoleNotification":       "Un rol puede conceder como máximo {max} permisos.",
		"CannotGrantUnheldPermissionNotification":    "No puede conceder un permiso que usted no posee.",
		"CannotGrantWildcardPermissionNotification":  "Un permiso comodín no puede concederse a un rol.",
		"Role":                                             "Role",
		"RoleTenantIDField":                                "Inquilino",
		"RoleKeyField":                                     "Clave",
		"RoleNameField":                                    "Nombre",
		"RoleDescriptionField":                             "Descripción",
		"RolePermissionPermissionIDField":                  "Permiso",
		"RoleCreatedAtField":                               "Created At",
		"RoleUpdatedAtField":                               "Updated At",
		"RolePermissionResourceField":                      "Recurso",
		"RolePermissionActionField":                        "Acción",
		"InvalidRoleKeyNotification":                       "La clave del rol no es válida.",
		"RolePermissionPermissionField":                    "Permiso",
		"RoleTenantWorkspaceField":                         "Workspace",
		"RoleTenantStatusField":                            "Situación del inquilino",
		"InvalidGroupKeyNotification":                      "La clave del grupo no es válida.",
		"GroupKeyAlreadyExistsNotification":                "Ya existe un grupo con esta clave en este inquilino.",
		"GroupKeyIsImmutableNotification":                  "La clave del grupo no se puede modificar.",
		"GroupTenantIsImmutableNotification":               "El inquilino propietario no se puede modificar.",
		"GroupTenantDoesNotExistNotification":              "El inquilino propietario no existe, está archivado o está suspendido.",
		"RoleNotAvailableInTenantNotification":             "Este rol no está disponible en su inquilino, o ya no está activo.",
		"GroupAlreadyGrantsRoleNotification":               "Este grupo ya confiere este rol.",
		"TooManyRolesInGroupNotification":                  "Un grupo puede conferir como máximo {max} roles.",
		"CannotGrantRoleWithUnheldPermissionsNotification": "No puede conferir un rol que concede permisos que usted no posee.",
		"CannotGrantWildcardRoleNotification":              "Un rol que concede un permiso comodín no puede ser conferido por un grupo.",
		"Group":                                            "Group",
		"GroupTenantIDField":                               "Inquilino",
		"GroupKeyField":                                    "Clave",
		"GroupNameField":                                   "Nombre",
		"GroupDescriptionField":                            "Descripción",
		"GroupRoleRoleIDField":                             "Rol",
		"GroupCreatedAtField":                              "Created At",
		"GroupUpdatedAtField":                              "Updated At",
		"GroupTenantWorkspaceField":                        "Workspace",
		"GroupTenantStatusField":                           "Situación del inquilino",
		"GroupRoleRoleKeyField":                            "Clave del rol",
		"GroupRoleRoleNameField":                           "Nombre del rol",
	}
}
