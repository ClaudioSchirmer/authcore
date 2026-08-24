// ITA is the ITA translation catalog.
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

type ita struct{}

func ITA() translation.Module { return ita{} }

func (ita) Language() configuration.Language { return configuration.LangIT }

func (ita) Translations() map[string]string {
	return map[string]string{
		"TenantWorkspaceAlreadyExistsNotification":  "Questo workspace è già in uso.",
		"TenantIDAlreadyExistsNotification":         "Esiste già un tenant con questo ID.",
		"UnknownTenantStatusNotification":           "Stato del tenant sconosciuto.",
		"InvalidDisplayNameNotification":            "Il nome non è un nome visualizzato valido.",
		"InvalidDescriptionNotification":            "La descrizione non è valida.",
		"InvalidTenantWorkspaceNotification":        "Il workspace deve avere da 3 a 63 caratteri tra lettere minuscole, cifre e trattini.",
		"ReservedTenantWorkspaceNotification":       "Questo workspace è riservato dalla piattaforma.",
		"TenantWorkspaceIsImmutableNotification":    "Il workspace non può essere modificato.",
		"TenantIDIsImmutableNotification":           "L'ID del tenant non può essere modificato.",
		"TenantIDDerivationMismatchNotification":    "L'ID del tenant non corrisponde a quello derivato dal workspace.",
		"TenantDescriptionMustDifferNotification":   "La descrizione deve dire qualcosa di diverso dal nome o dal workspace.",
		"InvalidTenantStatusTransitionNotification": "Il tenant non può passare a questo stato.",
		"Tenant":                                     "Tenant",
		"TenantTenantIDField":                        "ID tenant",
		"TenantNameField":                            "Nome",
		"TenantWorkspaceField":                       "Workspace",
		"TenantDescriptionField":                     "Descrizione",
		"TenantStatusField":                          "Stato",
		"TenantCreatedAtField":                       "Created At",
		"TenantUpdatedAtField":                       "Updated At",
		"TenantStatus.trial":                         "Prova",
		"TenantStatus.active":                        "Attivo",
		"TenantStatus.suspended":                     "Sospeso",
		"PermissionAlreadyExistsNotification":        "Questo permesso esiste già.",
		"PermissionKeyIsImmutableNotification":       "La risorsa e l'azione non possono essere modificate.",
		"PermissionDescriptionEchoesKeyNotification": "La descrizione deve spiegare il permesso, non ripeterlo.",
		"InvalidResourceNameNotification":            "La risorsa deve essere composta da slug minuscoli separati da due punti, o esattamente \"*\".",
		"InvalidActionNameNotification":              "L'azione deve essere un singolo slug minuscolo, o esattamente \"*\".",
		"UnmatchablePermissionKeyNotification":       "Una risorsa jolly richiede che anche l'azione sia \"*\".",
		"Permission":                                 "Permission",
		"PermissionResourceField":                    "Risorsa",
		"PermissionActionField":                      "Azione",
		"PermissionDescriptionField":                 "Descrizione",
		"PermissionKeyField":                         "Permesso",
		"PermissionPermissionField":                  "Permesso",
		"PermissionCreatedAtField":                   "Created At",
		"PermissionUpdatedAtField":                   "Updated At",
	}
}
