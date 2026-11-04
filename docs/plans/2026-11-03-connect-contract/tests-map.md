# Tests map

Every test of the snapshot (`a83acc9`) in `backend/internal/httpapi/`, `backend/cmd/app/` and the frontend's test files, with the test that replaces it or the reason it is retired (decision D4-A). `python3 docs/plans/2026-11-03-connect-contract/check-plan.py tests [GLOB]` fails on a snapshot test this file does not name. Intended behavior changes are written in the row of the test that shows them.

Columns: old test, new test (file), what changed. A test that keeps its name and moves to Connect lists the same name.

## Step 2: `backend/internal/httpapi/items_test.go` (16 tests, now `ItemAppService`)

| Old test | New test | What changed |
|---|---|---|
| TestCreateItem | TestCreateItem (`items_test.go`) | `POST /api/items` 201 → `CreateItem` returns `{item}`. The `Location` header is gone: the descriptor check (`TestConnectServicesReturnTheirResources`) requires every Create to return the resource and to have a Get, and the test reads the item back with `GetItem` (inventory B23, D9 replaced by the ADR). No description: was `"description": null`, now the field is unset (proto3 JSON omits it); the empty string stays a description. New: an `id` or `owner_id` the client sends is ignored. |
| TestCreateItemUnauthenticated | TestItemsNeedSession (`items_test.go`) | 401 problem → `unauthenticated` (`SESSION_INVALID`) for all five methods, before the body is decoded (malformed JSON still gets `unauthenticated`); runs without a database. |
| TestCreateItemTitleTooLong | TestCreateItemTitleLimits (`items_test.go`) | 422 `/title` → `invalid_argument` with a `BadRequest` violation on `item.title`. The limit moved from `openapi.yaml` to `items.Service` (D2-A, `checkTitle`). Intended change: an empty title was `too_short` and a missing one `required`; proto3 cannot tell the two apart, so both are `REQUIRED`. 201 characters is `TOO_LONG`; 200 multi-byte characters pass; the database CHECK still refuses 201. |
| TestListMyItems | TestListMyItems (`items_test.go`) | `next_cursor: null` → `next_page_token` empty on the last page (AIP-158). The "empty array, not null" assertion is gone: proto3 JSON omits an empty repeated field and every generated client reads it as empty. The anonymous call moved to TestItemsNeedSession. |
| TestPaginationKeyset | TestPaginationKeyset (`items_test.go`) | `limit`/`cursor` → `page_size`/`page_token`; same keyset behavior (the token is still the `internal/platform/httpx` cursor). |
| TestListItemsInvalidPage | TestListItemsPageParameters (`items_test.go`) | Intended changes (AIP-158): `limit=101` was 422 `out_of_range`, `page_size` 101 or 1000 is now read as 100; an empty `cursor=` was 422 `invalid`, an empty `page_token` is now the first page. Negative `page_size` is `OUT_OF_RANGE` on `page_size`; a token this API did not return is `INVALID` on `page_token`. |
| TestDefaultPageSizeMatchesContract | TestDefaultPageSizeMatchesContract (`auth_resources_test.go`) and TestListItemsPageParameters | Moved: the REST users and sessions lists still read `defaultPageSize` against the contract's `Limit` default until step 3＋4. For items, the default 20 and the maximum 100 are in the proto comment and checked by TestListItemsPageParameters (101 rows: page_size 0 → 20, 101 → 100). |
| TestListItemsLimitNotDecimal | retired | Tested the REST parameter binder against kin-openapi's base-0 integer parsing (`limit=0x10`). `page_size` is a proto `int32` that protojson decodes; there is no query parameter binder of ours left to test. |
| TestGetItem | TestGetItem (`items_test.go`) | 404 problem → `not_found`; a non-UUID id was 422 `format` on `/id`, now `INVALID` on `id` (`rpc.ParseID`). |
| TestGetOtherUsersItem | TestGetOtherUsersItem (`items_test.go`) | 404 → `not_found`; still indistinguishable from a missing item. |
| TestUpdateItem | TestUpdateItem (`items_test.go`) | `PATCH` → `UpdateItem` with `update_mask` `[title, description]`; unknown id `not_found`; new: a non-UUID `item.id` is `INVALID` on `item.id`. |
| TestUpdateItemMergePatch | TestUpdateItemMask (`items_test.go`), TestUpdatePaths (`rpc/request_test.go`) | JSON Merge Patch → `update_mask` (AIP-134). Omitted field unchanged → field not in the mask unchanged, even when the request carries a value for it; `description: null` clears → `description` in the mask and unset clears; `{}` → no mask and no values changes nothing. Intended changes: `title: null` (`null_not_allowed`) → `title` in the mask and empty is `REQUIRED` on `item.title`; the 415 for `application/json` is gone (Connect accepts JSON and binary Protobuf, connect-go answers other content types itself). New: no mask changes the fields that have a value; `"*"` is both fields; listed server-set fields (`update_time`, `owner_id`) are ignored; a path that is not an Item field is `INVALID` on `update_mask`; a 201-character title is `TOO_LONG`. Also intended (plan step 2): unknown JSON fields are dropped, not 422 `additional_property` (connect-go's `DiscardUnknown`). |
| TestUpdatedAtChanges | TestUpdatedAtChanges (`items_test.go`) | `updated_at`/`created_at` → `update_time`/`create_time` (`google.protobuf.Timestamp`). The "new item has updated_at = created_at" check moved to the first row's new test; the "empty patch leaves updated_at" check is now an update with no mask and no values. |
| TestDeleteItem | TestDeleteItem (`items_test.go`) | 204 with no body → empty `DeleteItemResponse`; second delete `not_found`; new: a non-UUID id is `INVALID` on `id`. |
| TestUpdateOtherUsersItem | TestUpdateOtherUsersItem (`items_test.go`) | 404 → `not_found` for update and delete; the item is unchanged. |
| TestCreateItemCrossSite | TestCreateItemCrossSite (`items_test.go`) | 403 problem → `permission_denied` with `CROSS_ORIGIN_REQUEST`, through the generated client with `Sec-Fetch-Site: cross-site`; nothing is created. |

Also changed in step 2, outside `items_test.go`:

- TestListQueriesDoNotGrowWithRows (`queries_test.go`): the items list is a Connect GET to `ListItems` with `{"pageSize":100}`; users and sessions stay REST until step 3＋4.
- TestOperationAccessEnforced (`authz_test.go`): it runs one subtest per REST operation in the contract, so it lost the five items subtests (21 → 16 operations); the Connect side is TestItemsNeedSession's five subtests. The full row comes with step 3＋4.
- TestEveryWritablePathHasGet (`spec_test.go`): its self-check removed GET from `/items/{id}`, which left the contract; it now uses `/sessions/{id}`.

## Step 3＋4: `backend/internal/httpapi` auth and REST-shell tests (now `UserPublicService`, `UserAppService`, `UserAdminService`, `SessionPublicService`, `SessionAppService`)

Status and code mapping (ADR D3, `rpc/errors.go` `kindCodes`): 401 → `unauthenticated` (`SESSION_INVALID` or `INVALID_CREDENTIALS`); 403 `insufficient_role` → `permission_denied` `INSUFFICIENT_ROLE`; 403 `reauthentication_required` → `permission_denied` `REAUTHENTICATION_REQUIRED`; 403 cross-site → `permission_denied` `CROSS_ORIGIN_REQUEST`; 409 `account_taken` → `already_exists` `ACCOUNT_TAKEN`; 409 `last_admin` → `failed_precondition` `LAST_ADMIN`; 429 → `resource_exhausted` with `RetryInfo` (the `Retry-After` header is gone; tests check the delay within the second up to the old header value); 422 → `invalid_argument` with one `BadRequest` violation per invalid field; 404 → `not_found`. Cookies are read from the response headers, or from the error's metadata on a failed call. Renamed fields: `is_disabled` → `disabled`, `created_at` → `create_time`, `last_used_at` → `last_use_time`, `expires_at` → `expire_time`. Lists: `limit`/`cursor` → `page_size`/`page_token`, `next_cursor: null` → empty `next_page_token`. No `Location` header anywhere (B23): a created resource is read back through its Get with the session the response set.

### `auth_test.go` (37 tests)

| Old test | New test | What changed |
|---|---|---|
| TestOperationsRejectsUnknownName | retired | `operations()` (reflection over `gen.StrictServerInterface`, B14) was deleted with `limitOperations`. The Connect counterpart, `NewHandler` refusing a `connectLimited` procedure that is not mounted ("rate limit: %s is not a mounted procedure"), is TestConnectRateLimitNamesMountedProcedures (`connect_test.go`, since step 1); `connectLimited` names generated `...Procedure` constants, so a misspelling does not compile. |
| TestCreateUserSetsSessionCookie | TestCreateUserSetsSessionCookie (`auth_test.go`) | `POST /api/users` 201 → `UserPublicService.CreateUser` returning `{user}`. No `Location`: the new user is read back through `UserAppService.GetUser` with the cookie the response set (what `followLocation` checked), and TestConnectServicesReturnTheirResources requires CreateUser's GetUser in `app.users.v1`. `role: "user"` → `ROLE_USER`; `is_disabled` → `disabled`, always set; `created_at` → `create_time` (protojson always writes RFC 3339 in UTC, so the "ends in Z" check became "is set"); the id is a UUIDv7. Cookie attributes unchanged (HttpOnly, Secure, Path=/, SameSite=Lax, no Domain, Max-Age 7 days, 43-character token). |
| TestCreateUserDuplicate | TestCreateUserDuplicate (`auth_test.go`) | 409 `account_taken` → `already_exists` `ACCOUNT_TAKEN` (Conflict split into AlreadyExists and FailedPrecondition, plan step 1). |
| TestCreateUserConcurrentSameAccount | TestCreateUserConcurrentSameAccount (`auth_test.go`) | One 201 and 19 × 409 → one success and 19 × `already_exists`. |
| TestCreateUserPasswordTooShort | TestCreateUserLimits (`auth_test.go`) | 422 `too_short` at `/password` → `invalid_argument`, violation `password TOO_SHORT`. The limits moved from `openapi.yaml` to `auth.checkNewAccount` (D2-A; seed-admin and recover-admin use the same). New cases: password of 129 characters `TOO_LONG` (plan mutation "拿掉 password 的 128 上限"), empty `REQUIRED`; account of 2 `TOO_SHORT`, 51 `TOO_LONG`, with a space `PATTERN`; a 50-character account with a 128-code-point (384-byte) password and a 15-character password pass. Intended changes: an empty account was `too_short` (the contract's minLength 3) and is `REQUIRED` (proto3 cannot tell empty from unset); every invalid field is reported at once from the auth service (`account TOO_SHORT` and `password TOO_SHORT` in one `BadRequest`), so seed-admin gets the same list as HTTP; a field the request does not have is dropped, not 422 `additional_property` (connect-go `DiscardUnknown`, plan step 2), and a `role` sent with the sign-up still makes a plain user. |
| TestCreateUserNoCompositionRule | TestCreateUserNoCompositionRule (`auth_test.go`) | 201 → success. |
| TestCreateUserCommonPassword | TestCreateUserCommonPassword (`auth_test.go`) | 422 `password_too_common` at `/password` → `invalid_argument`, violation `password PASSWORD_TOO_COMMON`. |
| TestCreateUserBreachedPassword | TestCreateUserBreachedPassword (`auth_test.go`) | Same mapping; the breach check still sees the NFKC form, and its failure still lets the sign-up through. |
| TestCreateSession | TestCreateSession (`auth_test.go`) | `POST /api/sessions` 201 → `SessionPublicService.CreateSession` returning `{session}`; the session's `user.id` is the account's; the new cookie authenticates GetCurrentUser. New test next to it: TestCreateSessionLimits, the sign-in limits now in `auth.checkSignIn` (account 1–50, password 1–128): 129-character password `TOO_LONG` (plan mutation), 51-character account `TOO_LONG`, both empty `account REQUIRED` + `password REQUIRED` in one response, both too long in one response; a 2-character account or a 50/128 pair is left to the credentials (`INVALID_CREDENTIALS`), so a rule tightened later does not lock out older accounts. |
| TestCreateSessionWrongPassword | TestCreateSessionWrongPassword (`auth_test.go`) | 401 `invalid_credentials` → `unauthenticated` `INVALID_CREDENTIALS`. |
| TestCreateSessionUnknownAccountRunsHash | TestCreateSessionUnknownAccountRunsHash (`auth_test.go`) | "Same problem JSON without request_id" → the same Connect error: code, message and every detail byte for byte (`assertSameError`); still one password verification. |
| TestCreateSessionIssuesNewToken | TestCreateSessionIssuesNewToken (`auth_test.go`) | Over the client with the old cookie; after the sign-in the old token is `unauthenticated` and the response carries exactly one session cookie, the new one (plan mutation "CreateSession 不再結束請求帶來的舊 session"). |
| TestCreateSessionMixedCaseAccount | TestCreateSessionMixedCaseAccount (`auth_test.go`) | Through the generated client only. |
| TestPasswordNormalized | TestPasswordNormalized (`auth_test.go`) | 201 → success. |
| TestCreateSessionDisabledAccount | TestCreateSessionDisabledAccount (`auth_test.go`) | `INVALID_CREDENTIALS`, the same as a wrong password. |
| TestMe | TestMe (`auth_test.go`) | `GET /api/users/me` → `UserAppService.GetCurrentUser` (GET, `NO_SIDE_EFFECTS`); the raw `"role":"user"` → `ROLE_USER`; the user equals the one CreateUser returned; UUIDv7 id. |
| TestMeWithoutCookie | TestMeWithoutCookie (`auth_test.go`) | 401 problem → `unauthenticated` `SESSION_INVALID`. |
| TestMeWithUnknownToken | TestMeWithUnknownToken (`auth_test.go`) | Same. |
| TestInvalidSessionCookieCleared | TestInvalidSessionCookieCleared (`auth_test.go`) | The clearing cookie of a failed call is read from the error's metadata. New: a failed sign-in carrying the dead cookie also gets the clearing cookie; a sign-in and a sign-up carrying it get exactly one session cookie, the new one (`clearStaleCookie`: connect-go adds the handler's headers to those already on the writer, so `authenticate` clears only when the response starts without one); a valid session gets no `Set-Cookie`. |
| TestMeIgnoresBearer | TestMeIgnoresBearer (`auth_test.go`) | `Authorization: Bearer <token>` on GetCurrentUser → `unauthenticated`. |
| TestMeDisabledAccount | TestMeDisabledAccount (`auth_test.go`) | `unauthenticated` `SESSION_INVALID`. |
| TestMeAfterUserDeleted | TestMeAfterUserDeleted (`auth_test.go`) | Same. |
| TestSessionIdleExpiry | TestSessionIdleExpiry (`auth_test.go`) | Same, through GetCurrentUser. |
| TestSessionAbsoluteExpiry | TestSessionAbsoluteExpiry (`auth_test.go`) | Same. |
| TestDeleteCurrentSessionClearsCookie | TestDeleteCurrentSessionClearsCookie (`auth_test.go`) | `DELETE /api/sessions/current` 204 → `SessionPublicService.DeleteCurrentSession` (Public, so a stale cookie can sign out). New cases: a session that has ended, an unknown token and an idle-expired session all succeed and get `Max-Age=0` (plan mutation "DeleteCurrentSession 改成需要登入"). |
| TestDeleteCurrentSessionRevokes | TestDeleteCurrentSessionRevokes (`auth_test.go`) | Same through the client. |
| TestDeleteAllSessions | TestDeleteAllSessions (`auth_test.go`) | `DELETE /api/sessions` → `SessionAppService.DeleteAllSessions`; anonymous → `unauthenticated`. |
| TestAdminDisablesUserRevokesSessions | TestAdminDisablesUserRevokesSessions (`auth_test.go`) | `PATCH /api/users/{id}` merge-patch `{"is_disabled":…}` → `UserAdminService.UpdateUser` `{user: {id, disabled}}` with `update_mask: [disabled]` (AIP-134); the empty merge patch → no mask and `disabled` unset, the user comes back unchanged; 404 → `not_found`. New: a non-UUID `user.id` is `INVALID` on `user.id`, a mask path that is not a User field (`is_disabled`) is `INVALID` on `update_mask`. |
| TestLastAdminCannotBeDisabled | TestLastAdminCannotBeDisabled (`auth_test.go`) | 409 `last_admin` → `failed_precondition` `LAST_ADMIN`. |
| TestConcurrentAdminDisableLeavesOne | TestConcurrentAdminDisableLeavesOne (`auth_test.go`) | One 200 and one 409 or 401 → one success and one `failed_precondition` or `unauthenticated`. |
| TestNonAdminCannotPatchUser | TestNonAdminCannotPatchUser (`auth_test.go`) | 403 `insufficient_role` → `permission_denied` `INSUFFICIENT_ROLE` (the admin interceptor, B11). |
| TestUnauthenticatedBeforeValidation | TestUnauthenticatedBeforeValidation (`auth_test.go`) | One anonymous PATCH with a bad body → every App and Admin procedure that `connectServices` mounts (19 subtests): anonymous malformed JSON and `Content-Type: text/plain` are `unauthenticated`, while the Public CreateSession answers the malformed body `invalid_argument`. Runs without a database: `connectEntry` refuses before connect-go decodes (plan premise, invariant table row 1). |
| TestLoginThrottlePerAccount | TestLoginThrottlePerAccount (`auth_test.go`) | 429 `login_throttled` with `Retry-After: 60` → `resource_exhausted` `LOGIN_THROTTLED` with a 60-second `RetryInfo`; an existing and an unknown account get the same error (`assertSameError`). |
| TestLoginThrottleConcurrent | TestLoginThrottleConcurrent (`auth_test.go`) | 5 × 401 and 15 × 429 → 5 × `unauthenticated` and 15 × `resource_exhausted`; still 5 password verifications. |
| TestRateLimitAuthOperations | TestRateLimitAuthOperations (`auth_test.go`) | 2 a minute with `Retry-After: 30` → 5 a minute: the 6th call from one address is `resource_exhausted` with a 12-second `RetryInfo`, for CreateUser and for CreateSession (one subtest each; the procedures are listed in the test, not read from `connectLimited`, so the plan mutation that drops CreateSession from it fails). Intended change (plan premise, Will 2026-11-03): the limit runs at the Connect entry before decoding, so a malformed body (`{not json`, 400) and an invalid request (empty password) use a token too, and over the limit a malformed body is `resource_exhausted`; before, invalid requests used no token (the validator ran first). Also checked: sign-up and sign-in share an address's allowance, another address has its own, DeleteCurrentSession and GetCurrentUser are not limited. |
| TestCredentialsNeverLogged | TestCredentialsNeverLogged (`auth_test.go`) | The malformed body carrying the password is a Connect POST with truncated JSON (400 `invalid_argument`); the access log's path is `/api/app.sessions.v1.SessionPublicService/CreateSession`. |
| TestSecurityEventsLogged | TestSecurityEventsLogged (`auth_test.go`) | The refused admin call `GET /api/users` → `UserAdminService.ListUsers`; `authz.denied` is now logged by the rpc admin interceptor. |

### `auth_resources_test.go` (13 tests)

| Old test | New test | What changed |
|---|---|---|
| TestGetUser | TestGetUser (`auth_resources_test.go`) | `GET /api/users/{id}` → `UserAppService.GetUser` for one's own account, `UserAdminService.GetUser` for an admin. Intended change (plan step 3＋4): `UserAppService.GetUser` is self-only, so an admin asking it for another user gets `not_found`; admins read others through `UserAdminService.GetUser` (the REST `getUser` let admins read anyone). A non-UUID id was 422 `format` at `/id`, now `INVALID` on `id`; an unknown id on the admin side is `not_found`. |
| TestGetOtherUserNotFound | TestGetOtherUserNotFound (`auth_resources_test.go`) | 404 problem → `not_found`, the same error as a missing id (`assertSameError`); anonymous → `unauthenticated`. |
| TestListUsers | TestListUsers (`auth_resources_test.go`) | `limit`/`cursor` → `page_size`/`page_token`; `next_cursor: null` → empty `next_page_token`; `is_disabled` → `disabled`; bob is disabled through UpdateUser. Still newest first, disabled users included. |
| TestNonAdminCannotListUsers | TestNonAdminCannotListUsers (`auth_resources_test.go`) | `permission_denied` `INSUFFICIENT_ROLE`; anonymous `unauthenticated` `SESSION_INVALID`. |
| TestCreateSessionReturnsSession | TestCreateSessionReturnsSession (`auth_resources_test.go`) | 201 with `Location` → `{session}`, read back through `SessionAppService.GetSession` with the cookie the response set (the descriptor check requires CreateSession's GetSession in `app.sessions.v1`); `created_at`/`last_used_at`/`expires_at` → `create_time`/`last_use_time`/`expire_time`, still now, now and now + 7 days. |
| TestGetCurrentSession | TestGetCurrentSession (`auth_resources_test.go`) | `GET /api/sessions/current` → `GetCurrentSession`; `GET /api/sessions/{id}` → `GetSession`; anonymous → `unauthenticated`. |
| TestListSessions | TestListSessions (`auth_resources_test.go`) | Paging as for users; `current` marks only the request's session. |
| TestListSessionsExcludesExpired | TestListSessionsExcludesExpired (`auth_resources_test.go`) | 404 → `not_found` for GetSession and DeleteSession of an expired session. |
| TestDeleteSession | TestDeleteSession (`auth_resources_test.go`) | `DELETE /api/sessions/{id}` → `DeleteSession`: another of one's sessions gets no `Set-Cookie`, the current one gets the clearing cookie (plan mutation "DeleteSession 對非目前的 session 也清 cookie"). New: a non-UUID id is `INVALID` on `id`. |
| TestDeleteOtherUsersSession | TestDeleteOtherUsersSession (`auth_resources_test.go`) | `not_found` for DeleteSession and GetSession of another user's session; new: the same error as a missing id (`assertSameError`). |
| TestDeleteCurrentUser | TestDeleteCurrentUser (`auth_resources_test.go`) | `DELETE /api/users/me` → `UserAppService.DeleteCurrentUser`; 403 `reauthentication_required` → `permission_denied` `REAUTHENTICATION_REQUIRED`; clearing cookie, items gone, audit row, the name is free again. |
| TestDeleteCurrentUserLastAdmin | TestDeleteCurrentUserLastAdmin (`auth_resources_test.go`) | `failed_precondition` `LAST_ADMIN`. |
| TestAdminDeletesUser | TestAdminDeletesUser (`auth_resources_test.go`) | `DELETE /api/users/{id}` → `UserAdminService.DeleteUser`; the deleted user is `not_found` through `UserAdminService.GetUser`; `LAST_ADMIN`; a stale admin session `REAUTHENTICATION_REQUIRED`. New: a non-UUID id is `INVALID` on `id`. |

Also in `auth_resources_test.go`: TestDefaultPageSizeMatchesContract (an `items_test.go` snapshot test, moved here in step 2) is replaced by TestListUsersPageParameters and TestListSessionsPageParameters: the page sizes are in the proto comments, not a contract default, and both lists are read with 101 extra rows. `page_size` 0 is 20; intended change (AIP-158): 101 and 1000 are read as 100 (the REST `limit > 100` was 422 `out_of_range`); negative is `OUT_OF_RANGE` on `page_size`; a token this API did not return is `INVALID` on `page_token`.

### `throttle_test.go` (5 tests)

| Old test | New test | What changed |
|---|---|---|
| TestSignInThrottlePerClient | TestSignInThrottlePerClient (`throttle_test.go`) | 429 → `resource_exhausted` `LOGIN_THROTTLED`; the client address is the request's peer (`fromPeer`). |
| TestUntrustedClientsLockedTrustedDeviceNot | TestUntrustedClientsLockedTrustedDeviceNot (`throttle_test.go`) | The device cookie is read from the response headers, attributes unchanged (HttpOnly, Secure, SameSite=Strict, Path=/, 180 days); the lock is `resource_exhausted` `LOGIN_THROTTLED`. New test next to it: TestFirstSignInSetsTwoCookies, a browser's first sign-in gets exactly two `Set-Cookie` on the one response, the session and the device cookie, and a sign-in carrying the device cookie gets only the session (plan mutation "拿掉 device cookie 的 Header().Add"). |
| TestDeviceCookieIsPerAccount | TestDeviceCookieIsPerAccount (`throttle_test.go`) | Same through the client. |
| TestSignOutEverywhereForgetsDevices | TestSignOutEverywhereForgetsDevices (`throttle_test.go`) | `DELETE /api/sessions` → `DeleteAllSessions`. |
| TestAdminUnlocksSignIn | TestAdminUnlocksSignIn (`throttle_test.go`) | `GET` and `DELETE /api/users/{id}/sign-in-lock` → `GetSignInLock` and `ResetSignInLock` (AIP-136 custom method); `failures` (now `int32`) is checked too, 0 before and 100 after the lock-out; the unlock is audited. New: an unknown `user_id` is `not_found`, a non-UUID one `INVALID` on `user_id`, for both. |

### `ratelimit_test.go` (5 tests)

| Old test | New test | What changed |
|---|---|---|
| TestRateLimitPerIPBehindTrustedProxy | TestRateLimitPerIPBehindTrustedProxy (`ratelimit_test.go`) | The REST stub operation behind `limitOperations` (deleted) → the test Public service's UpdatePublicThing limited at the Connect entry (`Deps.testLimited`); 429 problem with `Retry-After: 30` → `resource_exhausted` with a 30-second `RetryInfo`; the clock is an injected `testClock` (was `time.Now`). |
| TestRateLimitIgnoresXFFFromUntrustedPeer | TestRateLimitIgnoresXFFFromUntrustedPeer (`ratelimit_test.go`) | Same setup; a forged `X-Forwarded-For` from an untrusted peer still shares the peer's bucket. |
| TestRateLimitOnlyNamedOperations | TestRateLimitOnlyNamedOperations (`ratelimit_test.go`) | GetPublicThing (not in the limit) keeps answering after UpdatePublicThing is exhausted, which is now asserted too. |
| TestIPLimiterRefillsAndSweeps | TestIPLimiterRefillsAndSweeps (`ratelimit_test.go`) | Unchanged (the token bucket itself). |
| TestIPLimiterKeysIPv6By64 | TestIPLimiterKeysIPv6By64 (`ratelimit_test.go`) | Unchanged. |

### `authz_test.go` (3 tests)

| Old test | New test | What changed |
|---|---|---|
| TestAdminOperationsChecksTable | retired | `operationAccess` and `adminOperations` were deleted: a service's class comes from its name (`rpc.ClassOf`), so there is no table to drift from the contract. `NewHandler` refuses a service whose name gives no class (TestNewHandlerRejectsUnclassifiedService), a side effect on GET (TestNewHandlerRejectsSideEffectsOnGet) and the rest of `rpc.NewMux`'s checks (TestNewMuxRejects, TestClassOf in `rpc/`). |
| TestOperationAccessEnforced | TestProcedureAccessEnforced (`authz_test.go`) | One subtest per REST operation in the contract → one per procedure `connectServices` mounts (22, items included): anonymous App and Admin procedures are 401 `unauthenticated` `SESSION_INVALID`; an Admin procedure with a plain user's session is 403 `permission_denied` `INSUFFICIENT_ROLE` before its handler; a Public procedure answered anonymously is not 401, 403 or 404. A new service or method is covered without a change to the test. Complements TestUnauthenticatedBeforeValidation and TestItemsNeedSession. New test next to it: TestRolesMapped (no database): every role users.role's CHECK allows is one the auth module defines (`auth.RoleAdmin`, `auth.RoleUser`) and is sent as its own proto `Role`, never `ROLE_UNSPECIFIED`. |
| TestAdminChangesAreAudited | TestAdminChangesAreAudited (`authz_test.go`) | The state-changing admin operations were the non-GET entries of `operationAccess`; now they are the mounted Admin-class procedures (`rpc.ClassOf`) not marked `NO_SIDE_EFFECTS`, and the cases (UpdateUser → `user.disabled`, DeleteUser → `user.deleted`, ResetSignInLock → `user.unlocked`) must equal that list both ways, so a new Admin write without a case fails (backend/AGENTS.md rule 11). |

### `queries_test.go` (1 test)

| Old test | New test | What changed |
|---|---|---|
| TestListQueriesDoNotGrowWithRows | TestListQueriesDoNotGrowWithRows (`queries_test.go`) | The sessions and users lists are Connect GETs (`ListSessions`, `ListUsers`, `page_size` 100), like items in step 2; items too now go through the generated client (GET) instead of a hand-written URL. |

### `server_test.go` (8 tests)

| Old test | New test | What changed |
|---|---|---|
| TestHealth | TestHealth (`server_test.go`) | Unchanged; stays after step 5 (`/health` is outside `/api` and the REST layer). |
| TestReady | TestReady (`server_test.go`) | Unchanged; stays after step 5. |
| TestReadyDBDown | TestReadyDBDown (`server_test.go`) | Unchanged in step 3＋4. The 503 body is a problem (`writeProblem`), so step 5, which deletes the problem writer, chooses its new body; the status stays. |
| TestShutdownFinishesInFlightRequest | TestShutdownFinishesInFlightRequest (`server_test.go`) | Unchanged; stays after step 5. |
| TestBodyTooLarge | TestBodyTooLarge (`server_test.go`), TestConnectBodyTooLarge (`connect_test.go`) | Unchanged in step 3＋4 for REST (deleted in step 5). New: TestConnectBodyTooLarge covers the limit on the Connect path: a body over `maxBodyBytes` is `resource_exhausted` (HTTP 429) without `RetryInfo`, which tells it from the rate limit (`server.go` `protect`, B16). |
| TestCrossOriginPostRejected | TestCrossOriginPostRejected (`server_test.go`) | Unchanged in step 3＋4 (REST layer, deleted in step 5); Connect: TestCrossOriginRejectedInEachFormat, TestCreateItemCrossSite. CrossOriginProtection stays, so step 5 rewrites the table's expectation to the Connect error. |
| TestSlowHeaderClosed | TestSlowHeaderClosed (`server_test.go`) | Unchanged; stays after step 5. |
| TestUnknownPathNotFound | TestUnknownPathNotFound (`server_test.go`) | Unchanged in step 3＋4 (REST layer, deleted in step 5); Connect: TestConnectUnknownProcedureNotFound. Step 5 chooses the body of a 404 outside `/api/app.`. |

### `middleware_test.go` (9 tests)

| Old test | New test | What changed |
|---|---|---|
| TestRequestIDFromTrustedProxy | TestRequestIDFromTrustedProxy (`middleware_test.go`) | Unchanged in step 3＋4. The middleware stays, but the test drives the REST stub (`newStubAPI` over `testdata/test-openapi.yaml`), which step 5 deletes: step 5 moves it to the Connect test services (`newConnectAPI`). |
| TestRequestIDFromUntrustedPeerReplaced | TestRequestIDFromUntrustedPeerReplaced (`middleware_test.go`) | Same as the row above. |
| TestRequestIDMalformedReplaced | TestRequestIDMalformedReplaced (`middleware_test.go`) | Same as the row above. |
| TestProblemCarriesRequestID | TestProblemCarriesRequestID (`middleware_test.go`) | Unchanged in step 3＋4; retire in step 5: Connect error bodies carry no `request_id` (B17), and `decodeWireError` checks in every Connect wire test that the body has none and `X-Request-ID` is set. |
| TestPanicLogAndAccessLogCarryRequestID | TestPanicLogAndAccessLogCarryRequestID (`middleware_test.go`) | Unchanged in step 3＋4; drives the REST stub's `/api/fail`, so step 5 moves it to a Connect test service that panics (TestConnectHandlerErrors has one). |
| TestSecurityHeaders | TestSecurityHeaders (`middleware_test.go`) | Unchanged in step 3＋4; drives the REST stub, so step 5 moves it; Connect: TestConnectPublicServiceThroughNewHandler checks `Cache-Control: no-store`. |
| TestClientIP | TestClientIP (`middleware_test.go`) | Unchanged; a unit test of `trust.clientIP`, stays after step 5. |
| TestAccessLogAbortBeforeWriting | TestAccessLogAbortBeforeWriting (`middleware_test.go`) | Unchanged; drives `protect` directly (with `validate_test.go`'s `do`, which step 5 must keep or replace). |
| TestResponseControllerReachesConnection | TestResponseControllerReachesConnection (`middleware_test.go`) | Unchanged; same as the row above. |

### `problem_test.go` (8 tests)

| Old test | New test | What changed |
|---|---|---|
| TestAppErrToProblem | TestAppErrToProblem (`problem_test.go`) | Unchanged in step 3＋4 (REST layer, deleted in step 5); Connect: TestConvert (`rpc/`) maps each kind to its code. |
| TestAppErrValidation | TestAppErrValidation (`problem_test.go`) | Changed in step 3＋4 by the B27 rewrite, not by the move to Connect: `apperr.Invalid` takes the field name (`password`) and the REST writer still answers pointer `/password`. REST layer, deleted in step 5; Connect: TestConvertValidation (`rpc/`), TestCreateUserLimits. |
| TestRateLimitedRetryAfter | TestRateLimitedRetryAfter (`problem_test.go`) | Unchanged in step 3＋4; retire in step 5: the `Retry-After` header becomes `RetryInfo` (TestConvertRateLimited in `rpc/`, TestConnectRateLimitBeforeDecoding, TestRateLimitAuthOperations). |
| TestUnhandledErrorHidden | TestUnhandledErrorHidden (`problem_test.go`) | Unchanged in step 3＋4 (REST layer, deleted in step 5); Connect: TestConnectHandlerErrors, TestConvertUnexpectedIsInternal, TestInternalErrorLogged. |
| TestPanicReturnsProblem | TestPanicReturnsProblem (`problem_test.go`) | Unchanged in step 3＋4. The outer `recoverPanics` stays (B7) but writes through `writeError`, which after step 5 only writes Connect errors: step 5 rewrites the expected body. Connect: TestConnectHandlerErrors (a panicking handler). |
| TestPanicAbortHandlerPropagates | TestPanicAbortHandlerPropagates (`problem_test.go`) | Unchanged; stays after step 5 (net/http recovery, B7). |
| TestPanicAfterResponseStartedAborts | TestPanicAfterResponseStartedAborts (`problem_test.go`) | Unchanged; stays after step 5; its "no problem appended" string check becomes "no error appended". |
| TestErrorAfterResponseStartedAborts | TestErrorAfterResponseStartedAborts (`problem_test.go`) | Unchanged; stays after step 5 (`writeError` aborts a started response in both formats). |

### `validate_test.go` (12 tests)

All test the kin-openapi validator over the stub contract `testdata/test-openapi.yaml` (B31): unchanged in step 3＋4, deleted with `validate.go` in step 5.

| Old test | New test | What changed |
|---|---|---|
| TestValidRequestReachesHandler | TestValidRequestReachesHandler (`validate_test.go`) | Retire in step 5: the validator letting a valid body through; every Connect client test covers the Connect path. |
| TestValidationMinLength | TestValidationMinLength (`validate_test.go`) | Retire in step 5: limits are in the services (D2); Connect: TestCreateUserLimits, TestCreateSessionLimits, TestCreateItemTitleLimits. |
| TestValidationCodes | TestValidationCodes (`validate_test.go`) | Retire in step 5: the JSON-Schema keyword → code table (`schemaCodes`) goes with kin-openapi; the services choose `FieldViolationReason` values (TestEveryAppErrCodeIsDeclared). |
| TestValidationListsEveryError | TestValidationListsEveryError (`validate_test.go`) | Retire in step 5: Connect: TestCreateUserLimits and TestCreateSessionLimits (several violations in one `BadRequest`), TestConvertValidation (`rpc/`). |
| TestUnsupportedMediaType | TestUnsupportedMediaType (`validate_test.go`) | Retire in step 5: connect-go answers other content types itself; Connect: TestConnectUnauthenticatedBeforeDecoding and TestUnauthenticatedBeforeValidation (`text/plain`, anonymous → `unauthenticated`). |
| TestMalformedJSON | TestMalformedJSON (`validate_test.go`) | Retire in step 5: Connect: TestConnectUnauthenticatedBeforeDecoding and TestUnauthenticatedBeforeValidation (a Public method answers `{not json` with `invalid_argument`). |
| TestBodyReadErrorIsNotValidation | TestBodyReadErrorIsNotValidation (`validate_test.go`) | Retire in step 5: the REST body read path goes; connect-go reads the body. |
| TestUndefinedRoute | TestUndefinedRoute (`validate_test.go`) | Retire in step 5: Connect: TestConnectUnknownProcedureNotFound. |
| TestValidatedBodyTooLarge | TestValidatedBodyTooLarge (`validate_test.go`) | Retire in step 5; the Connect path is TestConnectBodyTooLarge (see TestBodyTooLarge). |
| TestPanicInOperationReturnsProblem | TestPanicInOperationReturnsProblem (`validate_test.go`) | Retire in step 5: Connect: TestConnectHandlerErrors. |
| TestHealthOutsideValidation | TestHealthOutsideValidation (`validate_test.go`) | Retire in step 5: there is no validator to stay outside of; TestHealth stays. |
| TestDocs | TestDocs (`validate_test.go`) | Retire in step 5: the dev docs and Swagger UI are deleted, not replaced (B20, plan 不做). |

### `bind_test.go` (2 tests)

| Old test | New test | What changed |
|---|---|---|
| TestParamBindErrorIsValidation | TestParamBindErrorIsValidation (`bind_test.go`) | Unchanged in step 3＋4; retire in step 5: the generated-parameter binder (B5) goes; a Connect request has no path or query parameters, only its message. |
| TestBodyDecodeError | TestBodyDecodeError (`bind_test.go`) | Unchanged in step 3＋4; retire in step 5: connect-go decodes the body (TestConnectUnauthenticatedBeforeDecoding's Public case). |

### `problem_contract_test.go` (2 tests)

| Old test | New test | What changed |
|---|---|---|
| TestProblemValuesInContract | TestProblemValuesInContract (`problem_contract_test.go`) | Changed only to compile: `errInsufficientRole` (deleted `authz.go`) → `rpc.ErrInsufficientRole`. Retire in step 5 with the contract; Connect: TestEveryAppErrCodeIsDeclared (`rpc/codes_test.go`, step 6), which reads the code of every `apperr.New`/`Invalid` call and `apperr.Error` literal from the source. |
| TestValuesOutsideContract | TestValuesOutsideContract (`problem_contract_test.go`) | Unchanged in step 3＋4; retire in step 5 with `checkProblems`. |

### `spec_test.go` (1 test)

| Old test | New test | What changed |
|---|---|---|
| TestEveryWritablePathHasGet | TestEveryWritablePathHasGet (`spec_test.go`) | The contract has no paths left; the self-check, which removed GET from `/sessions/{id}` (a path that left the contract), now adds `/widgets/{id}` with only DELETE. Retire in step 5 with the contract; Connect: TestConnectServicesReturnTheirResources. |

Also changed in step 3＋4, outside these files' snapshot tests:

- `connect_test.go`: `resourceProblems` looks for `Get<X>` among the mounted services of the method's proto package, not only its own service (sign-up and sign-in are Public, their reads App). TestResourceProblemsReportsViolations also requires that a CreateUser whose only GetUser is in another package (`app.admin.users.v1`) is reported, and that UserAppService's GetUser satisfies it. `wireError` decodes `ErrorInfo.reason` for raw wire tests.
- `connect_auth_test.go`: TestAuthenticateDatabaseErrorInEachFormat checks GetCurrentUser too, and its REST half uses a REST path (`/api/rest-path`), since `GET /api/users/me` left the contract.
- `items_test.go`: compile only (`u.Id.String()` → `u.GetId()`, the user is now a `usersv1.User`).

## Step 3＋4: `backend/cmd/app/` (6 tests, now 7)

| Old test | New test | What changed |
|---|---|---|
| TestHealthcheck | TestHealthcheck (`main_test.go`) | Unchanged: `app healthcheck` probes `/health`, outside the API. |
| TestProbeAddr | TestProbeAddr (`main_test.go`) | Unchanged. |
| TestSeedAdminCreatesAdmin | TestSeedAdminCreatesAdmin (`seed_test.go`) | Unchanged. |
| TestSeedAdminRejectsShortPassword | TestSeedAdminRejectsShortPassword (`seed_test.go`) | The rules no longer come from the contract's `CreateUserRequest` (kin-openapi `VisitJSON` over the embedded spec, inventory B28) but from `auth.Service.EnsureAdmin`, which runs the checks `UserPublicService.CreateUser` runs (D2-A, `auth/limits.go`). The message names the variable, the rule and the code (`ADMIN_PASSWORD: password must be at least 15 characters (too_short)`), never the value; every invalid field is listed (new assertion: account and password both invalid name both variables). |
| TestSeedAdminExistingAccountUnchanged | TestSeedAdminExistingAccountUnchanged (`seed_test.go`) | Unchanged. |
| TestRecoverAdmin | TestRecoverAdmin, TestRecoverAdminRejectsShortPassword (`seed_test.go`) | Unchanged; new: `recover-admin` checks its input through `auth.Service.RecoverAdmin` with the same rules, before it changes the password. |

## Step 3＋4, frontend: the 9 test files of the snapshot (auth calls now go through the generated Connect clients)

Paths are under `frontend/`. Every mocked call now goes out the way connect-web sends it, `fetch(url, init)` to `/api/<package>.<Service>/<Method>` (a GET with `?connect=v1&encoding=json&message=…` for methods marked `NO_SIDE_EFFECTS`, otherwise a POST with a JSON body). Errors are Connect's JSON `{code, message, details}`, and each detail is `{type, value}` with `value` the base64 of the binary proto. Tests build these responses from the generated schemas in `src/lib/api/__tests__/connect-wire.ts` (`connectResponse`, `connectError`, `requestOf`). Messages moved from `problems.types/codes/fields` to `errors.codes.<Connect code>`, `errors.reasons.<ErrorReason>`, `errors.fields.<FieldViolationReason>` (D3-A, inventory F10).

| Old test | New test | What changed |
|---|---|---|
| `frontend/src/lib/api/__tests__/client.test.ts` | Retired along with `src/lib/api/client.ts` (openapi-fetch). Its 6 tests: "calls the API on the page's own origin under /api" → `transport-call.test.ts` "calls reads with GET on the page's origin under /api, with a timeout" (step 1). "sends a JSON body" → `auth-form.test.tsx` "leaves length rules to the API" and `delete-account.test.tsx` "signs in again, deletes the account and leaves for the login page". "sends the user to login on a 401 without trying to refresh" → `transport-call.test.ts` "a failed call > sends the user to sign in when the session is gone, without trying again" (new). "leaves a 401 from signing in to the form" → `auth-form.test.tsx` "sign-in failure stays on the login page and shows the error" and `transport.test.ts` "leaves a failed sign-in on its form" (step 1). "returns the data, and undefined for 204" → retired: there is no `unwrap` any more, and the generated client always returns a typed message. Empty responses (`DeleteCurrentUser`) are covered by the delete-account test. "turns a body that is not a problem into a generic one" → `transport-call.test.ts` "a failed call > turns a response that is not a Connect error into the code of its status" (new) and `connect-errors.test.ts` "never shows the message, even for an error that is not a Connect error". | REST `Request`s to `/api/users/me` and `/api/sessions` with problem+json errors → Connect calls through `src/lib/api/transport.ts`. A 401 is now `unauthenticated`. The redirect exemption is now `ErrorInfo.reason` `INVALID_CREDENTIALS` instead of `POST /sessions` (plan step 1). The new redirect test goes through the real transport, so removing the interceptor from the transport now fails a test; before, only the interceptor's own unit tests covered it. Intended change: a proxy's HTML 502 used to become `about:blank` and showed the generic "Something went wrong" message. connect-web now maps the status to `unavailable`, so the user sees "The service is temporarily unavailable." |
| `frontend/src/lib/api/__tests__/errors.test.ts` | Retired along with `src/lib/api/errors.ts`, replaced by `connect-errors.test.ts` (new): "errorMessage > translates the reason, not the message" (was "translates the code"), "errorMessage > falls back to the code when the reason has no message" (was "falls back to the type when the code has no message"), "errorMessage > never shows the message, even for an error that is not a Connect error" (was "never shows detail, even for an unknown type"), "violationMessage > translates the reason, and one this client does not know as INVALID" plus `transport.test.ts` "maps each invalid field to its reason" (step 1) (together they replace "keys errors by body property and translates their codes"). New: "codeName > names each code as the Connect protocol does on the wire". | The lookup order was problem `code`, then `type`. It is now `ErrorInfo.reason`, then the Connect code (D3-A). The developer `message` replaces `detail` and is still never shown. Anything that is not a Connect error reads as code `unknown`, where it used to read as `about:blank`. Field errors were keyed by JSON Pointer (`/password`); violations are now keyed by field path (`password`). Field messages were lowercase codes (`password_too_common`, fallback `invalid`); they are now `FieldViolationReason` names (`PASSWORD_TOO_COMMON`, fallback `INVALID`). The empty-pointer case (`""`, a request without a body) has no Connect equivalent: connect-go answers an unreadable body with `invalid_argument` and no violations, which shows the `invalid_argument` message. |
| `frontend/src/lib/api/__tests__/messages.test.ts` | `messages.test.ts` "error messages > en translates exactly the contract's codes and reasons" and "… > zhTW translates exactly the contract's codes and reasons" | The compile-time half used to read `ProblemTypeSlug`, `ProblemCode` and `FieldErrorCode`, built from `schema.d.ts` by `scripts/gen-api.mjs`. It now reads `CodeName` (every Connect `Code`, in snake_case), `ErrorReasonName` and `FieldViolationReasonName` (the generated enums without `*_UNSPECIFIED`) from `src/lib/api/connect-errors.ts`. A proto value without a translation in either locale fails `pnpm lint`. Mutation: removing `errors.reasons.LAST_ADMIN` from `en.json` fails `tsc` with TS2322 at `messages.test.ts:25`. The runtime half is stricter: each locale's keys must equal the contract's names exactly. Before, it only checked that the two locales had the same keys. Now one test per locale. |
| `frontend/src/lib/api/__tests__/server-base.test.ts` | `server-base.test.ts` "sends server-side calls to an unresolvable host when the site URL is unset" | `api.GET("/users/me")` on the REST client → `GetCurrentUser` through the new `src/lib/api/server-transport.ts`, carrying a cookie. The test makes the same `.invalid` hostname assertion, on the request the transport sends. `transport-server.test.ts` (step 1) keeps the unit check on `apiBaseUrl()`. |
| `frontend/src/features/auth/__tests__/server.test.ts` | The same five tests. One is renamed: "forwards only the request cookies to /api/users/me and returns the user" → "forwards only the request cookies with a GET to GetCurrentUser and returns the user". | URL `https://app.example.com/api/users/me` → a GET to `https://app.example.com/api/app.users.v1.UserAppService/GetCurrentUser`, a `NO_SIDE_EFFECTS` read. The test asserts the method, the path, the `cookie` header and the absence of `x-forwarded-for`, `x-real-ip` and `x-origin-secret`. It now also asserts `cache: "no-store"`, which was passed before but never checked (plan 驗收, line 181). 401 problem → `unauthenticated` with `SESSION_INVALID` → null. 500 problem → Connect `internal`, so the test checks `code: Code.Internal` instead of `status: 500`. The user is now a generated `User` message: `role` (enum), `disabled`, `create_time` replace `role` (string), `is_disabled`, `created_at`. Mutation: removing `headers: { cookie }` from `server.ts` fails the cookie assertion. |
| `frontend/src/features/auth/__tests__/auth-form.test.tsx` | Same names: "renders account and password fields", "requires both fields without calling the API", "leaves length rules to the API", "shows a field error from the API next to its field, translated". Renamed: "shows a wrong password as a translated message, not the detail" → "sign-in failure stays on the login page and shows the error". | Sign-up: `POST /api/users` 201 → POST `/api/app.users.v1.UserPublicService/CreateUser`. The test now also asserts the JSON content type and body, which takes over client.test's "sends a JSON body". Field error: a 422 with pointer `/password` and code `password_too_common` → `invalid_argument` with a `BadRequest` violation on `password`, reason `PASSWORD_TOO_COMMON`. It now also asserts that the form shows no general alert. Wrong password: a 401 with `invalid_credentials` → `unauthenticated` with `ErrorInfo` `INVALID_CREDENTIALS`. It now also asserts that the call went to `CreateSession` and that the transport did not redirect (`location.href` unchanged), so the plan's mutation (redirecting on `INVALID_CREDENTIALS`) fails this test. The required-field message key `problems.fields.required` is now `errors.fields.REQUIRED`: the zod message is now the `FieldViolationReason` name. The form renders inside connect-query's `TransportProvider`, as in the app. |
| `frontend/src/features/auth/__tests__/delete-account.test.tsx` | "signs in again, deletes the account and leaves for the login page" (same name). Renamed: "shows the problem and deletes nothing when the password is wrong" → "shows the error and deletes nothing when the password is wrong". | `POST /api/sessions` then `DELETE /api/users/me` (204) → POST `/api/app.sessions.v1.SessionPublicService/CreateSession` with the same JSON body, then POST `/api/app.users.v1.UserAppService/DeleteCurrentUser`, which returns an empty message. A 401 `invalid_credentials` problem → `unauthenticated` with `INVALID_CREDENTIALS`. The test now also asserts that the page did not redirect. |
| `frontend/src/features/auth/__tests__/callback.test.ts` | Unchanged (same file, same tests) | It makes no API call, so there is no wire to rewrite. |
| `frontend/src/__tests__/proxy.test.ts` | Unchanged (same file, same tests) | It makes no API call. The cookie names and the CSP are unchanged (plan 不能動, inventory F13). |

Tests added in step 3＋4 that replace no snapshot test: `use-auth.test.tsx` "after login the current user is the new account, without fetching it again" and "after register …" both check that `setQueryData` uses the key of connect-query's `GetCurrentUser` query. Dropping `input` from that key fails both. The two `transport-call.test.ts` "a failed call > …" tests and the five `connect-errors.test.ts` tests are listed in the rows above. The frontend total stays at 50: the snapshot's 41, plus 9 from step 1, minus 10 retired (client 6, errors 4), plus 10 added (connect-errors 5, use-auth 2, transport-call 2, messages +1).

## Step 5: the REST shell deleted

The rows above that say "retire in step 5" or "step 5 chooses" end here. Every error the net/http layer writes is now a Connect error (`writeError` has no REST branch), checked with `decodeWireError`, which also checks that the request id is only in `X-Request-ID` (B17). The stub around `assemble` (`newStubAPI`, `do`) moved from `validate_test.go` to `handler_test.go` and serves a stub in place of the Connect services.

| Test (state at `3e85b7f`) | After step 5 | What changed |
|---|---|---|
| TestReadyDBDown | TestReadyDBDown (`server_test.go`) | The 503 body was a problem; now `{"status":"unavailable"}`, the same shape as the 200 bodies (`/health*` is outside the contract). |
| TestBodyTooLarge | TestBodyTooLarge (`server_test.go`) | Checks only that `protect` caps the body: the stub handler answers 413 on `*http.MaxBytesError`. The Connect answer (`resource_exhausted` without `RetryInfo`) stays in TestConnectBodyTooLarge. |
| TestCrossOriginPostRejected | TestCrossOriginPostRejected (`server_test.go`) | 403 problem with code `cross_origin_request` → `permission_denied` with `CROSS_ORIGIN_REQUEST`. |
| TestUnknownPathNotFound | TestUnknownPathNotFound (`server_test.go`) | 404 problem → `not_found`, for `/nope`, `/health/`, `/api/nope` and `/api/`. |
| TestProblemCarriesRequestID | TestErrorCarriesRequestIDHeader (`middleware_test.go`) | The body has no `request_id`; the header carries it. |
| TestSecurityHeaders | TestSecurityHeaders (`middleware_test.go`) | Its `problem` case is now `error` (a Connect `not_found`). |
| TestAppErrToProblem, TestAppErrValidation, TestRateLimitedRetryAfter | retired | The problem writer is gone; the Connect mapping is TestConvert, TestConvertValidation and TestConvertRateLimited (`rpc/`). |
| TestUnhandledErrorHidden | TestUnhandledErrorHidden (`errors_test.go`, was `problem_test.go`) | Asserts a Connect `internal` without the error's text. |
| TestPanicReturnsProblem | TestPanicReturnsInternal (`errors_test.go`) | Asserts a Connect `internal`. |
| TestPanicAbortHandlerPropagates, TestPanicAfterResponseStartedAborts, TestErrorAfterResponseStartedAborts | same names (`errors_test.go`) | Moved with the file; "no problem appended" is "no error appended". |
| TestValidRequestReachesHandler, TestValidationMinLength, TestValidationCodes, TestValidationListsEveryError, TestUnsupportedMediaType, TestMalformedJSON, TestBodyReadErrorIsNotValidation, TestUndefinedRoute, TestValidatedBodyTooLarge, TestPanicInOperationReturnsProblem, TestHealthOutsideValidation, TestDocs | retired with `validate_test.go` | As each row in `validate_test.go` above says. |
| TestParamBindErrorIsValidation, TestBodyDecodeError | retired with `bind_test.go` | As in `bind_test.go` above. |
| TestProblemValuesInContract, TestValuesOutsideContract | retired with `problem_contract_test.go` | TestEveryAppErrCodeIsDeclared covers the Connect codes. |
| TestEveryWritablePathHasGet | retired with `spec_test.go` | TestConnectServicesReturnTheirResources. |
| TestAuthenticateDatabaseErrorInEachFormat | TestAuthenticateDatabaseErrorIsInternal (`connect_auth_test.go`) | The REST half is gone. |
| TestCursorRoundTrip, TestCursorTamperedIsInvalid (`internal/platform/httpx`) | TestPageAfter (`rpc/rpc_test.go`) | The page token encoding moved into `rpc` (`PageToken`, `PageAfter`): a round trip; padded base64 and one byte short are `INVALID` on `page_token`. `httpx` is deleted; the row limit and trim moved to `internal/platform/keyset` (new TestTrim). The service decodes no token any more (S22, S34). |

Backend test count (lock): 303 at `3e85b7f` − 45 (the retired tests above with their subtests, counted by running them at `3e85b7f`) + 1 (TestTrim) = 259.
