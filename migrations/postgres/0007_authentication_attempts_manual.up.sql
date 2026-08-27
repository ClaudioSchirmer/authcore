-- Hand-written. No generator declares this table, and none should: nothing here
-- is a business aggregate — nobody edits one, nothing validates one.
--
-- IT IS A ROLLUP, AND THE CEILING IS THE POINT: at most TWO rows per identity —
-- one `failure`, one `success` — enforced by a UNIQUE constraint, not by
-- convention. The table grows with the number of DISTINCT IDENTITIES ever tried,
-- never with traffic, and the lockout is answered by a single point lookup on
-- that same unique index instead of a windowed scan.
--
-- THIS REPLACES AN APPEND-ONLY LOG, and the trade was made deliberately. The
-- earlier shape wrote one row per attempt and was two things at once: the
-- lockout AND the forensic record. Only the first survives here. Every attempt
-- is still announced — the sign-in publishes one `"event"` record on the
-- service's structured stdout channel for every outcome, through the framework's
-- events.Publisher port — so "what has this IP been doing", "show me the burst at
-- 04:00" and the ordered attempt timeline are now questions for the observability
-- stack (collector → Elasticsearch/Loki → Kibana), not for SQL.
--
-- WHAT THAT COSTS, said plainly so nobody rediscovers it in an incident:
--   * Retention becomes the log pipeline's policy, not a decision taken here.
--   * The forensic half is now BEST-EFFORT. The counters below are written in the
--     request path and a failure to write them refuses the request; the log
--     record is published through a port whose errors are warned and swallowed.
--     The lockout is load-bearing and lives here; the narrative does not.
--   * Identities in clear now leave the box for the log pipeline, which usually
--     has wider read access than this database does. See the note on `identity`.
--
-- WHY THE COUNTERS ARE NOT COLUMNS ON `users` — unchanged, and still the reason
-- this table exists at all. A counter on the user row would only exist for users
-- that EXIST, so an unknown address could never lock while a real one locked on
-- the sixth attempt — an existence oracle no wording repairs, and one that shows
-- up in the response TIME too. Keying by the ATTEMPTED identity closes it: a row
-- exists for any string somebody tries, so an unknown address locks on the sixth
-- attempt exactly as a real one does, and gets the same answer.
--
-- THE ATTEMPTED PASSWORD NEVER ENTERS THIS TABLE — not in clear, not hashed, not
-- truncated, not a length. What is recorded is who somebody tried to be, never
-- what they presented. There is no column below it could go in, and that is on
-- purpose. The same rule governs the published log record.

CREATE TABLE "authentication_attempts" (
  -- Surrogate row id, and nothing more: the row is addressed by its natural key
  -- (see the UNIQUE constraint below), never by this. It exists because this
  -- project works with simple primary keys only — the same shape the framework's
  -- own upserted registries use (`omnicore_integration_failures` declares a
  -- surrogate `id` beside a `_natural_key` UNIQUE and conflicts on the latter).
  --
  -- It is minted on the INSERT branch of the upsert and never appears in the
  -- update set, so a row that already exists keeps the id it was born with.
  "id" UUID NOT NULL,

  -- WHO SOMEBODY TRIED TO BE, exactly as the sign-in normalised it (an e-mail is
  -- lower-cased and trimmed before the lookup, so the same address always counts
  -- toward the same row).
  --
  -- It is `identity` and not `email` because the same table serves the coming
  -- client-credentials route, where the attempted identity is a client id.
  --
  -- STORED IN CLEAR, and hashing was considered and rejected — twice now, for the
  -- same reason. A hash would leave a reviewer able to see that SOME identity was
  -- tried four hundred times but not which, which is most of the value gone.
  -- `users.email` already stores addresses in clear, so this is not a new class
  -- of data; what IS new is that the published log record carries it off-box.
  "identity" VARCHAR(320) NOT NULL,

  -- Which kind of subject was being claimed: 'user' today, 'client' when
  -- POST /auth/client/token lands. It is part of the natural key, so a client id
  -- that happens to spell an e-mail address counts on its own row.
  "identity_kind" VARCHAR(16) NOT NULL,

  -- Which of the two rows this is. The CHECK is what makes "at most two rows per
  -- identity" a fact about the database rather than a promise about the code:
  -- with the UNIQUE constraint below, the vocabulary IS the ceiling.
  --
  -- 'failure' — the row that carries the lockout. Every counter below except
  --             `total_count` is meaningful only here.
  -- 'success' — the lifetime count of successful sign-ins and when the last one
  --             happened. It anchors nothing: a success CLEARS the failure row's
  --             `current_count` directly, which is what "a successful sign-in
  --             resets the counter" now means.
  --
  -- THERE IS NO 'locked' VALUE ANY MORE. An attempt refused because the identity
  -- was already locked bumps `total_blocked` on the failure row (see below) and
  -- is published to the log stream; it gets no row of its own.
  "outcome" VARCHAR(16) NOT NULL,

  -- THE LIFETIME TOTAL, AND IT IS NEVER RESET BY ANYTHING. On the failure row:
  -- every failed attempt this identity has ever made. On the success row: every
  -- successful sign-in it has ever made.
  --
  -- It is deliberately independent of `current_count`: a successful sign-in and
  -- an expiring window both zero the lockout counter, and neither may erase the
  -- history. "This address has failed 4 000 times over six months" is a fact the
  -- lockout must not be able to destroy.
  "total_count" BIGINT NOT NULL DEFAULT 0,

  -- THE RESETTABLE COUNTER — this one IS the lock.
  --
  -- Failures inside the live window. It reaches zero two ways and only two: a
  -- successful sign-in clears it, or the window ages out and the next failure
  -- restarts it at 1. Nothing sweeps it, nothing schedules it.
  --
  -- FAILURE ROW ONLY. Always 0 on the success row — declared, not forgotten.
  "current_count" INTEGER NOT NULL DEFAULT 0,

  -- ATTEMPTS REFUSED WHILE ALREADY LOCKED. Lifetime, never reset.
  --
  -- It replaces what used to be a row per refusal, and it keeps the question that
  -- row existed to answer: "did somebody keep hammering this identity right
  -- through the lock?" — which is how a determined attack looks different from a
  -- user who mistyped five times and gave up.
  --
  -- BUMPING IT CANNOT EXTEND THE LOCK, and that is the property to preserve: it
  -- touches neither `current_count` nor `window_started_at`, so the expiry
  -- derived from them is unmoved. If attempts made during a lock extended it,
  -- anyone could hold somebody else's account shut forever just by continuing to
  -- try.
  --
  -- FAILURE ROW ONLY. Always 0 on the success row.
  "total_blocked" BIGINT NOT NULL DEFAULT 0,

  -- WHEN THE LIVE WINDOW OPENED — the moment `current_count` last went 0 → 1.
  --
  -- THE LOCK'S EXPIRY IS DERIVED FROM IT AND IS NEVER STORED: the lock ends at
  -- `window_started_at` + the window, which is the same moment the window ages
  -- out and the count stops applying. Nothing has to be cleared, and no stored
  -- expiry can drift from the data it came from.
  --
  -- NULL IN TWO STATES, AND CONFLATING THEM IS A TRAP THAT ALREADY BIT ONCE:
  -- before the first failure, and after a SUCCESSFUL SIGN-IN, which clears the
  -- counter and closes the window by writing NULL here.
  --
  -- Whatever re-opens a burst must therefore treat "NULL" and "aged out" as the
  -- same condition. A predicate that only compares (`window_started_at <= x`)
  -- silently skips the NULL rows, because `NULL <= x` is NULL and not TRUE — and
  -- since no upsert writes this column on conflict, nothing else would ever set
  -- it again. The failure counter then climbs forever against a NULL anchor, no
  -- expiry can be derived from it, and every identity that has ever signed in
  -- successfully becomes PERMANENTLY UNLOCKABLE.
  --
  -- FAILURE ROW ONLY; always NULL on the success row.
  "window_started_at" TIMESTAMPTZ NULL,

  -- The most recent attempt of THIS row's kind. On the failure row: the last
  -- failure. On the success row: WHEN THIS ACCOUNT LAST SIGNED IN SUCCESSFULLY —
  -- a fact worth having on its own, and one the append-only shape could only
  -- produce with a sort over the whole identity's history.
  "last_at" TIMESTAMPTZ NOT NULL,

  -- Where the most recent attempt of this row's kind came from. 45 characters
  -- holds an IPv6 address in full, including an IPv4-mapped form. Empty when the
  -- request carried nothing usable rather than NULL, so a report never has to
  -- distinguish "no IP" from "unknown IP".
  --
  -- IT IS THE LAST ONE, NOT THE SET. A rollup row cannot show one IP walking a
  -- hundred identities — that question moved to the log stream with the rest of
  -- the forensic record. What it still answers is "where did this identity's most
  -- recent attempt come from", which is what an operator looking at one locked
  -- account asks first.
  "last_ip" VARCHAR(45) NOT NULL DEFAULT '',

  -- Whether the identity NAMED AN ACTUAL ACCOUNT the last time anything
  -- established it.
  --
  -- It costs nothing — the sign-in has already performed that lookup — and it
  -- opens no oracle, because it is a column an attacker cannot read while the
  -- response stays uniform. What it buys is the distinction that decides urgency:
  -- a locked row with FALSE is credential stuffing from a leaked list and is
  -- noise; a locked row with TRUE is a real account under attack and those people
  -- should be told.
  --
  -- NULLABLE, and the null is not laziness: when nothing established the answer —
  -- a lookup that itself failed — the column keeps whatever was last established
  -- rather than being overwritten with a guess. The writer omits it from the
  -- update entirely in that case, which is why a store outage cannot erase a
  -- verdict earned before it.
  --
  -- Always TRUE on the success row: a credential cannot verify against an account
  -- that is not there, so the value is implied by the outcome rather than told.
  "identity_existed" BOOLEAN NULL,

  CONSTRAINT "authentication_attempts_pkey" PRIMARY KEY ("id"),

  -- THE CEILING, AS A CONSTRAINT. Two rows per (identity, kind) is now something
  -- the database refuses to violate, not something the application remembers to
  -- respect.
  --
  -- It is also the index every request-path read uses and the key every write
  -- conflicts on: the lockout probe is a point lookup on all three columns, and
  -- the recording upsert names exactly this triple as its conflict target. One
  -- index, both jobs.
  CONSTRAINT "authentication_attempts_natural_key"
    UNIQUE ("identity", "identity_kind", "outcome"),

  CONSTRAINT "authentication_attempts_outcome_check"
    CHECK ("outcome" IN ('failure', 'success')),

  -- Counters count. A negative one means something wrote a bare value instead of
  -- an increment, and it should fail where it happened rather than quietly make
  -- an account unlockable.
  CONSTRAINT "authentication_attempts_counts_non_negative"
    CHECK ("total_count" >= 0 AND "current_count" >= 0 AND "total_blocked" >= 0)
);

COMMENT ON TABLE "authentication_attempts" IS 'Rollup of authentication attempts: at most two rows per identity (one failure, one success). Holds the brute-force lockout counters and the lifetime totals. The per-attempt forensic record lives in the service log stream, not here. The attempted password is never stored, in any form.';
COMMENT ON COLUMN "authentication_attempts"."id" IS 'Surrogate row id. The row is addressed by its natural key; this is never used to find it, and it survives every upsert.';
COMMENT ON COLUMN "authentication_attempts"."identity" IS 'Who somebody tried to be, normalised and in clear. An e-mail today, a client id when the client-credentials route lands.';
COMMENT ON COLUMN "authentication_attempts"."identity_kind" IS 'Which kind of subject was claimed: user or client. Part of the natural key.';
COMMENT ON COLUMN "authentication_attempts"."outcome" IS 'Which of the two rows this is: failure (carries the lockout) or success (lifetime sign-in count and last sign-in). There is no locked row — a refusal while locked bumps total_blocked.';
COMMENT ON COLUMN "authentication_attempts"."total_count" IS 'Lifetime count for this row kind. NEVER reset — neither a successful sign-in nor an expiring window may erase history.';
COMMENT ON COLUMN "authentication_attempts"."current_count" IS 'Failures inside the live lockout window — the resettable counter that IS the lock. Cleared by a successful sign-in or by the window ageing out. Failure row only; 0 on success.';
COMMENT ON COLUMN "authentication_attempts"."total_blocked" IS 'Lifetime attempts refused because the identity was already locked. Bumping it cannot extend the lock — it touches neither current_count nor window_started_at. Failure row only; 0 on success.';
COMMENT ON COLUMN "authentication_attempts"."window_started_at" IS 'When the live lockout window opened. The expiry is DERIVED from it (window_started_at + the window) and never stored. Failure row only; NULL on success.';
COMMENT ON COLUMN "authentication_attempts"."last_at" IS 'Most recent attempt of this row kind. On the success row, this is when the account last signed in successfully.';
COMMENT ON COLUMN "authentication_attempts"."last_ip" IS 'Origin of the most recent attempt of this row kind; empty when the request carried nothing usable. The LAST one, not the set — cross-IP patterns live in the log stream.';
COMMENT ON COLUMN "authentication_attempts"."identity_existed" IS 'Whether the identity named a real account the last time anything established it. Left untouched (never overwritten with a guess) when a lookup failed. Separates a targeted attack from credential stuffing.';

-- NO OTHER INDEX, and that is a decision rather than an omission.
--
-- The only query on the request path is the point lookup the natural key already
-- serves. The operator's question — "which identities are locked right now?" — is
-- a scan, and it is affordable precisely because of this migration: the table is
-- now bounded by distinct identities ever tried rather than by attempts, so the
-- thing that used to make a scan unthinkable is gone. Add an index when a real
-- report proves it needs one, not before.
