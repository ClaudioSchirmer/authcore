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

// PasswordUnchangedNotification reaches the caller as 422. The struct NAME is
// the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type PasswordUnchangedNotification struct{ domain.DomainNotificationBase }

// PasswordChangeRequiresSelfNotification reaches the caller as 403. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
//
// It is the change endpoint's row decision: the permission on the route says
// WHO may attempt the verb, and this says WHOSE row they reached. A caller who
// holds user:change-password and points it at somebody else meets this — the
// reset is the operation for that, and it asks for a different permission.
type PasswordChangeRequiresSelfNotification struct {
	domain.DomainNotificationBase
}

func (PasswordChangeRequiresSelfNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticForbidden
}

// PasswordResetRequiresAnotherUserNotification reaches the caller as 403. The
// struct NAME is the translation key, so renaming it here without renaming it
// in the seven catalogs leaves the message untranslated.
//
// It is the OTHER half, and it is not symmetry for its own sake. Without it a
// holder of user:reset-password could point the reset at their OWN row and
// replace their credential without proving the previous one — which is exactly
// what the change endpoint's currentPassword exists to stop, defeated by
// choosing the other URL. It keeps the two operations disjoint: same id is
// always the change, different id is always the reset.
type PasswordResetRequiresAnotherUserNotification struct {
	domain.DomainNotificationBase
}

func (PasswordResetRequiresAnotherUserNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticForbidden
}

// InvalidCurrentPasswordNotification reaches the caller as 422. The struct NAME
// is the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
//
// 422 and not 403: the caller is who they say they are and may perform the
// verb — they mistyped a field. The 403s above are about which ROW was reached,
// which is a different refusal and deserves a different status.
type InvalidCurrentPasswordNotification struct {
	domain.DomainNotificationBase
}

// ClientNameAlreadyExistsNotification reaches the caller as 409 (already
// exists). The struct NAME is the translation key, so renaming it here without
// renaming it in the seven catalogs leaves the message untranslated.
type ClientNameAlreadyExistsNotification struct {
	domain.DomainNotificationBase
}

func (ClientNameAlreadyExistsNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticConflict
}

// ClientTenantIsImmutableNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type ClientTenantIsImmutableNotification struct{ domain.DomainNotificationBase }

// ClientTenantDoesNotExistNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type ClientTenantDoesNotExistNotification struct{ domain.DomainNotificationBase }

// InvalidClientStatusTransitionNotification reaches the caller as 422. The
// struct NAME is the translation key, so renaming it here without renaming it
// in the seven catalogs leaves the message untranslated.
type InvalidClientStatusTransitionNotification struct{ domain.DomainNotificationBase }

// TooManyRolesForClientNotification reaches the caller as 422. The struct NAME
// is the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated. It interpolates max into the
// message.
type TooManyRolesForClientNotification struct {
	domain.DomainNotificationBase
	Max string `tvar:"max"`
}

// ClientAlreadyGrantsRoleNotification reaches the caller as 409 (already
// exists). The struct NAME is the translation key, so renaming it here without
// renaming it in the seven catalogs leaves the message untranslated.
type ClientAlreadyGrantsRoleNotification struct {
	domain.DomainNotificationBase
}

func (ClientAlreadyGrantsRoleNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticConflict
}

// TooManyAllowedCIDRsForClientNotification reaches the caller as 422. The
// struct NAME is the translation key, so renaming it here without renaming it
// in the seven catalogs leaves the message untranslated. It interpolates max
// into the message.
type TooManyAllowedCIDRsForClientNotification struct {
	domain.DomainNotificationBase
	Max string `tvar:"max"`
}

// ClientAlreadyAllowsCIDRNotification reaches the caller as 409 (already
// exists). The struct NAME is the translation key, so renaming it here without
// renaming it in the seven catalogs leaves the message untranslated.
type ClientAlreadyAllowsCIDRNotification struct {
	domain.DomainNotificationBase
}

func (ClientAlreadyAllowsCIDRNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticConflict
}

// ClientMayOnlyModifyItselfNotification reaches the caller as 403. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type ClientMayOnlyModifyItselfNotification struct {
	domain.DomainNotificationBase
}

func (ClientMayOnlyModifyItselfNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticForbidden
}

// InvalidGracePeriodNotification reaches the caller as 422. The struct NAME is
// the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated. It interpolates max into the
// message.
type InvalidGracePeriodNotification struct {
	domain.DomainNotificationBase
	Max string `tvar:"max"`
}

// ClientMustBeActiveToRotateNotification reaches the caller as 409 (wrong
// state). The struct NAME is the translation key, so renaming it here without
// renaming it in the seven catalogs leaves the message untranslated.
type ClientMustBeActiveToRotateNotification struct {
	domain.DomainNotificationBase
}

func (ClientMustBeActiveToRotateNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticStateConflict
}
