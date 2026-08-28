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

// InvalidRoleKeyNotification reaches the caller as 422. The struct NAME is the
// translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type InvalidRoleKeyNotification struct{ domain.DomainNotificationBase }

// InvalidGroupKeyNotification reaches the caller as 422. The struct NAME is
// the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type InvalidGroupKeyNotification struct{ domain.DomainNotificationBase }

// InvalidEmailNotification reaches the caller as 422. The struct NAME is the
// translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type InvalidEmailNotification struct{ domain.DomainNotificationBase }

// InvalidPersonNameNotification reaches the caller as 422. The struct NAME is
// the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type InvalidPersonNameNotification struct{ domain.DomainNotificationBase }

// WeakPasswordNotification reaches the caller as 422. The struct NAME is the
// translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type WeakPasswordNotification struct{ domain.DomainNotificationBase }

// UnknownUserStatusNotification reaches the caller as 422. The struct NAME is
// the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type UnknownUserStatusNotification struct{ domain.DomainNotificationBase }

// InvalidCIDRBlockNotification reaches the caller as 422. The struct NAME is
// the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type InvalidCIDRBlockNotification struct{ domain.DomainNotificationBase }

// UniversalCIDRNotAllowedNotification reaches the caller as 422. The struct
// NAME is the translation key, so renaming it here without renaming it in the
// seven catalogs leaves the message untranslated.
type UniversalCIDRNotAllowedNotification struct{ domain.DomainNotificationBase }

// UnknownClientStatusNotification reaches the caller as 422. The struct NAME
// is the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type UnknownClientStatusNotification struct{ domain.DomainNotificationBase }

// CIDRHasHostBitsSetNotification reaches the caller as 422. The struct NAME is
// the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type CIDRHasHostBitsSetNotification struct{ domain.DomainNotificationBase }

// InvalidClaimNameNotification reaches the caller as 422. The struct NAME is
// the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type InvalidClaimNameNotification struct{ domain.DomainNotificationBase }

// UnknownClaimValueTypeNotification reaches the caller as 422. The struct NAME
// is the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type UnknownClaimValueTypeNotification struct{ domain.DomainNotificationBase }

// UnknownClaimAppliesToNotification reaches the caller as 422. The struct NAME
// is the translation key, so renaming it here without renaming it in the seven
// catalogs leaves the message untranslated.
type UnknownClaimAppliesToNotification struct{ domain.DomainNotificationBase }
