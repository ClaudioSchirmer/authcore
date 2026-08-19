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
