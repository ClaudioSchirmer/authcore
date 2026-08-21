// ENG is the ENG translation catalog.
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

type eng struct{}

func ENG() translation.Module { return eng{} }

func (eng) Language() configuration.Language { return configuration.LangENG }

func (eng) Translations() map[string]string {
	return map[string]string{
		"TenantWorkspaceAlreadyExistsNotification":  "This workspace is already taken.",
		"TenantIDAlreadyExistsNotification":         "A tenant with this tenant ID already exists.",
		"UnknownTenantStatusNotification":           "Unknown tenant status.",
		"InvalidDisplayNameNotification":            "The name is not a valid display name.",
		"InvalidDescriptionNotification":            "The description is not valid.",
		"InvalidTenantWorkspaceNotification":        "The workspace must be 3 to 63 characters of lowercase letters, digits and hyphens.",
		"ReservedTenantWorkspaceNotification":       "This workspace is reserved by the platform.",
		"TenantWorkspaceIsImmutableNotification":    "The workspace cannot be changed.",
		"TenantIDIsImmutableNotification":           "The tenant ID cannot be changed.",
		"TenantIDDerivationMismatchNotification":    "The tenant ID does not match the one derived from the workspace.",
		"TenantDescriptionMustDifferNotification":   "The description must say something other than the name or the workspace.",
		"InvalidTenantStatusTransitionNotification": "The tenant cannot move to that status.",
		"Tenant":                                     "Tenant",
		"TenantTenantIDField":                        "Tenant ID",
		"TenantNameField":                            "Name",
		"TenantWorkspaceField":                       "Workspace",
		"TenantDescriptionField":                     "Description",
		"TenantStatusField":                          "Status",
		"PermissionAlreadyExistsNotification":        "This permission already exists.",
		"PermissionKeyIsImmutableNotification":       "The resource and the action cannot be changed.",
		"PermissionDescriptionEchoesKeyNotification": "The description must explain the permission, not repeat it.",
		"InvalidResourceNameNotification":            "The resource must be lowercase slugs joined by colons, or exactly *.",
		"InvalidActionNameNotification":              "The action must be a single lowercase slug, or exactly *.",
		"UnmatchablePermissionKeyNotification":       "A wildcard resource requires a wildcard action.",
		"Permission":                                 "Permission",
		"PermissionResourceField":                    "Resource",
		"PermissionActionField":                      "Action",
		"PermissionDescriptionField":                 "Description",
		"PermissionKeyField":                         "Key",
		"PermissionPermissionField":                  "Permission",
		"RoleKeyAlreadyExistsNotification":           "This tenant already has a role with this key.",
		"RoleKeyIsImmutableNotification":             "The role key cannot be changed.",
		"RoleTenantIsImmutableNotification":          "The role's tenant cannot be changed.",
		"RoleTenantDoesNotExistNotification":         "The tenant does not exist or is no longer active.",
		"PermissionNotInCatalogNotification":         "This permission is not in the catalog, or is no longer active.",
		"RoleAlreadyGrantsPermissionNotification":    "This role already grants this permission.",
		"TooManyPermissionsInRoleNotification":       "A role may grant at most {max} permissions.",
		"CannotGrantUnheldPermissionNotification":    "You cannot grant a permission you do not hold.",
		"CannotGrantWildcardPermissionNotification":  "A wildcard permission cannot be granted on a role.",
		"InvalidRoleKeyNotification":                 "The role key must be a single lowercase slug of 2 to 64 characters.",
		"Role":                                       "Role",
		"RoleTenantIDField":                          "Tenant ID",
		"RoleKeyField":                               "Key",
		"RoleNameField":                              "Name",
		"RoleDescriptionField":                       "Description",
		"RolePermissionPermissionIDField":            "Permission",
	}
}
