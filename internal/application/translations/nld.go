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
		"TenantIDAlreadyExistsNotification":         "Er bestaat al een tenant met dit tenant-ID.",
		"UnknownTenantStatusNotification":           "Onbekende tenantstatus.",
		"InvalidDisplayNameNotification":            "De naam is geen geldige weergavenaam.",
		"InvalidDescriptionNotification":            "De beschrijving is niet geldig.",
		"InvalidTenantWorkspaceNotification":        "De workspace moet 3 tot 63 tekens bevatten uit kleine letters, cijfers en koppeltekens.",
		"ReservedTenantWorkspaceNotification":       "Deze workspace is gereserveerd door het platform.",
		"TenantWorkspaceIsImmutableNotification":    "De workspace kan niet worden gewijzigd.",
		"TenantIDIsImmutableNotification":           "Het tenant-ID kan niet worden gewijzigd.",
		"TenantIDDerivationMismatchNotification":    "Het tenant-ID komt niet overeen met het uit de workspace afgeleide ID.",
		"TenantDescriptionMustDifferNotification":   "De beschrijving moet meer zeggen dan de naam of de workspace.",
		"InvalidTenantStatusTransitionNotification": "De tenant kan niet naar die status.",
		"Tenant":                                     "Tenant",
		"TenantTenantIDField":                        "Tenant-ID",
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
	}
}
