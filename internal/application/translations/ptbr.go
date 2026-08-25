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
		"UnknownTenantStatusNotification":           "Situação de tenant desconhecida.",
		"InvalidDisplayNameNotification":            "O nome não é um nome de exibição válido.",
		"InvalidDescriptionNotification":            "A descrição não é válida.",
		"InvalidTenantWorkspaceNotification":        "O workspace deve ter de 3 a 63 caracteres entre letras minúsculas, dígitos e hífens.",
		"ReservedTenantWorkspaceNotification":       "Este workspace é reservado pela plataforma.",
		"TenantWorkspaceIsImmutableNotification":    "O workspace não pode ser alterado.",
		"TenantDescriptionMustDifferNotification":   "A descrição deve dizer algo além do nome ou do workspace.",
		"InvalidTenantStatusTransitionNotification": "O tenant não pode ir para essa situação.",
		"Tenant":                                     "Tenant",
		"TenantNameField":                            "Nome",
		"TenantWorkspaceField":                       "Workspace",
		"TenantDescriptionField":                     "Descrição",
		"TenantStatusField":                          "Situação",
		"TenantCreatedAtField":                       "Created At",
		"TenantUpdatedAtField":                       "Updated At",
		"TenantStatus.trial":                         "Avaliação",
		"TenantStatus.active":                        "Ativo",
		"TenantStatus.suspended":                     "Suspenso",
		"PermissionAlreadyExistsNotification":        "Esta permissão já existe.",
		"PermissionKeyIsImmutableNotification":       "O recurso e a ação não podem ser alterados.",
		"PermissionDescriptionEchoesKeyNotification": "A descrição deve explicar a permissão, não repeti-la.",
		"InvalidResourceNameNotification":            "O recurso deve ser composto por slugs minúsculos separados por dois-pontos, ou exatamente \"*\".",
		"InvalidActionNameNotification":              "A ação deve ser um único slug minúsculo, ou exatamente \"*\".",
		"UnmatchablePermissionKeyNotification":       "Um recurso curinga exige que a ação também seja \"*\".",
		"Permission":                                 "Permission",
		"PermissionResourceField":                    "Recurso",
		"PermissionActionField":                      "Ação",
		"PermissionDescriptionField":                 "Descrição",
		"PermissionKeyField":                         "Permissão",
		"PermissionPermissionField":                  "Permissão",
		"PermissionCreatedAtField":                   "Created At",
		"PermissionUpdatedAtField":                   "Updated At",
		"RoleKeyAlreadyExistsNotification":           "Já existe um papel com esta chave neste inquilino.",
		"RoleKeyIsImmutableNotification":             "A chave do papel não pode ser alterada.",
		"RoleTenantIsImmutableNotification":          "O inquilino proprietário não pode ser alterado.",
		"RoleTenantDoesNotExistNotification":         "O inquilino proprietário não existe, está arquivado ou está suspenso.",
		"PermissionNotInCatalogNotification":         "Esta permissão não está no catálogo ou não está mais ativa.",
		"RoleAlreadyGrantsPermissionNotification":    "Este papel já concede esta permissão.",
		"TooManyPermissionsInRoleNotification":       "Um papel pode conceder no máximo {max} permissões.",
		"CannotGrantUnheldPermissionNotification":    "Você não pode conceder uma permissão que não possui.",
		"CannotGrantWildcardPermissionNotification":  "Uma permissão curinga não pode ser concedida a um papel.",
		"Role":                                             "Role",
		"RoleTenantIDField":                                "Inquilino",
		"RoleKeyField":                                     "Chave",
		"RoleNameField":                                    "Nome",
		"RoleDescriptionField":                             "Descrição",
		"RolePermissionPermissionIDField":                  "Permissão",
		"RoleCreatedAtField":                               "Created At",
		"RoleUpdatedAtField":                               "Updated At",
		"RolePermissionResourceField":                      "Recurso",
		"RolePermissionActionField":                        "Ação",
		"InvalidRoleKeyNotification":                       "A chave do papel não é válida.",
		"RolePermissionPermissionField":                    "Permissão",
		"RoleTenantWorkspaceField":                         "Workspace",
		"RoleTenantStatusField":                            "Situação do inquilino",
		"InvalidGroupKeyNotification":                      "A chave do grupo não é válida.",
		"GroupKeyAlreadyExistsNotification":                "Já existe um grupo com esta chave neste inquilino.",
		"GroupKeyIsImmutableNotification":                  "A chave do grupo não pode ser alterada.",
		"GroupTenantIsImmutableNotification":               "O inquilino proprietário não pode ser alterado.",
		"GroupTenantDoesNotExistNotification":              "O inquilino proprietário não existe, está arquivado ou está suspenso.",
		"RoleNotAvailableInTenantNotification":             "Este papel não está disponível no seu inquilino, ou não está mais ativo.",
		"GroupAlreadyGrantsRoleNotification":               "Este grupo já confere este papel.",
		"TooManyRolesInGroupNotification":                  "Um grupo pode conferir no máximo {max} papéis.",
		"CannotGrantRoleWithUnheldPermissionsNotification": "Você não pode conferir um papel que concede permissões que você não possui.",
		"CannotGrantWildcardRoleNotification":              "Um papel que concede uma permissão curinga não pode ser conferido por um grupo.",
		"Group":                                            "Group",
		"GroupTenantIDField":                               "Inquilino",
		"GroupKeyField":                                    "Chave",
		"GroupNameField":                                   "Nome",
		"GroupDescriptionField":                            "Descrição",
		"GroupRoleRoleIDField":                             "Papel",
		"GroupCreatedAtField":                              "Created At",
		"GroupUpdatedAtField":                              "Updated At",
		"GroupTenantWorkspaceField":                        "Workspace",
		"GroupTenantStatusField":                           "Situação do inquilino",
		"GroupRoleRoleKeyField":                            "Chave do papel",
		"GroupRoleRoleNameField":                           "Nome do papel",
	}
}
