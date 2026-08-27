-- Hand-written. No generator declares this table, and none should: a refresh
-- token is not a business aggregate.
--
-- It backs authcore.RefreshTokenStore, a FRAMEWORK PORT. The Issuer owns the
-- security-critical half — opaque values, single use, rotation on every
-- redemption, family revocation on reuse — and delegates only storage. So this
-- table has no revision, no archive stamp, no audit trail and no REST surface,
-- and the absence of each is deliberate:
--
--   * no revision: nothing here is edited by a caller, so there is no lost-update
--     race for an optimistic guard to catch;
--   * no deleted_at: an expired token is deleted outright, because keeping the
--     hash of a dead credential earns nothing and costs a growing table;
--   * no REST surface: an endpoint listing refresh tokens would be a credential
--     exfiltration endpoint, whatever permission guarded it.
--
-- THE RAW TOKEN NEVER REACHES THIS TABLE. Only the SHA-256 hash crosses the port,
-- which is what makes a database leak survivable: the hashes are useless without
-- the values, and the values exist only in the clients that hold them.

CREATE TABLE "authentication_refresh_tokens" (
  -- The SHA-256 hash of the opaque value, hex-encoded — 64 characters, always.
  -- It is the PRIMARY KEY rather than a surrogate id because lookup is BY hash on
  -- every redemption, and a second unique column would only be a slower path to
  -- the same row.
  "hash" CHAR(64) NOT NULL,

  -- All tokens descended from ONE login. Reuse of an already-redeemed token
  -- revokes the whole family — the standard signal that a stolen token was
  -- replayed, and the reason a compromised session dies entirely instead of
  -- racing its legitimate owner for the next rotation.
  "family_id" VARCHAR(64) NOT NULL,

  -- The user this token authenticates, as a string: it is whatever the Issuer was
  -- handed as TokenRequest.Subject, and this table is not the place to re-assert
  -- that it is a users.id. NO FOREIGN KEY for the same reason — the port is
  -- framework-owned and knows nothing about this service's aggregates.
  "subject" VARCHAR(255) NOT NULL,

  -- The audiences the redeemed access token will carry, as a JSON array. JSON and
  -- not a native array type because this table is written by hand-rolled SQL that
  -- must render on every dialect the framework supports, and only one of them has
  -- arrays.
  "audience" TEXT NOT NULL,

  "expires_at" TIMESTAMPTZ NOT NULL,

  -- Single use. MarkUsed sets this; a second redemption of the same hash is the
  -- reuse signal, not an error to smooth over.
  "used" BOOLEAN NOT NULL DEFAULT FALSE,

  -- Set for every member of a family when reuse is detected. Distinct from `used`
  -- on purpose: `used` is the normal end of a token's life, `revoked` is a kill.
  "revoked" BOOLEAN NOT NULL DEFAULT FALSE,

  "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),

  CONSTRAINT "authentication_refresh_tokens_pkey" PRIMARY KEY ("hash")
);

COMMENT ON TABLE "authentication_refresh_tokens" IS 'Opaque refresh tokens, stored as SHA-256 hashes only. Backs the framework''s authcore.RefreshTokenStore port; the Issuer owns rotation and reuse detection, this table owns persistence.';
COMMENT ON COLUMN "authentication_refresh_tokens"."hash" IS 'SHA-256 of the opaque token value, hex-encoded. The raw value is never stored.';
COMMENT ON COLUMN "authentication_refresh_tokens"."family_id" IS 'All tokens descended from one login. A reuse revokes the entire family.';
COMMENT ON COLUMN "authentication_refresh_tokens"."subject" IS 'The authenticated subject, as handed to the Issuer. Deliberately not a foreign key: the port is framework-owned.';
COMMENT ON COLUMN "authentication_refresh_tokens"."audience" IS 'JSON array of audiences the redeemed access token carries.';
COMMENT ON COLUMN "authentication_refresh_tokens"."expires_at" IS 'When this token stops being redeemable. Rows past it are swept on redemption.';
COMMENT ON COLUMN "authentication_refresh_tokens"."used" IS 'Single-use marker set at redemption. A second redemption of a used hash is the reuse signal.';
COMMENT ON COLUMN "authentication_refresh_tokens"."revoked" IS 'Set across a whole family when reuse is detected. Distinct from used: that is a normal end of life, this is a kill.';
COMMENT ON COLUMN "authentication_refresh_tokens"."created_at" IS 'When the token was minted; written by the database default.';

-- RevokeFamily updates every row of one family, and reuse detection is the one
-- path that must be fast under attack.
CREATE INDEX "authentication_refresh_tokens_family_idx"
  ON "authentication_refresh_tokens" ("family_id");

-- THE SELF-CLEANING SWEEP RIDES THIS INDEX. Every successful redemption deletes
-- rows already past their expiry, so the table drains itself and no scheduled job
-- exists to forget to deploy. With this index that DELETE is a range scan over
-- exactly the dead rows — never a table scan — and after the first pass there is
-- almost nothing left to find.
CREATE INDEX "authentication_refresh_tokens_expires_idx"
  ON "authentication_refresh_tokens" ("expires_at");
