// DEU is the DEU translation catalog.
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

type deu struct{}

func DEU() translation.Module { return deu{} }

func (deu) Language() configuration.Language { return configuration.LangDE }

func (deu) Translations() map[string]string {
	return map[string]string{
		"TenantWorkspaceAlreadyExistsNotification":  "Dieser Workspace ist bereits vergeben.",
		"UnknownTenantStatusNotification":           "Unbekannter Mandantenstatus.",
		"InvalidDisplayNameNotification":            "Der Name ist kein gültiger Anzeigename.",
		"InvalidDescriptionNotification":            "Die Beschreibung ist ungültig.",
		"InvalidTenantWorkspaceNotification":        "Der Workspace muss aus 3 bis 63 Zeichen aus Kleinbuchstaben, Ziffern und Bindestrichen bestehen.",
		"ReservedTenantWorkspaceNotification":       "Dieser Workspace ist von der Plattform reserviert.",
		"TenantWorkspaceIsImmutableNotification":    "Der Workspace kann nicht geändert werden.",
		"TenantDescriptionMustDifferNotification":   "Die Beschreibung muss mehr aussagen als der Name oder der Workspace.",
		"InvalidTenantStatusTransitionNotification": "Der Mandant kann nicht in diesen Status wechseln.",
		"Tenant":                                     "Tenant",
		"TenantNameField":                            "Name",
		"TenantWorkspaceField":                       "Workspace",
		"TenantDescriptionField":                     "Beschreibung",
		"TenantStatusField":                          "Status",
		"TenantCreatedAtField":                       "Created At",
		"TenantUpdatedAtField":                       "Updated At",
		"TenantStatus.trial":                         "Testphase",
		"TenantStatus.active":                        "Aktiv",
		"TenantStatus.suspended":                     "Gesperrt",
		"PermissionAlreadyExistsNotification":        "Diese Berechtigung existiert bereits.",
		"PermissionKeyIsImmutableNotification":       "Ressource und Aktion können nicht geändert werden.",
		"PermissionDescriptionEchoesKeyNotification": "Die Beschreibung muss die Berechtigung erklären, nicht wiederholen.",
		"InvalidResourceNameNotification":            "Die Ressource muss aus durch Doppelpunkte getrennten Kleinbuchstaben-Slugs bestehen oder genau \"*\" sein.",
		"InvalidActionNameNotification":              "Die Aktion muss ein einzelner Kleinbuchstaben-Slug oder genau \"*\" sein.",
		"UnmatchablePermissionKeyNotification":       "Eine Platzhalter-Ressource verlangt, dass auch die Aktion \"*\" ist.",
		"Permission":                                 "Permission",
		"PermissionResourceField":                    "Ressource",
		"PermissionActionField":                      "Aktion",
		"PermissionDescriptionField":                 "Beschreibung",
		"PermissionKeyField":                         "Berechtigung",
		"PermissionPermissionField":                  "Berechtigung",
		"PermissionCreatedAtField":                   "Created At",
		"PermissionUpdatedAtField":                   "Updated At",
		"RoleKeyAlreadyExistsNotification":           "In diesem Mandanten existiert bereits eine Rolle mit diesem Schlüssel.",
		"RoleKeyIsImmutableNotification":             "Der Rollenschlüssel kann nicht geändert werden.",
		"RoleTenantIsImmutableNotification":          "Der besitzende Mandant kann nicht geändert werden.",
		"RoleTenantDoesNotExistNotification":         "Der besitzende Mandant existiert nicht, ist archiviert oder ist gesperrt.",
		"PermissionNotInCatalogNotification":         "Diese Berechtigung ist nicht im Katalog oder nicht mehr aktiv.",
		"RoleAlreadyGrantsPermissionNotification":    "Diese Rolle gewährt diese Berechtigung bereits.",
		"TooManyPermissionsInRoleNotification":       "Eine Rolle darf höchstens {max} Berechtigungen gewähren.",
		"CannotGrantUnheldPermissionNotification":    "Sie können keine Berechtigung gewähren, die Sie nicht besitzen.",
		"CannotGrantWildcardPermissionNotification":  "Eine Platzhalter-Berechtigung kann keiner Rolle gewährt werden.",
		"Role":                            "Role",
		"RoleTenantIDField":               "Mandant",
		"RoleKeyField":                    "Schlüssel",
		"RoleNameField":                   "Name",
		"RoleDescriptionField":            "Beschreibung",
		"RolePermissionPermissionIDField": "Berechtigung",
		"RoleCreatedAtField":              "Created At",
		"RoleUpdatedAtField":              "Updated At",
		"RolePermissionResourceField":     "Ressource",
		"RolePermissionActionField":       "Aktion",
		"InvalidRoleKeyNotification":      "Der Rollenschlüssel ist ungültig.",
		"RolePermissionPermissionField":   "Berechtigung",
		"RoleTenantWorkspaceField":        "Workspace",
		"RoleTenantStatusField":           "Mandantenstatus",
	}
}
