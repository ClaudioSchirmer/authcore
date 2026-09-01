-- THIS FILE IS YOURS. Hand-written; no generator created it and none maintains it.
--
-- The bootstrap seed: the rows a fresh database needs before anyone can sign
-- in at all.
--
-- created: 2026-09-01
--
-- WHY A MIGRATION AND NOT AN API CALL. Every write endpoint in this service
-- sits behind RequirePermission, and every permission is a row in a catalog
-- that starts empty — so on a fresh database there is no caller who can create
-- the first permission, the first tenant or the first user. The wildcard is a
-- second, stronger reason: `no-wildcard-grant` in internal/domain/role_rules_manual.go
-- refuses `*:*` on any role through the API, always, with no caller exempt.
-- This file is therefore the ONLY way a super-admin can come into existence,
-- and that is deliberate rather than a gap.
--
-- WHAT IS NOT VALIDATED HERE. These INSERTs write past the domain: no value
-- object runs, no rule fires. Every value below was chosen to satisfy those
-- rules anyway, so the seeded rows are rows the API itself would have accepted
-- — with ONE stated exception, the bootstrap password, noted at the user row.
--
-- IDS ARE FIXED LITERALS. The framework mints UUID v7 at runtime and SQL
-- cannot, so these are constants in v7 form. Fixed ids are what let the down
-- migration delete exactly these rows and nothing a human added later.
--
-- Re-running is safe: every statement ends in ON CONFLICT DO NOTHING.

-- ─────────────────────────────────────────────────────────────────────────────
-- 1. The permission catalog
--
-- The 38 resource:action pairs some route in internal/web enforces today, plus
-- the wildcard. A route whose permission is missing from this catalog can never
-- be granted to anyone, so this list and the RequirePermission calls have to
-- move together.
-- ─────────────────────────────────────────────────────────────────────────────

INSERT INTO "permissions" ("id", "resource_name", "action_name", "description") VALUES
  ('01990000-0000-7000-8000-000000000000', '*', '*', 'Unrestricted platform access: satisfies every permission check and crosses tenant scope wherever a scope is applied.'),

  ('01990000-0000-7000-8000-000000000001', 'claim', 'insert', 'Add a claim definition to the catalog of claims a token may carry.'),
  ('01990000-0000-7000-8000-000000000002', 'claim', 'update', 'Change the definition of a claim that already exists in the catalog.'),
  ('01990000-0000-7000-8000-000000000003', 'claim', 'archive', 'Retire a claim definition so it can no longer be assigned to anyone.'),
  ('01990000-0000-7000-8000-000000000004', 'claim', 'read', 'List claim definitions and open a single one by its identifier.'),

  ('01990000-0000-7000-8000-000000000005', 'client', 'insert', 'Register a machine client that authenticates with credentials of its own.'),
  ('01990000-0000-7000-8000-000000000006', 'client', 'update', 'Change the descriptive attributes of a registered machine client.'),
  ('01990000-0000-7000-8000-000000000007', 'client', 'archive', 'Retire a machine client so it can no longer obtain a token.'),
  ('01990000-0000-7000-8000-000000000008', 'client', 'read', 'List machine clients and open a single one by its identifier.'),
  ('01990000-0000-7000-8000-000000000009', 'client', 'grant', 'Attach roles to a machine client, or withdraw the ones it holds.'),
  ('01990000-0000-7000-8000-00000000000a', 'client', 'set-claim', 'Assign claim values to a machine client, change them, or withdraw them.'),
  ('01990000-0000-7000-8000-00000000000b', 'client', 'manage-network', 'Maintain the CIDR ranges a machine client is allowed to connect from.'),
  ('01990000-0000-7000-8000-00000000000c', 'client', 'rotate-secret', 'Mint a fresh secret for a machine client and invalidate the previous one.'),

  ('01990000-0000-7000-8000-00000000000d', 'group', 'insert', 'Create a group that bundles roles for the users who belong to it.'),
  ('01990000-0000-7000-8000-00000000000e', 'group', 'update', 'Change the descriptive attributes of a group that already exists.'),
  ('01990000-0000-7000-8000-00000000000f', 'group', 'archive', 'Retire a group so no user can be placed into it any longer.'),
  ('01990000-0000-7000-8000-000000000010', 'group', 'read', 'List groups and open a single one by its identifier.'),
  ('01990000-0000-7000-8000-000000000011', 'group', 'grant', 'Attach roles to a group, or withdraw the ones it confers.'),

  ('01990000-0000-7000-8000-000000000012', 'permission', 'insert', 'Add an enforceable resource and action pair to the platform catalog.'),
  ('01990000-0000-7000-8000-000000000013', 'permission', 'update', 'Change the wording that explains what a catalog entry lets a caller do.'),
  ('01990000-0000-7000-8000-000000000014', 'permission', 'archive', 'Retire a catalog entry so no role can bundle it any longer.'),
  ('01990000-0000-7000-8000-000000000015', 'permission', 'read', 'List catalog entries and open a single one by its identifier.'),

  ('01990000-0000-7000-8000-000000000016', 'role', 'insert', 'Create a role that bundles catalog permissions inside one tenant.'),
  ('01990000-0000-7000-8000-000000000017', 'role', 'update', 'Change the descriptive attributes of a role that already exists.'),
  ('01990000-0000-7000-8000-000000000018', 'role', 'archive', 'Retire a role so it can no longer be granted to anyone.'),
  ('01990000-0000-7000-8000-000000000019', 'role', 'read', 'List roles and open a single one by its identifier.'),
  ('01990000-0000-7000-8000-00000000001a', 'role', 'grant', 'Attach catalog permissions to a role, or withdraw the ones it bundles.'),

  ('01990000-0000-7000-8000-00000000001b', 'tenant', 'insert', 'Create an isolation partition of the platform with a handle of its own.'),
  ('01990000-0000-7000-8000-00000000001c', 'tenant', 'update', 'Change the descriptive attributes of a tenant that already exists.'),
  ('01990000-0000-7000-8000-00000000001d', 'tenant', 'archive', 'Archive a tenant, and restore one that was archived before.'),
  ('01990000-0000-7000-8000-00000000001e', 'tenant', 'read', 'List tenants and open a single one by its identifier.'),

  ('01990000-0000-7000-8000-00000000001f', 'user', 'insert', 'Create the account of a person inside a tenant, with its first credential.'),
  ('01990000-0000-7000-8000-000000000020', 'user', 'update', 'Change the descriptive attributes of a user account that already exists.'),
  ('01990000-0000-7000-8000-000000000021', 'user', 'archive', 'Retire a user account so the person can no longer sign in.'),
  ('01990000-0000-7000-8000-000000000022', 'user', 'read', 'List user accounts and open a single one by its identifier.'),
  ('01990000-0000-7000-8000-000000000023', 'user', 'grant', 'Attach roles and group memberships to a user, or withdraw them.'),
  ('01990000-0000-7000-8000-000000000024', 'user', 'set-claim', 'Assign claim values to a user, change them, or withdraw them.'),
  ('01990000-0000-7000-8000-000000000025', 'user', 'change-password', 'Replace the password of a user who proves the current one first.'),
  ('01990000-0000-7000-8000-000000000026', 'user', 'reset-password', 'Set a new password for a user without knowing the current one.')
ON CONFLICT DO NOTHING;

-- ─────────────────────────────────────────────────────────────────────────────
-- 2. The master tenant
--
-- `master` rather than `admin`, `platform`, `system` or `root`: all four are in
-- reservedTenantWorkspaces (internal/domain/vos/tenant_workspace.go) and the
-- workspace value object would refuse them.
-- ─────────────────────────────────────────────────────────────────────────────

INSERT INTO "tenants" ("id", "name", "workspace", "description", "status") VALUES
  ('01990000-0001-7000-8000-000000000001',
   'Master',
   'master',
   'The tenant the platform keeps for itself: home of the operator role and of the bootstrap administrator.',
   'active')
ON CONFLICT DO NOTHING;

-- ─────────────────────────────────────────────────────────────────────────────
-- 3. The master role, and the wildcard grant no API call can reproduce
-- ─────────────────────────────────────────────────────────────────────────────

INSERT INTO "roles" ("id", "tenant_id", "role_key", "name", "description") VALUES
  ('01990000-0002-7000-8000-000000000001',
   '01990000-0001-7000-8000-000000000001',
   'master',
   'Master',
   'The unrestricted operator role: it carries the wildcard permission and that permission alone.')
ON CONFLICT DO NOTHING;

INSERT INTO "role_permissions" ("id", "role_id", "permission_id") VALUES
  ('01990000-0003-7000-8000-000000000001',
   '01990000-0002-7000-8000-000000000001',
   '01990000-0000-7000-8000-000000000000')
ON CONFLICT DO NOTHING;

-- ─────────────────────────────────────────────────────────────────────────────
-- 4. The bootstrap administrator
--
-- THE PASSWORD IS `admin`, AND IT IS THE ONE VALUE HERE THE API WOULD REFUSE:
-- the Password value object (internal/domain/vos/password.go) demands at least
-- eight runes carrying a lowercase letter, an uppercase letter, a digit and a
-- symbol. Writing the hash directly is what gets past that, and it is why
-- `must_change_password` is TRUE below — the account cannot stay in this state.
--
-- The hash is an Argon2id digest of `admin`, PHC-encoded, produced by this
-- service's own hasher (internal/infra/password_hasher.go, m=19456 t=2 p=1)
-- over the fixed salt `authcore-bootstr`. Postgres cannot compute Argon2id, so
-- a literal is the only form available. It is a literal in a tracked file:
-- treat this credential as public knowledge and rotate it on first sign-in.
-- ─────────────────────────────────────────────────────────────────────────────

INSERT INTO "users" (
  "id", "tenant_id", "given_name", "family_name", "email",
  "password_hash", "password_changed_at", "must_change_password", "status"
) VALUES (
  '01990000-0004-7000-8000-000000000001',
  '01990000-0001-7000-8000-000000000001',
  'Admin',
  'Master',
  'admin@authcore.local',
  '$argon2id$v=19$m=19456,t=2,p=1$YXV0aGNvcmUtYm9vdHN0cg$eVeLgJtuuJ1PXTzhsEpGaD1PMbyQ862koRRL3Ef/q+k',
  NOW(),
  TRUE,
  'active'
)
ON CONFLICT DO NOTHING;

INSERT INTO "user_roles" ("id", "user_id", "role_id") VALUES
  ('01990000-0005-7000-8000-000000000001',
   '01990000-0004-7000-8000-000000000001',
   '01990000-0002-7000-8000-000000000001')
ON CONFLICT DO NOTHING;
