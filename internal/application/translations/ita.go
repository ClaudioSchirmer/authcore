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
		"UnknownTenantStatusNotification":           "Stato del tenant sconosciuto.",
		"InvalidDisplayNameNotification":            "Il nome non è un nome visualizzato valido.",
		"InvalidDescriptionNotification":            "La descrizione non è valida.",
		"InvalidTenantWorkspaceNotification":        "Il workspace deve avere da 3 a 63 caratteri tra lettere minuscole, cifre e trattini.",
		"ReservedTenantWorkspaceNotification":       "Questo workspace è riservato dalla piattaforma.",
		"TenantWorkspaceIsImmutableNotification":    "Il workspace non può essere modificato.",
		"TenantDescriptionMustDifferNotification":   "La descrizione deve dire qualcosa di diverso dal nome o dal workspace.",
		"InvalidTenantStatusTransitionNotification": "Il tenant non può passare a questo stato.",
		"Tenant":                                     "Tenant",
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
		"RoleKeyAlreadyExistsNotification":           "Esiste già un ruolo con questa chiave in questo tenant.",
		"RoleKeyIsImmutableNotification":             "La chiave del ruolo non può essere modificata.",
		"RoleTenantIsImmutableNotification":          "Il tenant proprietario non può essere modificato.",
		"RoleTenantDoesNotExistNotification":         "Il tenant proprietario non esiste, è archiviato o è sospeso.",
		"PermissionNotInCatalogNotification":         "Questo permesso non è nel catalogo o non è più attivo.",
		"RoleAlreadyGrantsPermissionNotification":    "Questo ruolo concede già questo permesso.",
		"TooManyPermissionsInRoleNotification":       "Un ruolo può concedere al massimo {max} permessi.",
		"CannotGrantUnheldPermissionNotification":    "Non è possibile concedere un permesso che non si possiede.",
		"CannotGrantWildcardPermissionNotification":  "Un permesso jolly non può essere concesso a un ruolo.",
		"Role":                            "Role",
		"RoleTenantIDField":               "Tenant",
		"RoleKeyField":                    "Chiave",
		"RoleNameField":                   "Nome",
		"RoleDescriptionField":            "Descrizione",
		"RolePermissionPermissionIDField": "Permesso",
		"RoleCreatedAtField":              "Created At",
		"RoleUpdatedAtField":              "Updated At",
		"RolePermissionResourceField":     "Risorsa",
		"RolePermissionActionField":       "Azione",
		"InvalidRoleKeyNotification":      "La chiave del ruolo non è valida.",
		"RolePermissionPermissionField":   "Permesso",
		"RoleTenantWorkspaceField":        "Workspace",
		"RoleTenantStatusField":           "Stato del tenant",
	}
}
