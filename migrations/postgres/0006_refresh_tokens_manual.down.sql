-- The reverse of 0006_refresh_tokens_manual.up.sql.
--
-- Dropping the table takes every live refresh token with it: after this runs,
-- every client holding one is logged out at its next rotation and has to sign in
-- again. That is the correct behaviour for a rollback — the alternative would be
-- orphaned hashes nothing can redeem — but it is a user-visible consequence and
-- not a silent one, so it is written down here.
--
-- The two indexes go with the table; naming them would be noise.
DROP TABLE IF EXISTS "authentication_refresh_tokens";
