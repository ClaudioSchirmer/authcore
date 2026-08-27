-- Hand-written. No generator declares this table, and none should: an
-- authentication attempt is not a business aggregate — nobody edits one, nothing
-- validates one, and it is written exactly once and then only read.
--
-- IT IS TWO THINGS AT ONCE, and both are why it exists:
--
--   1. THE LOCKOUT. The sign-in counts recent failures for one identity and
--      refuses when there are too many. The count is a query over this table, not
--      a stored counter, and that difference is deliberate — see below.
--   2. THE FORENSIC RECORD. A security reviewer reads it to find the pattern: one
--      identity hammered, one IP spraying a hundred addresses, a burst at 04:00.
--      That is the product, not a by-product.
--
-- WHY THE COUNTER IS NOT A COLUMN ON `users`. That was the approved design and it
-- was wrong. A counter on the user row only exists for users that EXIST, so an
-- unknown address could never lock while a real one locked on the sixth attempt —
-- and that difference in behaviour is an existence oracle no wording repairs. It
-- shows up in the response TIME too: a locked account is refused without paying
-- Argon2id while an unknown address keeps costing a full verification, reopening
-- the exact timing oracle the sign-in was built to close. Keying by the ATTEMPTED
-- identity closes it: a row exists for any string somebody tries, so an unknown
-- address locks on the sixth attempt too and gets the same answer.
--
-- APPEND-ONLY, and that buys three things a counter column could not. There is no
-- UPDATE, so parallel failed attempts against one identity cannot collide on a
-- revision guard — which on the security path would have meant 409s. The lockout
-- becomes a windowed COUNT rather than a number somebody has to maintain. And the
-- auto-release is free: the window slides, and nothing has to clear anything.
--
-- IT IS NOT SWEPT. The refresh-token table deletes its own dead rows because a
-- dead hash earns nothing; this one keeps everything, because the failures ARE the
-- evidence and a table that pruned itself would prune the attack it exists to
-- show. RETENTION IS THE MAINTAINER'S POLICY CALL — nothing here expires a row,
-- and no rollup shadows this data to outlive a deletion they chose to perform.
--
-- TWO RULES THAT FOLLOW FROM KEEPING IT FOREVER:
--
--   * THE ATTEMPTED PASSWORD NEVER ENTERS THIS TABLE — not in clear, not hashed,
--     not truncated, not a length. What is recorded is who somebody tried to be,
--     never what they presented. There is no column below it could go in, and
--     that is on purpose.
--   * The identity is stored IN CLEAR. Hashing was considered and rejected: it
--     would leave a reviewer able to see that SOME identity was tried four hundred
--     times but not which, which is most of the value gone. `users.email` already
--     stores addresses in clear, so this is not a new class of data — but it does
--     accumulate identifiers of people who never registered, which is the other
--     reason retention is a decision somebody has to take.

CREATE TABLE "authentication_attempts" (
  "id" UUID NOT NULL,

  -- WHO SOMEBODY TRIED TO BE, exactly as the sign-in normalised it (an e-mail is
  -- lower-cased and trimmed before the lookup, so the same address always counts
  -- toward the same window).
  --
  -- It is `identity` and not `email` because the same table serves the coming
  -- client-credentials route, where the attempted identity is a client id.
  "identity" VARCHAR(320) NOT NULL,

  -- Which kind of subject was being claimed: 'user' today, 'client' when
  -- POST /auth/client/token lands. It keeps the two populations separable in a
  -- report without a second table.
  "identity_kind" VARCHAR(16) NOT NULL,

  -- 'failure' — a credential was presented and rejected. ONLY THIS ONE COUNTS
  --             toward the lockout.
  -- 'success' — the credential verified. It also ANCHORS the window: failures
  --             before the most recent success are not counted, which is how "a
  --             successful sign-in clears the counter" works without an update.
  -- 'locked'  — refused because the identity was already locked. Recorded so a
  --             reviewer sees the persistence, and deliberately NOT counted: if
  --             attempts made during a lock extended it, anyone could hold
  --             somebody else's account shut forever just by continuing to try.
  "outcome" VARCHAR(16) NOT NULL,

  -- Whether the identity NAMED AN ACTUAL ACCOUNT at the moment it was tried.
  --
  -- It costs nothing — the sign-in has already performed that lookup — and it
  -- opens no oracle, because it is a column an attacker cannot read while the
  -- response stays uniform. What it buys is the distinction that decides urgency:
  -- hundreds of attempts all FALSE is credential stuffing from a leaked list and
  -- is noise; a few dozen against three identities all TRUE is a targeted attack
  -- and those people should be told. The RATIO is sharper still — an attacker
  -- hitting real accounts far above chance is an attacker holding a valid user
  -- list, which means an enumeration leak already happened somewhere else.
  --
  -- IT IS FILLED ON A `locked` ROW TOO, and that is the reason the column earns
  -- its place rather than merely having one. A lock is decided BEFORE any lookup —
  -- that is the economy of a lockout — so that path never asks the question
  -- itself; instead the verdict already reached by the failures that CAUSED the
  -- lock rides back from the same query that decided it, and is stamped here.
  -- Without that, somebody filtering `WHERE outcome = 'locked'` — which is
  -- exactly how you look for accounts under attack — would see a column of blanks
  -- and could not tell a real account being hammered from noise.
  --
  -- NULLABLE, and the null is not laziness: when nothing established the answer —
  -- a lookup that failed, or a lock whose failures were all recorded during an
  -- outage — the column stays empty. It never guesses.
  --
  -- PAST TENSE on purpose. It records what was true at that moment; an account
  -- created or archived afterwards does not rewrite history.
  "identity_existed" BOOLEAN NULL,

  -- Where it came from. 45 characters holds an IPv6 address in full, including an
  -- IPv4-mapped form. Empty when the request carried nothing usable rather than
  -- NULL, so a report never has to distinguish "no IP" from "unknown IP".
  --
  -- This is the second dimension a spray attack is visible in: one identity from
  -- a hundred IPs is a distributed guess at one account; a hundred identities
  -- from one IP is a list being walked.
  "ip" VARCHAR(45) NOT NULL DEFAULT '',

  "occurred_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),

  CONSTRAINT "authentication_attempts_pkey" PRIMARY KEY ("id")
);

COMMENT ON TABLE "authentication_attempts" IS 'Every authentication attempt, append-only and never swept: it is both the lockout''s counter and the forensic record of who has been trying to get in. The attempted password is never stored, in any form.';
COMMENT ON COLUMN "authentication_attempts"."identity" IS 'Who somebody tried to be, normalised. An e-mail today, a client id when the client-credentials route lands.';
COMMENT ON COLUMN "authentication_attempts"."identity_kind" IS 'Which kind of subject was claimed: user or client.';
COMMENT ON COLUMN "authentication_attempts"."outcome" IS 'failure (counts toward the lockout) · success (anchors the window) · locked (recorded, never counted, so a lock cannot be extended indefinitely by an attacker).';
COMMENT ON COLUMN "authentication_attempts"."identity_existed" IS 'Whether the identity named a real account at that moment. NULL when the lookup itself failed and the answer is genuinely unknown. Separates a targeted attack from credential stuffing.';
COMMENT ON COLUMN "authentication_attempts"."ip" IS 'Origin address; empty when the request carried nothing usable. The dimension a spray attack is visible in.';
COMMENT ON COLUMN "authentication_attempts"."occurred_at" IS 'When the attempt happened. The lockout window slides over this column.';

-- THE LOCKOUT QUERY RIDES THIS INDEX, and the column order is the whole point:
-- identity first because the question is always "for THIS identity", occurred_at
-- second because the answer is always "in the last N minutes". A reversed pair
-- would scan every recent attempt in the system to answer one account's question.
CREATE INDEX "authentication_attempts_identity_time_idx"
  ON "authentication_attempts" ("identity", "occurred_at" DESC);

-- The reverse question, and nothing on the request path asks it: "what has this
-- address been doing?" is what a reviewer asks when a spray is suspected, and
-- without this index it is a full scan of a table that never shrinks.
CREATE INDEX "authentication_attempts_ip_time_idx"
  ON "authentication_attempts" ("ip", "occurred_at" DESC);
