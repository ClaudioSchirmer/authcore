// Notifications raised by this package's types.
//
// This file is a registration site: omnicore-gen appends the declarations an
// entity needs, and maintains the ones IT wrote — it records a hash of each,
// so a declaration you changed is recognised as yours and left alone (the
// report names it instead). Nothing here is ever rewritten wholesale: this
// file belongs to every entity in the project.
//
// It lives beside the types that raise it: the domain package imports vos, so
// a notification raised by a value object cannot live in domain.

package domain

import "github.com/ClaudioSchirmer/omnicore/domain"

// TenantWorkspaceAlreadyExistsNotification reaches the caller as 409 (already
// exists). The struct NAME is the translation key, so renaming it here without
// renaming it in the seven catalogs leaves the message untranslated.
type TenantWorkspaceAlreadyExistsNotification struct {
	domain.DomainNotificationBase
}

func (TenantWorkspaceAlreadyExistsNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticConflict
}

// TenantWorkspaceIsImmutableNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type TenantWorkspaceIsImmutableNotification struct{ domain.DomainNotificationBase }

// TenantDescriptionMustDifferNotification reaches the caller as 422. The
// struct NAME is the translation key, so renaming it here without renaming it
// in the seven catalogs leaves the message untranslated.
type TenantDescriptionMustDifferNotification struct{ domain.DomainNotificationBase }

// InvalidTenantStatusTransitionNotification reaches the caller as 422. The
// struct NAME is the translation key, so renaming it here without renaming it
// in the seven catalogs leaves the message untranslated.
type InvalidTenantStatusTransitionNotification struct{ domain.DomainNotificationBase }

// PermissionAlreadyExistsNotification reaches the caller as 409 (already
// exists). The struct NAME is the translation key, so renaming it here without
// renaming it in the seven catalogs leaves the message untranslated.
type PermissionAlreadyExistsNotification struct {
	domain.DomainNotificationBase
}

func (PermissionAlreadyExistsNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticConflict
}

// PermissionKeyIsImmutableNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type PermissionKeyIsImmutableNotification struct{ domain.DomainNotificationBase }

// PermissionDescriptionEchoesKeyNotification reaches the caller as 422. The
// struct NAME is the translation key, so renaming it here without renaming it
// in the seven catalogs leaves the message untranslated.
type PermissionDescriptionEchoesKeyNotification struct{ domain.DomainNotificationBase }

// RoleKeyAlreadyExistsNotification reaches the caller as 409 (already exists).
// The struct NAME is the translation key, so renaming it here without renaming
// it in the seven catalogs leaves the message untranslated.
type RoleKeyAlreadyExistsNotification struct {
	domain.DomainNotificationBase
}

func (RoleKeyAlreadyExistsNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticConflict
}

// RoleKeyIsImmutableNotification reaches the caller as 422. The struct NAME is
// the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type RoleKeyIsImmutableNotification struct{ domain.DomainNotificationBase }

// RoleTenantIsImmutableNotification reaches the caller as 422. The struct NAME
// is the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type RoleTenantIsImmutableNotification struct{ domain.DomainNotificationBase }

// RoleTenantDoesNotExistNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type RoleTenantDoesNotExistNotification struct{ domain.DomainNotificationBase }

// PermissionNotInCatalogNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type PermissionNotInCatalogNotification struct{ domain.DomainNotificationBase }

// RoleAlreadyGrantsPermissionNotification reaches the caller as 409 (already
// exists). The struct NAME is the translation key, so renaming it here without
// renaming it in the seven catalogs leaves the message untranslated.
type RoleAlreadyGrantsPermissionNotification struct {
	domain.DomainNotificationBase
}

func (RoleAlreadyGrantsPermissionNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticConflict
}

// TooManyPermissionsInRoleNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated. It interpolates max into the
// message.
type TooManyPermissionsInRoleNotification struct {
	domain.DomainNotificationBase
	Max string `tvar:"max"`
}

// CannotGrantUnheldPermissionNotification reaches the caller as 403. The
// struct NAME is the translation key, so renaming it here without renaming it
// in the seven catalogs leaves the message untranslated.
type CannotGrantUnheldPermissionNotification struct {
	domain.DomainNotificationBase
}

func (CannotGrantUnheldPermissionNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticForbidden
}

// CannotGrantWildcardPermissionNotification reaches the caller as 403. The
// struct NAME is the translation key, so renaming it here without renaming it
// in the seven catalogs leaves the message untranslated.
type CannotGrantWildcardPermissionNotification struct {
	domain.DomainNotificationBase
}

func (CannotGrantWildcardPermissionNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticForbidden
}

// GroupKeyAlreadyExistsNotification reaches the caller as 409 (already
// exists). The struct NAME is the translation key, so renaming it here without
// renaming it in the seven catalogs leaves the message untranslated.
type GroupKeyAlreadyExistsNotification struct {
	domain.DomainNotificationBase
}

func (GroupKeyAlreadyExistsNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticConflict
}

// GroupKeyIsImmutableNotification reaches the caller as 422. The struct NAME
// is the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type GroupKeyIsImmutableNotification struct{ domain.DomainNotificationBase }

// GroupTenantIsImmutableNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type GroupTenantIsImmutableNotification struct{ domain.DomainNotificationBase }

// GroupTenantDoesNotExistNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type GroupTenantDoesNotExistNotification struct{ domain.DomainNotificationBase }

// RoleNotAvailableInTenantNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type RoleNotAvailableInTenantNotification struct{ domain.DomainNotificationBase }

// GroupAlreadyGrantsRoleNotification reaches the caller as 409 (already
// exists). The struct NAME is the translation key, so renaming it here without
// renaming it in the seven catalogs leaves the message untranslated.
type GroupAlreadyGrantsRoleNotification struct {
	domain.DomainNotificationBase
}

func (GroupAlreadyGrantsRoleNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticConflict
}

// TooManyRolesInGroupNotification reaches the caller as 422. The struct NAME
// is the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated. It interpolates max into the
// message.
type TooManyRolesInGroupNotification struct {
	domain.DomainNotificationBase
	Max string `tvar:"max"`
}

// CannotGrantRoleWithUnheldPermissionsNotification reaches the caller as 403.
// The struct NAME is the translation key, so renaming it here without renaming
// it in the seven catalogs leaves the message untranslated.
type CannotGrantRoleWithUnheldPermissionsNotification struct {
	domain.DomainNotificationBase
}

func (CannotGrantRoleWithUnheldPermissionsNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticForbidden
}

// CannotGrantWildcardRoleNotification reaches the caller as 403. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type CannotGrantWildcardRoleNotification struct {
	domain.DomainNotificationBase
}

func (CannotGrantWildcardRoleNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticForbidden
}

// UserEmailAlreadyExistsNotification reaches the caller as 409 (already
// exists). The struct NAME is the translation key, so renaming it here without
// renaming it in the seven catalogs leaves the message untranslated.
type UserEmailAlreadyExistsNotification struct {
	domain.DomainNotificationBase
}

func (UserEmailAlreadyExistsNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticConflict
}

// UserEmailIsImmutableNotification reaches the caller as 422. The struct NAME
// is the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type UserEmailIsImmutableNotification struct{ domain.DomainNotificationBase }

// UserTenantIsImmutableNotification reaches the caller as 422. The struct NAME
// is the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type UserTenantIsImmutableNotification struct{ domain.DomainNotificationBase }

// UserTenantDoesNotExistNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type UserTenantDoesNotExistNotification struct{ domain.DomainNotificationBase }

// PasswordConfirmationMismatchNotification reaches the caller as 422. The
// struct NAME is the translation key, so renaming it here without renaming it
// in the seven catalogs leaves the message untranslated.
type PasswordConfirmationMismatchNotification struct{ domain.DomainNotificationBase }

// PasswordEchoesIdentityNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type PasswordEchoesIdentityNotification struct{ domain.DomainNotificationBase }

// InvalidUserStatusTransitionNotification reaches the caller as 409 (wrong
// state). The struct NAME is the translation key, so renaming it here without
// renaming it in the seven catalogs leaves the message untranslated.
type InvalidUserStatusTransitionNotification struct {
	domain.DomainNotificationBase
}

func (InvalidUserStatusTransitionNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticStateConflict
}

// GroupNotAvailableInTenantNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type GroupNotAvailableInTenantNotification struct{ domain.DomainNotificationBase }

// UserAlreadyInGroupNotification reaches the caller as 409 (already exists).
// The struct NAME is the translation key, so renaming it here without renaming
// it in the seven catalogs leaves the message untranslated.
type UserAlreadyInGroupNotification struct {
	domain.DomainNotificationBase
}

func (UserAlreadyInGroupNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticConflict
}

// UserAlreadyGrantsRoleNotification reaches the caller as 409 (already
// exists). The struct NAME is the translation key, so renaming it here without
// renaming it in the seven catalogs leaves the message untranslated.
type UserAlreadyGrantsRoleNotification struct {
	domain.DomainNotificationBase
}

func (UserAlreadyGrantsRoleNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticConflict
}

// TooManyGroupsForUserNotification reaches the caller as 422. The struct NAME
// is the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated. It interpolates max into the
// message.
type TooManyGroupsForUserNotification struct {
	domain.DomainNotificationBase
	Max string `tvar:"max"`
}

// TooManyRolesForUserNotification reaches the caller as 422. The struct NAME
// is the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated. It interpolates max into the
// message.
type TooManyRolesForUserNotification struct {
	domain.DomainNotificationBase
	Max string `tvar:"max"`
}

// CannotJoinGroupWithUnheldPermissionsNotification reaches the caller as 403.
// The struct NAME is the translation key, so renaming it here without renaming
// it in the seven catalogs leaves the message untranslated.
type CannotJoinGroupWithUnheldPermissionsNotification struct {
	domain.DomainNotificationBase
}

func (CannotJoinGroupWithUnheldPermissionsNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticForbidden
}

// CannotJoinWildcardGroupNotification reaches the caller as 403. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type CannotJoinWildcardGroupNotification struct {
	domain.DomainNotificationBase
}

func (CannotJoinWildcardGroupNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticForbidden
}

// InvalidCredentialsNotification reaches the caller as 403. The struct NAME is
// the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type InvalidCredentialsNotification struct {
	domain.DomainNotificationBase
}

func (InvalidCredentialsNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticForbidden
}

// PasswordUnchangedNotification reaches the caller as 422. The struct NAME is
// the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type PasswordUnchangedNotification struct{ domain.DomainNotificationBase }
