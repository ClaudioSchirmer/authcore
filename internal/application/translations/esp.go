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
		"TenantIDAlreadyExistsNotification":         "Ya existe un tenant con este ID de tenant.",
		"UnknownTenantStatusNotification":           "Estado de tenant desconocido.",
		"InvalidDisplayNameNotification":            "El nombre no es un nombre visible válido.",
		"InvalidDescriptionNotification":            "La descripción no es válida.",
		"InvalidTenantWorkspaceNotification":        "El workspace debe tener de 3 a 63 caracteres entre letras minúsculas, dígitos y guiones.",
		"ReservedTenantWorkspaceNotification":       "Este workspace está reservado por la plataforma.",
		"TenantWorkspaceIsImmutableNotification":    "El workspace no se puede modificar.",
		"TenantIDIsImmutableNotification":           "El ID de tenant no se puede modificar.",
		"TenantIDDerivationMismatchNotification":    "El ID de tenant no coincide con el derivado del workspace.",
		"TenantDescriptionMustDifferNotification":   "La descripción debe decir algo más que el nombre o el workspace.",
		"InvalidTenantStatusTransitionNotification": "El tenant no puede pasar a ese estado.",
		"Tenant":                                     "Tenant",
		"TenantTenantIDField":                        "ID de tenant",
		"TenantNameField":                            "Nombre",
		"TenantWorkspaceField":                       "Workspace",
		"TenantDescriptionField":                     "Descripción",
		"TenantStatusField":                          "Estado",
		"PermissionAlreadyExistsNotification":        "Este permiso ya existe.",
		"PermissionKeyIsImmutableNotification":       "El recurso y la acción no se pueden modificar.",
		"PermissionDescriptionEchoesKeyNotification": "La descripción debe explicar el permiso, no repetirlo.",
		"InvalidResourceNameNotification":            "El recurso debe ser slugs en minúsculas unidos por dos puntos, o exactamente *.",
		"InvalidActionNameNotification":              "La acción debe ser un único slug en minúsculas, o exactamente *.",
		"UnmatchablePermissionKeyNotification":       "Un recurso comodín exige una acción comodín.",
		"Permission":                                 "Permission",
		"PermissionResourceField":                    "Recurso",
		"PermissionActionField":                      "Acción",
		"PermissionDescriptionField":                 "Descripción",
		"PermissionKeyField":                         "Key",
		"PermissionPermissionField":                  "Permiso",
		"RoleKeyAlreadyExistsNotification":           "Este tenant ya tiene un rol con esta clave.",
		"RoleKeyIsImmutableNotification":             "La clave del rol no se puede modificar.",
		"RoleTenantIsImmutableNotification":          "El tenant del rol no se puede modificar.",
		"RoleTenantDoesNotExistNotification":         "El tenant no existe o ya no está activo.",
		"PermissionNotInCatalogNotification":         "Este permiso no está en el catálogo, o ya no está activo.",
		"RoleAlreadyGrantsPermissionNotification":    "Este rol ya concede este permiso.",
		"TooManyPermissionsInRoleNotification":       "Un rol puede conceder como máximo {max} permisos.",
		"CannotGrantUnheldPermissionNotification":    "No puede conceder un permiso que usted no posee.",
		"CannotGrantWildcardPermissionNotification":  "Un permiso comodín no puede concederse en un rol.",
		"InvalidRoleKeyNotification":                 "La clave del rol debe ser un único slug en minúsculas de 2 a 64 caracteres.",
		"Role":                                       "Role",
		"RoleTenantIDField":                          "ID de tenant",
		"RoleKeyField":                               "Clave",
		"RoleNameField":                              "Nombre",
		"RoleDescriptionField":                       "Descripción",
		"RolePermissionPermissionIDField":            "Permiso",
	}
}
