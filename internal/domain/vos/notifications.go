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

package vos

import "github.com/ClaudioSchirmer/omnicore/domain"

// UnknownTenantStatusNotification reaches the caller as 422. The struct NAME
// is the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type UnknownTenantStatusNotification struct{ domain.DomainNotificationBase }

// InvalidDisplayNameNotification reaches the caller as 422. The struct NAME is
// the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type InvalidDisplayNameNotification struct{ domain.DomainNotificationBase }

// InvalidDescriptionNotification reaches the caller as 422. The struct NAME is
// the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type InvalidDescriptionNotification struct{ domain.DomainNotificationBase }

// InvalidTenantWorkspaceNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type InvalidTenantWorkspaceNotification struct{ domain.DomainNotificationBase }

// ReservedTenantWorkspaceNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type ReservedTenantWorkspaceNotification struct{ domain.DomainNotificationBase }

// InvalidResourceNameNotification reaches the caller as 422. The struct NAME
// is the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type InvalidResourceNameNotification struct{ domain.DomainNotificationBase }

// InvalidActionNameNotification reaches the caller as 422. The struct NAME is
// the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type InvalidActionNameNotification struct{ domain.DomainNotificationBase }

// UnmatchablePermissionKeyNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type UnmatchablePermissionKeyNotification struct{ domain.DomainNotificationBase }
