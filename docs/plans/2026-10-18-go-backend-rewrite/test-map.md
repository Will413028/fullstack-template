# 舊測試 → Go 測試對照

來源：`rg -n "^async def test_|^def test_" backend/tests`（快照 `7a4d0fa`，30 條）。Go 測試名是預定名稱，實作時可改，但這張表的每列都要有對應的 Go 測試或刪除理由。

| 舊測試 | Go 測試 | 改寫重點 |
|---|---|---|
| `test_config.py::test_rejects_invalid_samesite` | `TestConfigRejectsInvalidSameSite` | 原樣 |
| `test_config.py::test_samesite_none_requires_secure` | `TestConfigSameSiteNoneRequiresSecure` | 原樣 |
| `test_config.py::test_samesite_none_with_secure_ok` | `TestConfigSameSiteNoneWithSecure` | 原樣 |
| `test_config.py::test_rejects_placeholder_secret_in_prod` | `TestConfigRejectsDriverSuffixInDatabaseURL` | `SECRET_KEY` 刪除（D3）；改測 `postgresql+asyncpg://` 啟動失敗 |
| `test_config.py::test_short_secret_rejected` | `TestConfigSessionIdleNotAboveAbsolute` | `SECRET_KEY` 刪除；改測 `SESSION_IDLE` > `SESSION_ABSOLUTE` 時失敗 |
| `test_api.py::test_health` | `TestHealth`、`TestReady`、`TestReadyDBDown` | 加 `/health/ready` 成功 body 與 503（E7） |
| `test_auth.py::test_register_sets_cookies` | `TestRegisterSetsSessionCookie` | 斷言只有 `session` cookie、`HttpOnly`、`Path=/` |
| `test_auth.py::test_register_duplicate` | `TestRegisterDuplicate` | 409 problem+json |
| `test_auth.py::test_register_weak_password` | `TestRegisterWeakPassword` | 422，`errors.password` 有訊息 |
| `test_auth.py::test_register_no_uppercase` | `TestRegisterNoUppercase` | 同上 |
| `test_auth.py::test_login_success` | `TestLoginSuccess` | body 改 JSON（P7） |
| `test_auth.py::test_login_wrong_password` | `TestLoginWrongPassword` | 401 |
| `test_auth.py::test_login_nonexistent_user` | `TestLoginUnknownAccountRunsHash` | 401，且注入的 hasher 被呼叫一次（timing 防護） |
| `test_auth.py::test_refresh_rotates_and_revokes_old_token` | `TestLoginRotatesSession` | refresh 刪除；改測再次登入發新 token、舊 token 仍有效到登出（fixation 防護：登入前帶的 token 不被沿用） |
| `test_auth.py::test_refresh_without_cookie` | `TestMeWithoutCookie` 併入 | refresh 刪除；無 cookie 的情境由 `/auth/me` 401 覆蓋 |
| `test_auth.py::test_invalid_refresh_token` | `TestMeWithUnknownToken` | 不存在的 token 401 |
| `test_auth.py::test_logout_clears_session` | `TestLogoutClearsCookie` | `Set-Cookie: session=; Max-Age=0` |
| `test_auth.py::test_logout_revokes_refresh_token` | `TestLogoutDeletesSession` | 登出後舊 token 呼叫 `/auth/me` 401 |
| `test_auth.py::test_me_authenticated` | `TestMeAuthenticated` | 原樣 |
| `test_auth.py::test_me_unauthenticated` | `TestMeWithoutCookie` | 原樣 |
| `test_auth.py::test_me_via_bearer_header` | `TestMeViaBearerSessionToken` | Bearer 帶 session token（P6） |
| `test_auth.py::test_expired_access_token` | `TestSessionIdleExpiry`、`TestSessionAbsoluteExpiry` | 以注入的時鐘推進時間 |
| `test_items.py::test_create_item` | `TestCreateItem` | 原樣 |
| `test_items.py::test_create_item_unauthenticated` | `TestCreateItemUnauthenticated` | 原樣 |
| `test_items.py::test_list_my_items` | `TestListMyItems` | 只回自己的 |
| `test_items.py::test_get_item` | `TestGetItem`、`TestGetOtherUsersItem` | 加 E1：他人 item 404 |
| `test_items.py::test_update_item` | `TestUpdateItem`、`TestUpdateItemTriState` | 加 B14：`description: null` 清空、不送保留；E9：`title: null` 422 |
| `test_items.py::test_delete_item` | `TestDeleteItem` | 原樣 |
| `test_items.py::test_update_other_users_item` | `TestUpdateOtherUsersItem` | 403 → 404（盤點 A11） |
| `test_items.py::test_pagination` | `TestPagination` | 加 E6：跨頁順序穩定 |

新增（舊系統沒有測試的行為）：`TestLoginDisabledAccount`、`TestMeDisabledAccount`（B15）、`TestMeAfterUserDeleted`（E2）、`TestSeedAdmin*`（E8）、`TestRateLimit*`（B6、E3）、`TestRequestID*`（E5）、`TestSecurityHeaders`（E4）、`TestCORSPreflight`、`TestPanicReturnsProblem`、`TestCreateItemTitleTooLong`（E9）。
