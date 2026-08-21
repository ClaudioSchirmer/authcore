// PTBR is the PTBR translation catalog.
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

type ptbr struct{}

func PTBR() translation.Module { return ptbr{} }

func (ptbr) Language() configuration.Language { return configuration.LangPTBR }

func (ptbr) Translations() map[string]string {
	return map[string]string{
		"TenantWorkspaceAlreadyExistsNotification":  "Este workspace já está em uso.",
		"TenantIDAlreadyExistsNotification":         "Já existe um tenant com este ID de tenant.",
		"UnknownTenantStatusNotification":           "Situação de tenant desconhecida.",
		"InvalidDisplayNameNotification":            "O nome não é um nome de exibição válido.",
		"InvalidDescriptionNotification":            "A descrição não é válida.",
		"InvalidTenantWorkspaceNotification":        "O workspace deve ter de 3 a 63 caracteres entre letras minúsculas, dígitos e hífens.",
		"ReservedTenantWorkspaceNotification":       "Este workspace é reservado pela plataforma.",
		"TenantWorkspaceIsImmutableNotification":    "O workspace não pode ser alterado.",
		"TenantIDIsImmutableNotification":           "O ID do tenant não pode ser alterado.",
		"TenantIDDerivationMismatchNotification":    "O ID do tenant não corresponde ao derivado do workspace.",
		"TenantDescriptionMustDifferNotification":   "A descrição deve dizer algo além do nome ou do workspace.",
		"InvalidTenantStatusTransitionNotification": "O tenant não pode ir para essa situação.",
		"Tenant":                                     "Tenant",
		"TenantTenantIDField":                        "ID do tenant",
		"TenantNameField":                            "Nome",
		"TenantWorkspaceField":                       "Workspace",
		"TenantDescriptionField":                     "Descrição",
		"TenantStatusField":                          "Situação",
		"PermissionAlreadyExistsNotification":        "Esta permissão já existe.",
		"PermissionKeyIsImmutableNotification":       "O recurso e a ação não podem ser alterados.",
		"PermissionDescriptionEchoesKeyNotification": "A descrição deve explicar a permissão, não repeti-la.",
		"InvalidResourceNameNotification":            "O recurso deve ser slugs minúsculos unidos por dois-pontos, ou exatamente *.",
		"InvalidActionNameNotification":              "A ação deve ser um único slug minúsculo, ou exatamente *.",
		"UnmatchablePermissionKeyNotification":       "Um recurso curinga exige uma ação curinga.",
		"Permission":                                 "Permission",
		"PermissionResourceField":                    "Recurso",
		"PermissionActionField":                      "Ação",
		"PermissionDescriptionField":                 "Descrição",
		"PermissionKeyField":                         "Key",
		"PermissionPermissionField":                  "Permissão",
	}
}
