-- THIS FILE IS YOURS. Written by hand, never by omnicore-gen.
--
-- The rollback of user_claims on postgres.
--
-- entity:     User
-- spec:       specs/omnicore-gen/user.omnicore.yaml
-- plan:       specs/evolve-entity/user/spec.md (APPROVED 2026-08-28)
-- created:    2026-08-28
--
-- Structure only. Dropping the table takes every value any user held with it, and
-- a later re-up brings back an empty table — said plainly rather than implied,
-- though the exposure here is as small as it gets: the table is new in this pair,
-- so a down can only lose what was written after it ran.

-- Rollback of user_claims.
-- Idempotent on purpose: an up that failed halfway leaves some objects
-- created and others not, so the down must tolerate what is absent.

DROP TABLE IF EXISTS "user_claims";
