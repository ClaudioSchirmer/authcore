-- THIS FILE IS YOURS. Written once by omnicore-gen, never again.
--
-- The rollback of clients on postgres.
--
-- entity:     Client
-- spec:       specs/omnicore-gen/client.omnicore.yaml
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

-- Rollback of clients.
-- Idempotent on purpose: an up that failed halfway leaves some objects
-- created and others not, so the down must tolerate what is absent.

-- The collections first: both carry a cascading key back to clients, and
-- dropping the owner while they exist is a dependency error rather than a
-- cascade. The hand-written foreign keys into tenants and roles go with the
-- tables that carry them, so nothing here has to drop a constraint by name.
DROP TABLE IF EXISTS "client_roles";
DROP TABLE IF EXISTS "client_allowed_cidrs";
DROP TABLE IF EXISTS "clients";
