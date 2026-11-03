# Plan review ledger

Cumulative findings across review rounds (Review protocol, `.agents/global/delegation.md` in the author's notes). Each row: round, severity, finding, handling, the check that proves it.

| # | Round | Severity | Finding | Handling | Proof |
|---|---|---|---|---|---|
| 1 | 1 | blocking | `check-plan.py tests` used `\w`/`\s` with `git grep -E`, matched 0 Go tests | fixed: reads files with `git show`, Python regex, whole-word match | `check-plan.py tests` reports 138 missing before a map exists; items glob reports 16; mapping `TestGetOtherUsersItemX` still reports `TestGetOtherUsersItem` |
| 2 | 1 | should | no per-file mode; scope beyond httpapi unclear; substring match | fixed: `tests [GLOB]`; scope `GO_DIRS` = httpapi + cmd/app + frontend test files; word match | same as #1 |
| 3 | 1 | blocking | Connect paths would hit the kin-openapi validator; one CSRF deny handler for two protocols | regressed in round 2 (#19), fixed again by #19; originally: step 1 transition mux splits on `/api/<root>.` after `authenticate`; deny/MaxBytes choose format by `ErrorWriter.IsSupported` | step 1 acceptance: test service reachable through `NewHandler`; REST CSRF still problem, Connect CSRF a Connect error |
| 4 | 1 | blocking | connect-go decodes before interceptors, so "auth before validation" is not kept by interceptor order | fixed: premise + invariant: net/http check by procedure class before the Connect handler | step 1 mutation: unauthenticated bad JSON to an App method → `unauthenticated` |
| 5 | 1 | blocking | `backend/Dockerfile` `COPY api api` breaks once `backend/api/` is deleted | fixed: step 5 scope + `make lint-docker`; `COPY api` in the step 5 grep | step 5 acceptance |
| 6 | 1 | blocking | step 1 mutations had no service to act on; descriptor test passes on empty set | fixed: `api/<root>/testing/v1alpha1/testing.proto` (Public/App/Admin, Get+Update), kept to step 6; count > 0 assertion | step 1 mutations |
| 7 | 1 | should | moving validation into modules would make invalid requests spend rate-limit tokens | fixed: premise + invariant: handler validates, then limits, then calls service | step 3+4 mutation: limit-before-validate fails the "no token" test |
| 8 | 1 | should | step 1 renamed gen targets while saying old generation stays; drift unguarded in steps 2-4 | fixed: old targets renamed `gen-openapi*` / `gen:openapi*`, kept in check until step 5 | step 1 scope |
| 9 | 1 | should | step 3 alone fails the push gate (frontend no longer compiles) | fixed: steps 3 and 4 merged into one push "3＋4" | step heading |
| 10 | 1 | should | proto package root under rename: hyphens, directory match, descriptor bytes | decided: D5-A (fixed root `app`), Will 2026-11-03; option text corrected (#20) | plan 決定 D5 |
| 11 | 1 | should | frontend CI job and lefthook cannot get buf; generated TS commit; error_details.proto for TS | fixed: `api/.buf-version` + `scripts/fetch-buf.sh`; generated code committed; `frontend/buf.gen.yaml` includes error_details | step 1 acceptance: Frontend job runs `gen:api:check` |
| 12 | 1 | should | quality locks move every step, plan only said step 5 | fixed: lock update listed in steps 1, 2, 3＋4, 5 | step scopes |
| 13 | 1 | should | no mutation for silently dropped limits; `recover-admin` missing | fixed: mutations for title length, password 128, seed-admin; `recoverAdmin` in consumers | step 2, 3＋4 acceptance |
| 14 | 1 | should | no E2E in CI; SSR cookie forwarding and GET path unverified; `useHttpGet` missing | fixed: `server.test.ts` asserts GET path + cookie header; compose job curl probe with cookie; browser walk-through as 需要人做的事; `useHttpGet: true` | step 3＋4 acceptance |
| 15 | 1 | nit | items rg evidence wrong | fixed: replaced with `rg -n "/items|listItems|createItem"` | premise |
| 16 | 1 | should | files not marked new/modified/deleted | fixed: markers in steps 1, 2, 3＋4, 5 | step scopes |
| 17 | 1 | nit | step 5 grep: use tracked files, ADR exception format | fixed: `git grep` with pathspec excludes; ADR hits listed as `file:line` | step 5 acceptance |
| 18 | 1 | nit | CI baseline run not verified by reviewer (no network) | accepted: author ran `gh run view 41586205101` earlier (all jobs success) | session record |

## Round 2 (stopped: #19 is a new blocking caused by the round-1 fix of #3; per Review protocol no further fixes, ledger handed to Will)

| # | Round | Severity | Finding | Handling | Proof |
|---|---|---|---|---|---|
| 19 | 2 | blocking | `ErrorWriter.IsSupported` treats any POST `application/*` and any GET as Connect, so REST CSRF denials would become Connect errors; connect-go already maps MaxBytes to `resource_exhausted` | fixed after Will approved (2026-11-03, 「照建議」): deny handler chooses by the same path test as the mux split (`/api/<root>.`); drop the MaxBytes half; note `resource_exhausted` is HTTP 429 like rate limiting, distinguished by `RetryInfo` | `connect@v1.21.1/error_writer.go:79-88`, `protocol_connect.go:50`, `items_test.go:347` |
| 20 | 2 | blocking | D5 option A claims rename needs no regeneration; false for both options: `rename.py` rewrites the module path inside generated `.pb.go`, whose raw descriptor stores `go_package` with a length prefix | fixed after Will approved (2026-11-03, 「照建議」): both options regenerate after rename (or rename skips `internal/gen` then `buf generate`), rename CI job installs buf and loads descriptors; fix inventory T24; A must address go-backend D4 | `scripts/rename.py:31,55-63`; hawkr `media.pb.go:2184`; `ci.yml:95-99` (inferred from code, rename not run) |
| 21 | 2 | should | inventory B2, B9 destinations still say "interceptor" for the unauthenticated check | fixed after Will approved (2026-11-03, 「照建議」): "net/http check by procedure class (step 1); admin check in interceptor" | inventory B2, B9 |
| 22 | 2 | should | rows B12, B15, B17, B26, B27 say step 1 but step 1 scope omits them; B27 (field names instead of JSON Pointer) would change REST output during step 1 | fixed after Will approved (2026-11-03, 「照建議」): assign each; B27 moves to step 3＋4 | `rg -n "step 1" inventory.md` |
| 23 | 2 | should | step 1 mutation for the breaking selftest has the wrong direction | fixed after Will approved (2026-11-03, 「照建議」): make `check-buf-breaking.sh` miss the break (e.g. exit 0), `make tools-selftest` must fail | `selftest-checks.sh:31-39` |
| 24 | 2 | should | no existing test proves "invalid requests use no token"; premise depends on D2 and needs an exported pure check | fixed after Will approved (2026-11-03, 「照建議」): new test in step 3＋4; premise marked "depends on D2" | `ratelimit_test.go` test list; `ratelimit.go:92` |
| 25 | 2 | should | step 0 tests-map mutation impossible before step 2 | fixed after Will approved (2026-11-03, 「照建議」): move to step 2 | `check-plan.py tests` = 138 lines |
| 26 | 2 | should | step 5 `git grep` excludes only root `go.sum`/`pnpm-lock.yaml`; new ADR not historical | fixed after Will approved (2026-11-03, 「照建議」): `':(glob,exclude)**/go.sum'` etc.; list new ADR as exception | `backend/tools/go.sum:52-55,175-176` |
| 27 | 2 | should | Connect GET + cookie curl probe has no cookie source or expected result | fixed after Will approved (2026-11-03, 「照建議」): CreateUser via Caddy, keep Set-Cookie, GET GetCurrentUser with `connect=v1&encoding=json&message=%7B%7D` → 200 with account; without cookie → 401 | `ci.yml` probes |
| 28 | 2 | nit | unknown procedure under a known prefix: unauthenticated or not_found unspecified | fixed after Will approved (2026-11-03): applied as described | `server_test.go:221` |
| 29 | 2 | nit | handler wording implies it does sign-in throttling (lives in `auth.Service.SignIn`) | fixed after Will approved (2026-11-03): applied as described | `sessions.go:93` |
| 30 | 2 | nit | 被擋於 misses D3 on step 1 and D2 on step 3＋4 | fixed after Will approved (2026-11-03): applied as described | plan headings |
| 31 | 2 | nit | a few files still lack new/modified/deleted markers | fixed after Will approved (2026-11-03): applied as described | plan steps 1, 3＋4, 5 |
| 32 | 2 | nit | `check-plan.py` keys tests by name (two `TestMain` merge) and counts `TestMain`; scale table says 7 frontend files, script requires 9 | fixed after Will approved (2026-11-03): applied as described | `check-plan.py:51-64` |

Round 1 rows re-verified in round 2: #1, #2, #4 (plan text), #5, #6, #8, #9, #11, #12, #13, #15 still fixed; #3 regressed (#19); #7 partly (#24); #14 partly (#27); #16 mostly (#31); #17 partly regressed (#26); #18 accepted.

Count: findings 32; fixed 15; recorded 1 (#18); raised 1 (#10); open after round 2: 14 (#19–#32), plus #3 regressed.

After round 2: Will approved all proposed fixes and D1–D5 as recommended (2026-11-03). No third review round (Review protocol: at most two); the design-review skill reviews the plan and the ADR next.

Count after fixes: findings 32; fixed 30; recorded 1 (#18); raised and decided 1 (#10); rejected 0.

## Design review, start of migration (design-review skill, round 1, 2026-11-03)

| # | Round | Severity | Finding | Handling | Proof |
|---|---|---|---|---|---|
| 33 | DR1 | blocking | oasdiff kept until step 5 would fail every push once steps 2–3 delete paths | fixed: oasdiff, its script and selftest case removed in step 1 with `Quality-Gate:` | plan step 1 閘門; inventory T4–T7 |
| 34 | DR1 | should | `WIRE_JSON` does not block deleting an RPC or service | fixed: `RPC_NO_DELETE` + `PACKAGE_SERVICE_NO_DELETE`; selftest "delete an RPC" case and mutation | reviewer ran `buf config ls-breaking-rules` (buf 1.50.0); plan step 1; ADR Decision |
| 35 | DR1 | should | "invalid requests use no token" had no ADR/threat-model source; hawkr limits at the entry | raised → Will: entry limiting like hawkr (2026-11-03); invariant removed, intentional change recorded | plan premise, step 1, step 3＋4; ADR Rationale |
| 36 | DR1 | should | Go `errdetails` source undecided; genproto not approved; `field_behavior` unused | raised → Will approved `google.golang.org/genproto/googleapis/rpc`; managed mode skips googleapis for Go; `field_behavior` dropped | plan deps, step 1 |
| 37 | DR1 | should | test proto under `api/` leaks into frontend, breaking and derived projects | fixed: separate buf module in `backend/internal/httpapi/rpc/testdata/`, Go only | plan step 1 |
| 38 | DR1 | should | unhandled-error logging not carried over | fixed: conversion interceptor logs internal errors; HTTP layer logs pre-interceptor errors; mutation added | plan step 1; inventory B6 |
| 39 | DR1 | should | unknown JSON fields silently discarded, not listed as intentional change | fixed: listed in step 2 intentional changes; inventory B3, B4; ADR Decision | `connect@v1.21.1/codec.go:180` |
| 40 | DR1 | should | baseline ADR D6 "limits only in contract" superseded but not listed | fixed: ADR `supersedes` and Related; baseline Review Note; plan step 0 不能動 | ADRs |
| 41 | DR1 | should | inventory lacked why/still-holds/from-scratch for carried mechanisms | fixed: inventory §7 carry-over audit (16 rows) | inventory §7 |
| 42 | DR1 | nit | re-auth rationale (no DELETE body) gone; claimed noted in ADR but was not | fixed: ADR Decision restates the remaining reason; inventory D8 | ADR |
| 43 | DR1 | nit | D5 reversibility text stale after D1-B; ADR and plan numbering differ | fixed: plan D5 可逆性; ADR uses D5 for the package root and notes D4 lives in the plan | plan D5; ADR |
| 44 | DR1 | nit | service-class checked in three places | fixed: startup check is the only class check (with its test); descriptor "three classes" test dropped; net/http `not_found` only for procedures absent from the table | plan step 1 |
| 45 | DR1 | nit | Connect GET puts the request in `?message=`, logged by Caddy/Vercel; hawkr §三 rule missing | fixed: premise + inventory D11 + step 5 api-design | plan |
| 46 | DR1 | nit | two buf download paths; T3 wording | fixed: `scripts/fetch-buf.sh` is the single download point; T3 rewritten | plan step 1; inventory T3 |
| 47 | DR1 | nit | 401 redirect keyed on a procedure list | fixed: redirect unless `ErrorInfo.reason` is `INVALID_CREDENTIALS` (default, overridable); mutation updated | plan step 1, step 3＋4 |

Count: findings 15; fixed 13; raised 2 (both decided by Will); recorded 0; rejected 0. Instruction-file reconciliation: 0 changes (the three `AGENTS.md` still describe the current code; they change in step 5).

## Design review round 2 (2026-11-03)

No blocking; no finding caused by a round-1 fix. Per Review protocol only fixes proven by a test or a run were applied; the rest are open and handed to Will.

| # | Round | Severity | Finding | Handling | Proof |
|---|---|---|---|---|---|
| 48 | DR2 | should | breaking rules miss deleting or renaming a whole package | fixed: `PACKAGE_NO_DELETE` added; selftest "delete a package" case + mutation | buf 1.50.0 fixture in scratchpad: without the rule deleting `app.b.v1` exit 0; with it exit 100, renaming `app.a.v1`→`app.c.v1` exit 100 |
| 49 | DR2 | should | `api/buf.yaml` googleapis deps + `buf.lock` have no importer after `field_behavior` was dropped | fixed after Will approved (2026-11-03): drop deps and lock; `frontend/buf.gen.yaml` takes googleapis as a pinned `module:` input (as hawkr's TS config); no Go `disable` needed | hawkr `packages/hawkr-api-ts/buf.gen.yaml` |
| 50 | DR2 | should | HTTP-layer logging of pre-interceptor errors has no necessity: those are 4xx and `accessLog` already records procedure + status | fixed after Will approved (2026-11-03): drop it, keep only "internal errors logged by the interceptor" | `middleware.go:109-139`; old system logged only unknown errors (`problem.go:126`) |
| 51 | DR2 | should | `fetch-buf.sh` + `.buf-version` duplicates the repo's Go-free download pattern (root `Makefile` hadolint with checksum, stamp, `tool_versions.py`); checksum location contradictory | fixed after Will approved (2026-11-03): `BUF_VERSION` + `BUF_SHA256_<platform>` in the root `Makefile`, everyone calls `make bin/buf`; or keep hawkr's `.buf-version` (Will's call) | root `Makefile:127-150`; `scripts/tool_versions.py:119-151` |
| 52 | DR2 | should | only the CSRF deny handler chooses Connect vs problem by path; `authenticate` DB errors and outer panic recovery still write problems on the Connect path, and lose their writer in step 5 | fixed after Will approved (2026-11-03): one HTTP-layer error writer choosing by the same path test, used by CSRF, authenticate, recover, unauthenticated check, rate limit; mutation for authenticate DB error → Connect `internal` | `auth.go:91`, `problem.go:149` |
| 53 | DR2 | should | descriptor test scans a hand-written package list; a new module outside the list escapes the `NO_SIDE_EFFECTS` rule | fixed after Will approved (2026-11-03): derive the scope from the mounted services (`NewHandler`'s procedure table) or fold the naming check into the startup check | plan step 1 |
| 54 | DR2 | should | apperr `Kind`→Connect code: `Conflict` maps to two codes (`account_taken` already_exists, `last_admin` failed_precondition); §7 lacks apperr, B11, B19, B15 | fixed after Will approved (2026-11-03): fix the mapping table in step 1 (split the Kind or let the reason pick the code), add the four rows to §7 | `apperr.go:11`; hawkr error model ADR |
| 55 | DR2 | nit | inventory F1, O10 still described the procedure-list redirect | fixed | inventory F1, O10 |
| 56 | DR2 | nit | `updateMask` example wrong (it is the official JSON name) | fixed: examples `update_masks`, `updatemask` | protojson JSON mapping |

Ledger re-check from round 2: #33, #35, #37, #39, #40, #42, #44, #45 still fixed; #34 completed by #48; #36 leaves #49; #38 leaves #50; #41 leaves #54; #43 depended on #48 (now holds); #46 not fixed → #51; #47 regressed in inventory → fixed by #55.

Count (design review): findings 24; fixed 16; raised 2 (decided by Will); open 6 (#49–#54); rejected 0.

After design review round 2: Will approved #49, #50, #52, #53, #54 as proposed and chose the root `Makefile` for buf (#51), 2026-11-03. No third round (Review protocol: at most two).

Count (design review, final): findings 24; fixed 22; raised 2 (decided by Will); open 0; rejected 0.

## Step 1 implementation review (2026-11-03)

Round 1 over `085c75c..6a39a76` (and `a381fea`): `/code-review high` and a diff-lens reviewer given the plan's step 1 as the contract. Fix commits: `1e0fcd7` and the commit after it.

| # | Round | Severity | Finding | Handling | Proof |
|---|---|---|---|---|---|
| S1 | 1 | blocking | `check-buf-breaking.sh` ran a relative buf path after its `cd`; `make api-breaking` would fail once main has `api/buf.yaml` (both reviewers; reproduced) | fixed: path resolved before the `cd`; selftest calls it as the Makefile does | old script fails 5 selftest cases, new passes; `../scripts/check-buf-breaking.sh ../bin/buf HEAD` exit 0 |
| S2 | 1 | should | `rpc.Convert` passed a wrapped or forwarded internal/unknown `*connect.Error` through with its text, unlogged (both reviewers) | fixed: internal and unknown become internal | `TestConvertUnexpectedIsInternal` fails with the old passthrough |
| S3 | 1 | should | NO_SIDE_EFFECTS rule was a bare prefix test (`GetOrCreateX`, `Listen` passed) | fixed: `isRead` needs Get/List plus a word, not `Or<Verb>` | `TestIsRead` |
| S4 | 1 | should | docs, skills and comments said `make gen` / `pnpm gen:api` regenerate the REST code (both reviewers) | fixed: they name gen-openapi / gen:openapi; quick references list both | `rg "make gen\b|gen:api"` hits only Connect mentions |
| S5 | 1 | should | `api-design.md` still listed `apperr.Conflict` | fixed | same file |
| S6 | 1 | should | gen-check compares with the index, so a staged-but-uncommitted regeneration passes pre-push; it overwrites hand edits in generated dirs | accepted: the rename job needs the index comparison; CI checks the committed tree; generated dirs are never edited by hand | — |
| S7 | 1 | should | depguard's `internal/gen` deny matched `internal/generated` too, unlike archtest | fixed: `internal/gen$` and `internal/gen/` | `make lint` |
| S8 | 1 | should | `errInsufficientRole`, `errNoSession` and the panic error duplicated existing values | fixed: one `rpc.ErrInsufficientRole`, `auth.ErrSessionInvalid`, `errInternal()` | `go vet`, tests |
| S9 | 1 | should | `apiBaseUrl` duplicated `client.ts`'s base URL, including the `.invalid` fallback | fixed: `src/lib/api/base-url.ts` shared | `transport-server.test.ts`, `server-base.test.ts` |
| S10 | 1 | should | field paths did not undo RFC 6901 escapes; REST camelCase names are not proto paths | fixed: `~1`/`~0` undone; the name mapping is step 3＋4 (B27) | `TestConvertValidation` |
| S11 | 1 | should | the connect-query compatibility stop condition was not evaluated in step 1 | recorded: connect-query is installed with its first caller; the stop condition moves to step 3＋4; peer range `>=5.62.0` vs `^5.105.0` | plan step 3＋4 stop condition |
| S12 | 1 | nit | the internal-error log lacked the HTTP method | fixed | `TestInternalErrorLogged` |
| S13 | 1 | nit | `807247a` added 27 backend tests without moving the lock (moved in `387014a`) | accepted: already pushed, history is not rewritten; later commits move the lock with their tests | — |

Count (step 1 round 1): findings 13; fixed 10; recorded 1; accepted 2; raised 0; rejected 0. Round 2 runs only on the fix commits if CI or a test does not settle them.

## Step 2 (items to Connect)

`/code-review high` (C) and the diff-view reviewer (V) on the uncommitted step 2 diff, round 1. Where both found the same defect it is one row.

| # | Round | Severity | Finding | Handling | Proof |
|---|---|---|---|---|---|
| S14 | 1 (C, V) | blocking | `CreateItem` read `in.Description` on a nil `item` and panicked (internal instead of `REQUIRED`) | fixed: a nil item becomes an empty one in `CreateItem` and `UpdateItem` | `TestCreateItemTitleLimits` (`create(nil)`), CI |
| S15 | 1 (C, V) | blocking | `frontend/src/lib/api/schema.d.ts` not regenerated after the items paths left `openapi.yaml`; `gen:openapi:check` fails | fixed: `pnpm gen:openapi` | `pnpm gen:openapi:check` exit 0 |
| S16 | 1 (C, V) | blocking | threat model not updated although `connectServices` mounts a new service and `operationAccess` lost five entries (its own update rule, `:132`) | fixed: entry, CSRF and admin-call rows name `ItemAppService` and its tests | `docs/security/threat-model-auth.md` |
| S17 | 1 (V) | should | `tests-map.md`: deleting the `TestCreateItem` row was not reported, another row named it | fixed: that row no longer names the test | each of the 16 rows deleted in turn: `check-plan.py tests` reports each |
| S18 | 1 (V) | should | no `field_behavior` and a handler-side writable list differ from hawkr §四 without a plan premise | fixed: premise added (default, overridable, with trigger) | plan 前提 |
| S19 | 1 (V) | should | the diff is over 1,000 changed lines | fixed: split into commits under the limit, or `Large-Change:` where one unit cannot split | `check_commit_policy.py` at commit |
| S20 | 1 (C) | should | `backend/AGENTS.md` rule 1 (limits only in `openapi.yaml`) and rule 6 (`operationAccess` for every operation) contradict D2 and the Connect services; `.claude/skills/add-crud.md` follows items over REST | recorded: plan step 5 rewrites the three `AGENTS.md` and the skills (D13–D15, D17); until then the plan and its ADR govern the migration | step 5 scope |
| S21 | 1 (C) | should | `docs/rules/api-design.md` cites code deleted from `items.go` as its examples | recorded: step 5 rewrites `api-design.md` as the Connect rules | step 5 scope |
| S22 | 1 (C) | should | `ListItems` decodes the page token twice (handler for the `page_token` field, service again through `httpx`) | recorded: the service keeps the `List(limit, cursor *string)` interface (step 2 不能動); step 5 deletes `httpx` and moves the cursor to `rpc`, which removes the second decode | step 5 scope |
| S23 | 1 (C) | nit | `item.id` was built as `/item.id` by hand while other fields use `rpc.Under` | fixed: `rpc.Under("item", rpc.ParseID("id", …))` | `TestUpdateItem` (`item.id`), CI |
| S24 | 1 (V) | nit | `TestUpdatedAtChanges` sleeps 10ms | rejected: carried over unchanged; it depends on two transactions' `now()` being 10ms apart, not on test order | — |
| S25 | 1 (V) | nit | the `update_time` comment said it changes only when a field changes | fixed: comment says any UpdateItem that sets a field | `items.proto` |

| S26 | CI | should | the test-count lock was set to 282, CI counted 277 (run 41644558611; every test passed) | fixed: the author missed the five `TestOperationAccessEnforced` subtests of the deleted items operations; lock 277 with a `Quality-Gate:` trailer, `tests-map.md` names the subtests | `check_locks.py` in CI |

Count (step 2 round 1): findings 12 (blocking 3); fixed 8; recorded 3; raised 0; rejected 1.

Step 3＋4 (2026-11-03): `/code-review high` (C) on `36dadf0..efe8c28`, then a diff-perspective reviewer (V) on `36dadf0...4a21706` (the first V reviewer failed with an API error and was replaced by one fresh reviewer), and the CI mutation runs.

| # | Round | Severity | Finding | Handling | Proof |
|---|---|---|---|---|---|
| S27 | 1 (C) | should | `apperr.Fields` saw only a join at the top: a join a service wraps with `%w` reported its first field only | fixed: `Fields` walks wrapping until a join or the end of the chain | `TestConvertValidation` (wrapped join); a `Fields` that stops at wrappers fails it |
| S28 | 1 (C) | should | password length is counted on the raw string, the hash and the list use the NFKC form | recorded: unchanged from REST (kin-openapi `minLength`/`maxLength` on the raw JSON string); trigger: the next revision of the password policy | `backend/api/openapi.yaml` at `36dadf0`, `CreateUserRequest` |
| S29 | 1 (C) | should | `clearStaleCookie` cleared the invalid cookie only on writes through it: an outer panic or a Flush sent no clearing | fixed: the clearing is on the response from the start; the wrapper removes it only when the handler set the session cookie | `TestInvalidSessionCookieCleared`, CI run 41673635223 |
| S30 | 1 (C) | should | REST `writeError` reports a joined validation error as its first field | rejected: `openapi.yaml` has no operation (`paths: {}`), so no module error reaches the REST writer; step 5 deletes it | `backend/api/openapi.yaml:16` |
| S31 | 1 (C) | should | `UpdateUser` with `disabled` in the mask and no value enables the account | fixed: documented as AIP-134 (a listed field takes the request's value, unset is the default) on `UpdateUserRequest.update_mask` | `api/app/admin/users/v1/users.proto` |
| S32 | 1 (C) | should | `AGENTS.md`, both READMEs and `frontend/AGENTS.md` still pointed at the deleted `gen:openapi`, `client.ts`, `errors.ts`; `backend/AGENTS.md` rule 6 at the deleted `operationAccess` | fixed in `4a21706` (rule 1 and 6 of `backend/AGENTS.md` too); the full rewrite stays in step 5 | `rg "gen:openapi\|openapi-fetch\|schema\.d\.ts" AGENTS.md README.md frontend` |
| S33 | 1 (C) | nit | `pageRequest` duplicated the paging items inlined | fixed: `rpc.PageRequest` for items, users and sessions | `TestPageRequest` |
| S34 | 1 (C) | nit | the page token is decoded twice | recorded: same as S22, step 5 | — |
| S35 | 1 (V) | should | threat model "註冊時自己指定角色" cited the REST `additional_property` check | fixed: names `DiscardUnknown` and `TestCreateUserLimits` (sign-up with `role` is `ROLE_USER`) | `docs/security/threat-model-auth.md` |
| S36 | 1 (V) | should | data inventory still named `GET /api/users/{id}/sign-in-lock`, `getCurrentUser`, `listSessions`, `patchUser` | fixed: the Connect methods | `rg "getCurrentUser\|patchUser" docs/privacy` (none) |
| S37 | 1 (V) | should | `frontend/AGENTS.md` (adding a feature), `backend/AGENTS.md:55`, `docs/rules/api-design.md`, `.claude/skills/add-crud.md`, `add-module.md` describe REST paths and deleted files | recorded: step 5 scope (D13–D17, S20, S21) | plan step 5 |
| S38 | 1 (V) | nit | `openapi.yaml` keeps components only the deleted operations used | recorded: step 5 deletes the file | plan step 5 |
| S39 | 1 (V) | nit | comments name `requireRole` (`auth/auth.go`, `queries/users.sql`); `.gitattributes` and `biome.json` name `schema.d.ts` | recorded: step 5 (D20 comments; `biome.json` needs `Quality-Gate:`) | plan step 5 |
| S40 | CI | should | mutation run 41663511023 stopped at `deadcode`: M1 left `Cookie.device` unreachable, no test ran | fixed in the mutation branch (M1 builds the cookie and drops it) | run 41666704688 reached the tests |
| S41 | CI | should | under M1, `TestDeviceCookieIsPerAccount` dereferenced the missing device cookie and the panic aborted the package (134 of 303 tests ran) | fixed: `mustDeviceCookie` fails the test instead | run 41673873713 ran all 303, 15 failures, every mutation caught |

Count (step 3＋4 round 1, C and V): findings 13 (blocking 0); fixed 7; recorded 5; raised 0; rejected 1. CI rounds: 2 more, both fixed.
