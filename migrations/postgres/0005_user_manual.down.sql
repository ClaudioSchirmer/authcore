-- THIS FILE IS YOURS. Written once by omnicore-gen, never again.
--
-- The rollback of users on postgres.
--
-- entity:     User
-- spec:       specs/omnicore-gen/user.omnicore.yaml
-- generator:  omnicore-gen (created this file, does not maintain it)
-- created:    2026-08-26
--
-- This pair was created once and is now yours. The generator will not
-- rewrite it, and you should not either once it has run anywhere: the
-- framework records an applied migration in its tracking table, so editing
-- the file changes the file and not the database. To change the shape, add a
-- NEW numbered pair in this folder — and remember that adding a NOT NULL
-- column to a table that already has rows needs a default, and that a rename
-- done as drop-then-add takes the data with it.
--
-- There is no checksum here on purpose: this file exists to be edited, so
-- hashing it would report drift every time you did the thing it is for.

-- Rollback of users.
-- Idempotent on purpose: an up that failed halfway leaves some objects
-- created and others not, so the down must tolerate what is absent.

-- The hand-written half, reversed, and it runs BEFORE the table drops below.
-- Dropping the tables would take these with them, so the order is not what
-- makes the rollback work — it is what makes it a true inverse rather than one
-- that happens to succeed because of a cascade. A reader comparing the two
-- files sees each statement undone by its counterpart, in reverse order.
DROP INDEX IF EXISTS "user_roles_role_idx";
DROP INDEX IF EXISTS "user_groups_group_idx";
DROP INDEX IF EXISTS "users_tenant_idx";
ALTER TABLE IF EXISTS "user_roles" DROP CONSTRAINT IF EXISTS "user_roles_role_fk";
ALTER TABLE IF EXISTS "user_groups" DROP CONSTRAINT IF EXISTS "user_groups_group_fk";
ALTER TABLE IF EXISTS "users" DROP CONSTRAINT IF EXISTS "users_tenant_fk";

DROP TABLE IF EXISTS "user_groups";
DROP TABLE IF EXISTS "user_roles";
DROP TABLE IF EXISTS "users";
