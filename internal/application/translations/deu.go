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
		"TenantIDAlreadyExistsNotification":         "Ein Mandant mit dieser Mandanten-ID existiert bereits.",
		"UnknownTenantStatusNotification":           "Unbekannter Mandantenstatus.",
		"InvalidDisplayNameNotification":            "Der Name ist kein gültiger Anzeigename.",
		"InvalidDescriptionNotification":            "Die Beschreibung ist ungültig.",
		"InvalidTenantWorkspaceNotification":        "Der Workspace muss aus 3 bis 63 Zeichen aus Kleinbuchstaben, Ziffern und Bindestrichen bestehen.",
		"ReservedTenantWorkspaceNotification":       "Dieser Workspace ist von der Plattform reserviert.",
		"TenantWorkspaceIsImmutableNotification":    "Der Workspace kann nicht geändert werden.",
		"TenantIDIsImmutableNotification":           "Die Mandanten-ID kann nicht geändert werden.",
		"TenantIDDerivationMismatchNotification":    "Die Mandanten-ID stimmt nicht mit der aus dem Workspace abgeleiteten überein.",
		"TenantDescriptionMustDifferNotification":   "Die Beschreibung muss mehr aussagen als der Name oder der Workspace.",
		"InvalidTenantStatusTransitionNotification": "Der Mandant kann nicht in diesen Status wechseln.",
		"Tenant":                                     "Tenant",
		"TenantTenantIDField":                        "Mandanten-ID",
		"TenantNameField":                            "Name",
		"TenantWorkspaceField":                       "Workspace",
		"TenantDescriptionField":                     "Beschreibung",
		"TenantStatusField":                          "Status",
		"PermissionAlreadyExistsNotification":        "Diese Berechtigung existiert bereits.",
		"PermissionKeyIsImmutableNotification":       "Ressource und Aktion können nicht geändert werden.",
		"PermissionDescriptionEchoesKeyNotification": "Die Beschreibung muss die Berechtigung erklären, nicht wiederholen.",
		"InvalidResourceNameNotification":            "Die Ressource muss aus durch Doppelpunkte verbundenen Kleinbuchstaben-Slugs bestehen oder genau * sein.",
		"InvalidActionNameNotification":              "Die Aktion muss ein einzelner Kleinbuchstaben-Slug sein oder genau *.",
		"UnmatchablePermissionKeyNotification":       "Eine Platzhalter-Ressource erfordert eine Platzhalter-Aktion.",
		"Permission":                                 "Permission",
		"PermissionResourceField":                    "Ressource",
		"PermissionActionField":                      "Aktion",
		"PermissionDescriptionField":                 "Beschreibung",
		"PermissionKeyField":                         "Key",
		"PermissionPermissionField":                  "Berechtigung",
		"RoleKeyAlreadyExistsNotification":           "Dieser Mandant hat bereits eine Rolle mit diesem Schlüssel.",
		"RoleKeyIsImmutableNotification":             "Der Rollenschlüssel kann nicht geändert werden.",
		"RoleTenantIsImmutableNotification":          "Der Mandant der Rolle kann nicht geändert werden.",
		"RoleTenantDoesNotExistNotification":         "Der Mandant existiert nicht oder ist nicht mehr aktiv.",
		"PermissionNotInCatalogNotification":         "Diese Berechtigung ist nicht im Katalog oder nicht mehr aktiv.",
		"RoleAlreadyGrantsPermissionNotification":    "Diese Rolle gewährt diese Berechtigung bereits.",
		"TooManyPermissionsInRoleNotification":       "Eine Rolle darf höchstens {max} Berechtigungen gewähren.",
		"CannotGrantUnheldPermissionNotification":    "Sie können keine Berechtigung gewähren, die Sie nicht selbst besitzen.",
		"CannotGrantWildcardPermissionNotification":  "Eine Platzhalter-Berechtigung kann keiner Rolle gewährt werden.",
		"InvalidRoleKeyNotification":                 "Der Rollenschlüssel muss ein einzelner Kleinbuchstaben-Slug mit 2 bis 64 Zeichen sein.",
		"Role":                                       "Role",
		"RoleTenantIDField":                          "Mandanten-ID",
		"RoleKeyField":                               "Schlüssel",
		"RoleNameField":                              "Name",
		"RoleDescriptionField":                       "Beschreibung",
		"RolePermissionPermissionIDField":            "Berechtigung",
	}
}
