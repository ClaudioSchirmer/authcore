# Capability plan — token-identity-kind

- **Status:** APPLIED (2026-08-27)
- **Framework pin:** `github.com/ClaudioSchirmer/omnicore v0.61.0`

## §1 The request (restated)

> podemos já incluir no token o kind ? na rota de users? Acho que está pré-descrito em specs/client
>
> da uma olhada o que precisa mudar na rota de users auth, para entrar mais para frente o token de client
>
> pode aplicar, já cria o caminho único para ambos que sitou, quando se trata das tabelas auxiliares

Follow-on to `specs/implement/authentication-attempt-counters/plan.md`, which shipped in PR #19
and left the client-credentials route explicitly unserved. This closes the two things that route
would otherwise have had to invent for itself.

## §2 What changed

Two changes made on the same branch once the maintainer asked whether the `identity_kind` claim
could be minted now, ahead of the client-credentials route.

**The claim.** `buildClaims` now mints `identity_kind: "user"`, so both token paths declare what
kind of subject they speak for. It changes **no decision today**: Client's two row rules compare
against `client` (`refuseClientCallerCreating`, `refuseForeignClientCaller`), so `"user"` and an
absent claim behave identically. What it removes is the inference — until now a user token was
recognised by the ABSENCE of the claim, which cannot distinguish a user from an issuer that
forgot, or from a token minted before the claim existed. The spec already declared it
(`specs/omnicore-gen/client.omnicore.yaml:251-260`, `claim: identity_kind`); this fills in the
producer that was missing.

**Why it had to be minted BEFORE the client route, not with it.** Tokens outlive a deploy by
their TTL. A rule written positively (`== "user"`) on the day the client route lands would
misjudge every token still in flight. Minting now means every live token already carries it by
then. **Until a full TTL has passed after this deploys, the rules stay written NEGATIVELY** —
both existing ones compare against `client`, and they must not be "improved" to positive form.

**One path for both routes** (`internal/application/commands/authentication_journal_manual.go`).
Every authentication outcome has to move a counter in `authentication_attempts` AND publish one
record on the log stream. Those were two call sites per branch, and a second route would have
duplicated the whole sequence — which is how a branch ends up counted but unannounced, or the
reverse, both invisible in a green build. `authenticationJournal` binds the two ports and the
subject kind once and exposes one method per outcome (`lockedUntil`, `refusedWhileLocked`,
`failed`, `succeeded`), each doing both halves. `POST /auth/client/token` will construct it with
`identityKindClient` and get the lockout, the counters and the announcements with nothing
re-implemented.

The handler keeps its `Attempts`/`Events` fields and builds the journal per call, so
`MountAuthentication`'s signature and every existing test construction site are untouched.
The refusal REASON stays the caller's to phrase and is passed in: the caller's answer is one
indistinguishable notification on every branch, while the stream keeps them apart — the journal
never invents a reason of its own.

| Artifact | Change |
|---|---|
| `internal/application/commands/authentication_journal_manual.go` | **NEW.** Holds `AttemptRecorder` and `AuthenticationEventPublisher` (moved), the `identityKindUser`/`identityKindClient` pair, `authenticationEventClass`, and the journal with its four outcome methods plus `values`/`announce`. |
| `internal/application/commands/authentication_commands_manual.go` | `claimIdentityKind` added to the claim block and to `buildClaims`; the five outcome sites now call the journal; `announce`/`attemptValues` removed (they live on the journal). |
| tests | the claim on **both** paths (a rotation rebuilds claims from the database, so a claim added to the sign-in alone would vanish at the first rotation); a pin that the minting name and Client's reading name agree, since nothing else checks and a rename would make Client's row rules go quietly inert; and the closed kind vocabulary. |

**Still not done, and named so it is a decision rather than an omission:** the client route needs
its own store port (`AuthenticationStore` is user-shaped) and its own result type (per RFC 6749
§4.4.3 a client-credentials response carries no refresh token and no user profile) — neither
should be widened to fit both.
