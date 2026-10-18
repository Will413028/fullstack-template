# 舊測試 → Go 測試對照

來源：`rg -n "^async def test_|^def test_" backend/tests`，快照 `7a4d0fa`，共 30 條。

- Go 測試名是預定的名稱，實作時可以改。
- 每一列都要有對應的 Go 測試，或寫明刪除理由。
- 新契約見 `inventory.md` 的 A、B 段。

## 舊測試

| 舊測試 | Go 測試 | 改寫重點 |
|---|---|---|
| `test_config.py::test_rejects_invalid_samesite` | `TestConfigRejectsBadTrustedProxies` | `COOKIE_SAMESITE` 已刪除（盤點 D4）。改測 `TRUSTED_PROXIES` 格式錯誤時啟動失敗 |
| `test_config.py::test_samesite_none_requires_secure` | `TestCookieNameFollowsSecure` | Secure 時 cookie 名稱為 `__Host-session`，否則為 `session` |
| `test_config.py::test_samesite_none_with_secure_ok` | 刪除 | SameSite 固定為 Lax，沒有東西可以測 |
| `test_config.py::test_rejects_placeholder_secret_in_prod` | `TestConfigRejectsDriverSuffixInDatabaseURL` | `SECRET_KEY` 已刪除。改測 `postgresql+asyncpg://` 時啟動失敗 |
| `test_config.py::test_short_secret_rejected` | `TestConfigSessionIdleNotAboveAbsolute` | 改測 `SESSION_IDLE` > `SESSION_ABSOLUTE` 時啟動失敗 |
| `test_api.py::test_health` | `TestHealth`、`TestReady`、`TestReadyDBDown`、`TestShutdownFinishesInFlightRequest` | E7、B19 |
| `test_auth.py::test_register_sets_cookies` | `TestCreateUserSetsSessionCookie` | `POST /api/users` 回 201、帶 `Location`、只設一個 session cookie，屬性為 `HttpOnly`、`Path=/`、`SameSite=Lax`，沒有 `Domain` |
| `test_auth.py::test_register_duplicate` | `TestCreateUserDuplicate` | 回 409 problem，`type` 為 `/problems/conflict`、code 為 `account_taken`；大小寫不同的同一個名稱也算重複 |
| `test_auth.py::test_register_weak_password` | `TestCreateUserPasswordTooShort` | 14 字回 422，`errors[0].pointer` 為 `/password`、code 為 `too_short` |
| `test_auth.py::test_register_no_uppercase` | `TestCreateUserNoCompositionRule`、`TestCreateUserCommonPassword` | 15 字以上、全小寫的密碼要成功（NIST 禁止組合規則）；清單內的密碼回 422 code `password_too_common` |
| `test_auth.py::test_login_success` | `TestCreateSession` | `POST /api/sessions` 送 JSON，回 201 加 `User` |
| `test_auth.py::test_login_wrong_password` | `TestCreateSessionWrongPassword` | 回 401，`type` 為 `/problems/unauthenticated` |
| `test_auth.py::test_login_nonexistent_user` | `TestCreateSessionUnknownAccountRunsHash` | 回 401，回應與密碼錯誤相同，且注入的 hasher 被呼叫一次 |
| `test_auth.py::test_refresh_rotates_and_revokes_old_token` | `TestCreateSessionIssuesNewToken` | refresh 已刪除。改測：登入一律發新 token，不沿用請求帶來的 token（防 fixation） |
| `test_auth.py::test_refresh_without_cookie` | 併入 `TestMeWithoutCookie` | refresh 已刪除 |
| `test_auth.py::test_invalid_refresh_token` | `TestMeWithUnknownToken` | 不存在的 token 回 401 |
| `test_auth.py::test_logout_clears_session` | `TestDeleteCurrentSessionClearsCookie` | 回 204，`Set-Cookie` 帶 `Max-Age=0` |
| `test_auth.py::test_logout_revokes_refresh_token` | `TestDeleteCurrentSessionRevokes` | 登出後用舊 token 打 `/api/users/me` 回 401 |
| `test_auth.py::test_me_authenticated` | `TestMe` | body 直接是 `User`，`id` 是 UUID 字串，`role` 是 `"user"` |
| `test_auth.py::test_me_unauthenticated` | `TestMeWithoutCookie` | 原樣 |
| `test_auth.py::test_me_via_bearer_header` | `TestMeIgnoresBearer` | Bearer 已刪除。帶 `Authorization: Bearer <有效 token>`、不帶 cookie，要回 401 |
| `test_auth.py::test_expired_access_token` | `TestSessionIdleExpiry`、`TestSessionAbsoluteExpiry` | 用注入的時鐘推進時間 |
| `test_items.py::test_create_item` | `TestCreateItem` | 回 201 加 `Location` |
| `test_items.py::test_create_item_unauthenticated` | `TestCreateItemUnauthenticated` | 原樣 |
| `test_items.py::test_list_my_items` | `TestListMyItems` | 回 `{items, next_cursor}`，只包含自己的 item |
| `test_items.py::test_get_item` | `TestGetItem`、`TestGetOtherUsersItem` | 加 E1：讀他人的 item 回 404 |
| `test_items.py::test_update_item` | `TestUpdateItem`、`TestUpdateItemMergePatch` | 加 B14：`description: null` 清空、沒送就保留、`title: null` 回 422；Content-Type 不是 `application/merge-patch+json` 時回 415 |
| `test_items.py::test_delete_item` | `TestDeleteItem` | 回 204 |
| `test_items.py::test_update_other_users_item` | `TestUpdateOtherUsersItem` | 403 改為 404（盤點 A11） |
| `test_items.py::test_pagination` | `TestPaginationKeyset` | 加 E6：分頁途中新增資料，結果不重複也不漏；`limit` 超過 100 回 422 |

## 新增（舊系統沒有測試的行為）

- **帳號與權限**：
  - `TestCreateSessionDisabledAccount`、`TestMeDisabledAccount`（B15）
  - `TestMeAfterUserDeleted`（E2）
  - `TestAdminDisablesUserRevokesSessions`、`TestNonAdminCannotPatchUser`（B17、B18）
  - `TestDeleteAllSessions`（B18）
  - `TestSeedAdminRejectsShortPassword`、`TestSeedAdminExistingAccountUnchanged`（E8、B17）
  - `TestPasswordNormalized`：全形與半形版本的同一組密碼可以互相登入（NFKC，B12）
  - `TestCreateSessionMixedCaseAccount`：用大寫帳號登入小寫帳號成功（B12）
- **節流與防護**：
  - `TestLoginThrottlePerAccount`：不存在的帳號也要被節流，回應不能透露帳號是否存在（B6）
  - `TestRateLimitPerIP*`（B6、E3）
  - `TestArgonConcurrencyBounded`（B10）
  - `TestCrossOriginPostRejected`（B21、E12）
- **HTTP 基礎**：
  - `TestRequestID*`（E5）
  - `TestSecurityHeaders`（B4、E4：有 `Cache-Control: no-store`，沒有 `X-XSS-Protection`）
  - `TestBodyTooLarge`（B19，回 413）、`TestSlowHeaderClosed`（B19）
  - `TestUnsupportedMediaType`（回 415）、`TestHealthThroughFullChain`（validator 不擋 `/health`）
  - `TestPanicReturnsProblem`
- **Items**：
  - `TestCreateItemTitleTooLong`（E9）
  - `TestUpdatedAtChanges`（盤點 C 段：`updated_at` 必須由查詢更新）
