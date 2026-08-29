-- THIS FILE IS YOURS. Written by hand, never by omnicore-gen.
--
-- The user_claims collection on postgres.
--
-- entity:     User
-- spec:       specs/omnicore-gen/user.omnicore.yaml
-- plan:       specs/evolve-entity/user/spec.md (APPROVED 2026-08-28)
-- created:    2026-08-28
--
-- WHY THIS PAIR EXISTS AT ALL. The generator writes an entity's first migration
-- once and never again: a migration is the only output whose effect outlives the
-- file, because once it has run anywhere the framework records it as applied, so
-- rewriting 0005 would change what that file CLAIMS and not one table. A change
-- to the storage is therefore always a NEW numbered pair, and this is it. The
-- code for this collection came back from the spec in seconds; the table did not
-- and never will.
--
-- Purely additive: one new table, nothing altered, nothing backfilled, no data at
-- risk. Every column is NOT NULL with a value the writer always supplies, so
-- there is no default to argue about.

-- user_claims
-- LEVEL 1 of the claim chain. claims.default_value is level 2 — the tenant-wide
-- default — and this table is what a resolution reads first. It changes no token
-- today: buildClaims is untouched, by decision, so this can be filled, read and
-- audited with no observable effect on anything issued.

CREATE TABLE "user_claims" (
  "id" UUID NOT NULL,
  "user_id" UUID NOT NULL,
  "claim_id" UUID NOT NULL,
  "value" VARCHAR(256) NOT NULL,
  "deleted_at" TIMESTAMPTZ NULL,
  "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT "user_claims_pkey" PRIMARY KEY ("id"),
  CONSTRAINT "user_claims_parent_fk" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE
);
COMMENT ON TABLE "user_claims" IS 'The claim values set on this user directly — level 1 of the two-level chain, read before the definition''s tenant-wide default. One row per definition, holding the definition''s id and the value. The claims collection of users.';
COMMENT ON COLUMN "user_claims"."id" IS 'Row id — a UUID v7 minted by the framework, not a sequence.';
COMMENT ON COLUMN "user_claims"."user_id" IS 'The users row this entry belongs to.';
COMMENT ON COLUMN "user_claims"."claim_id" IS 'The claim definition this value belongs to — the id and not the name, so a retired-and-recreated definition needs an explicit re-set rather than silently inheriting a value written for the old one.';
COMMENT ON COLUMN "user_claims"."value" IS 'The value this user carries for the definition above. Validated against the definition''s declared value_type by a domain rule, since no value object can read another aggregate''s field.';
COMMENT ON COLUMN "user_claims"."deleted_at" IS 'Archive stamp; a non-null value hides the entry from reads.';
COMMENT ON COLUMN "user_claims"."created_at" IS 'When the entry was created; written by the database default.';
COMMENT ON COLUMN "user_claims"."updated_at" IS 'When the entry was last written, maintained by the framework.';

-- NO revision column, deliberately. A collection entry is not an entity: the
-- optimistic-concurrency stamp lives on users and guards the aggregate as a
-- whole. user_groups, user_roles, client_roles and client_allowed_cidrs all agree.

-- every read of the aggregate loads this collection by the key below.
CREATE INDEX "user_claims_parent_idx" ON "user_claims" ("user_id");

-- the repository binds this constraint's violation to a clean 409.
-- Scoped by user_id, the owner: an entry has no identity outside its
-- collection, and the same value under a different owner is a
-- different, legitimate row. This is the backstop businessIdentity
-- cannot be: that check sees ONE write, never the concurrent one.
-- Scoped to the ACTIVE rows: an archived row releases the value, so it
-- can be taken again while the old row stays as history.
CREATE UNIQUE INDEX "user_claims_user_id_claim_id_key" ON "user_claims" ("user_id", "claim_id") WHERE "deleted_at" IS NULL;

-- ---------------------------------------------------------------------------
-- HAND-WRITTEN BELOW THIS LINE, in the sense that even on the generated path it
-- would be: a reference to ANOTHER aggregate is outside the spec language, so
-- the foreign key into claims and its reverse index are never emitted. The same
-- append 0005 and 0008 both carry for their own collections.
-- ---------------------------------------------------------------------------

-- user_claims.claim_id targets claims.id — the definition's primary key, and the
-- reason the entry stores an id rather than the claim's name: a retired-and-
-- recreated definition is a NEW row with a NEW id, so a value written for the old
-- one cannot silently re-attach to its replacement.
--
-- It is also what makes the repository's INNER join into Claim safe. That join is
-- declared INSIDE the collection, where an inner join drops the ENTRY rather than
-- the user — a silent hole in the array instead of a missing row — so NOT NULL
-- plus referential integrity is load-bearing here, not decoration.
--
-- NO ACTION: a claim definition is archived, never hard-deleted, so there is no
-- cascade to define — and if anyone ever tries the delete, refusing it while
-- principals still hold values for that definition is the correct answer.
ALTER TABLE "user_claims"
  ADD CONSTRAINT "user_claims_claim_fk"
  FOREIGN KEY ("claim_id") REFERENCES "claims" ("id");

-- The reverse index. Two readers need it and both matter: the read join above
-- resolves claim_id per entry on every load of a user, and Claim's own
-- ClaimIsHeldByAUser fact — the narrowing guard added in the same wave — asks
-- "does any active row here point at this definition".
CREATE INDEX "user_claims_claim_idx" ON "user_claims" ("claim_id");
