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

// TenantIDAlreadyExistsNotification reaches the caller as 409 (already
// exists). The struct NAME is the translation key, so renaming it here without
// renaming it in the seven catalogs leaves the message untranslated.
type TenantIDAlreadyExistsNotification struct {
	domain.DomainNotificationBase
}

func (TenantIDAlreadyExistsNotification) Semantic() domain.NotificationSemantic {
	return domain.SemanticConflict
}

// TenantWorkspaceIsImmutableNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type TenantWorkspaceIsImmutableNotification struct{ domain.DomainNotificationBase }

// TenantIDIsImmutableNotification reaches the caller as 422. The struct NAME
// is the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type TenantIDIsImmutableNotification struct{ domain.DomainNotificationBase }

// TenantIDDerivationMismatchNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type TenantIDDerivationMismatchNotification struct{ domain.DomainNotificationBase }

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
