// NLD is the NLD translation catalog.
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

type nld struct{}

func NLD() translation.Module { return nld{} }

func (nld) Language() configuration.Language { return configuration.LangNL }

func (nld) Translations() map[string]string {
	return map[string]string{
		"TenantWorkspaceAlreadyExistsNotification":  "Deze workspace is al in gebruik.",
		"UnknownTenantStatusNotification":           "Onbekende tenantstatus.",
		"InvalidDisplayNameNotification":            "De naam is geen geldige weergavenaam.",
		"InvalidDescriptionNotification":            "De beschrijving is niet geldig.",
		"InvalidTenantWorkspaceNotification":        "De workspace moet 3 tot 63 tekens bevatten uit kleine letters, cijfers en koppeltekens.",
		"ReservedTenantWorkspaceNotification":       "Deze workspace is gereserveerd door het platform.",
		"TenantWorkspaceIsImmutableNotification":    "De workspace kan niet worden gewijzigd.",
		"TenantDescriptionMustDifferNotification":   "De beschrijving moet meer zeggen dan de naam of de workspace.",
		"InvalidTenantStatusTransitionNotification": "De tenant kan niet naar die status.",
		"Tenant":                                     "Tenant",
		"TenantNameField":                            "Naam",
		"TenantWorkspaceField":                       "Workspace",
		"TenantDescriptionField":                     "Beschrijving",
		"TenantStatusField":                          "Status",
		"TenantCreatedAtField":                       "Created At",
		"TenantUpdatedAtField":                       "Updated At",
		"TenantStatus.trial":                         "Proefperiode",
		"TenantStatus.active":                        "Actief",
		"TenantStatus.suspended":                     "Opgeschort",
		"PermissionAlreadyExistsNotification":        "Dit recht bestaat al.",
		"PermissionKeyIsImmutableNotification":       "De resource en de actie kunnen niet worden gewijzigd.",
		"PermissionDescriptionEchoesKeyNotification": "De beschrijving moet het recht uitleggen, niet herhalen.",
		"InvalidResourceNameNotification":            "De resource moet bestaan uit kleine slugs gescheiden door dubbele punten, of exact \"*\".",
		"InvalidActionNameNotification":              "De actie moet één kleine slug zijn, of exact \"*\".",
		"UnmatchablePermissionKeyNotification":       "Een jokerteken-resource vereist dat de actie ook \"*\" is.",
		"Permission":                                 "Permission",
		"PermissionResourceField":                    "Resource",
		"PermissionActionField":                      "Actie",
		"PermissionDescriptionField":                 "Beschrijving",
		"PermissionKeyField":                         "Recht",
		"PermissionPermissionField":                  "Recht",
		"PermissionCreatedAtField":                   "Created At",
		"PermissionUpdatedAtField":                   "Updated At",
		"RoleKeyAlreadyExistsNotification":           "Er bestaat al een rol met deze sleutel in deze tenant.",
		"RoleKeyIsImmutableNotification":             "De rolsleutel kan niet worden gewijzigd.",
		"RoleTenantIsImmutableNotification":          "De eigenaar-tenant kan niet worden gewijzigd.",
		"RoleTenantDoesNotExistNotification":         "De eigenaar-tenant bestaat niet, is gearchiveerd of is opgeschort.",
		"PermissionNotInCatalogNotification":         "Dit recht staat niet in de catalogus of is niet meer actief.",
		"RoleAlreadyGrantsPermissionNotification":    "Deze rol verleent dit recht al.",
		"TooManyPermissionsInRoleNotification":       "Een rol mag maximaal {max} rechten verlenen.",
		"CannotGrantUnheldPermissionNotification":    "U kunt geen recht verlenen dat u zelf niet heeft.",
		"CannotGrantWildcardPermissionNotification":  "Een jokerrecht kan niet aan een rol worden verleend.",
		"Role":                                             "Role",
		"RoleTenantIDField":                                "Tenant",
		"RoleKeyField":                                     "Sleutel",
		"RoleNameField":                                    "Naam",
		"RoleDescriptionField":                             "Beschrijving",
		"RolePermissionPermissionIDField":                  "Recht",
		"RoleCreatedAtField":                               "Created At",
		"RoleUpdatedAtField":                               "Updated At",
		"RolePermissionResourceField":                      "Resource",
		"RolePermissionActionField":                        "Actie",
		"InvalidRoleKeyNotification":                       "De rolsleutel is ongeldig.",
		"RolePermissionPermissionField":                    "Recht",
		"RoleTenantWorkspaceField":                         "Workspace",
		"RoleTenantStatusField":                            "Tenantstatus",
		"InvalidGroupKeyNotification":                      "De groepssleutel is ongeldig.",
		"GroupKeyAlreadyExistsNotification":                "Er bestaat al een groep met deze sleutel in deze tenant.",
		"GroupKeyIsImmutableNotification":                  "De groepssleutel kan niet worden gewijzigd.",
		"GroupTenantIsImmutableNotification":               "De eigenaar-tenant kan niet worden gewijzigd.",
		"GroupTenantDoesNotExistNotification":              "De eigenaar-tenant bestaat niet, is gearchiveerd of is opgeschort.",
		"RoleNotAvailableInTenantNotification":             "Deze rol is niet beschikbaar in uw tenant, of is niet meer actief.",
		"GroupAlreadyGrantsRoleNotification":               "Deze groep verleent deze rol al.",
		"TooManyRolesInGroupNotification":                  "Een groep mag maximaal {max} rollen verlenen.",
		"CannotGrantRoleWithUnheldPermissionsNotification": "U kunt geen rol verlenen die rechten geeft die u zelf niet heeft.",
		"CannotGrantWildcardRoleNotification":              "Een rol die een jokerrecht verleent, kan niet door een groep worden verleend.",
		"Group":                                            "Group",
		"GroupTenantIDField":                               "Tenant",
		"GroupKeyField":                                    "Sleutel",
		"GroupNameField":                                   "Naam",
		"GroupDescriptionField":                            "Beschrijving",
		"GroupRoleRoleIDField":                             "Rol",
		"GroupCreatedAtField":                              "Created At",
		"GroupUpdatedAtField":                              "Updated At",
		"GroupTenantWorkspaceField":                        "Workspace",
		"GroupTenantStatusField":                           "Tenantstatus",
		"GroupRoleRoleKeyField":                            "Rolsleutel",
		"GroupRoleRoleNameField":                           "Rolnaam",
	}
}
