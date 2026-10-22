---
title: 帳號的濫用防護與生命週期：最後一個 admin、稽核、刪除帳號、依來源節流與資料庫加密
date: 2026-10-22
review-date: 2027-04-22
status: active
tags: [fullstack-template, decision, auth, security, privacy]
---

# 帳號的濫用防護與生命週期：最後一個 admin、稽核、刪除帳號、依來源節流與資料庫加密

## Context

認證的威脅建模（`docs/security/threat-model-auth.md`）與個資盤點（`docs/privacy/data-inventory.md`）列出五個沒處理的項目：

- admin 能停用最後一個 admin，包括自己，之後只能直接改資料庫救回；
- 沒有稽核紀錄；
- 沒有刪除帳號；
- 只依帳號節流，任何人都能把本人鎖在門外；
- backend 到 PostgreSQL 用 `sslmode=disable`。

範本複製出去後就收不到修正，所以這裡出貨的做法就是每個衍生專案的起點。

**約束**：

- `external`：Atlas 的 row-level security policy 要登入 Atlas 帳號才能用（v1.3.3 實測：「policies are available to logged-in users only」），範本不能要求衍生專案有 Atlas 帳號。
- `external`：沒有寄信服務（ADR api-and-auth D6），所以不能用 email 解鎖或重設密碼。
- `external`：一個 backend 副本、單一 PostgreSQL；模組在同一個程序與資料庫。
- `inherited`：`seed-admin` 不改既有帳號，是我們自己的設計，不限制選項。
- `inherited`：「刪除預設不連帶刪除」寫在 `docs/rules/database-design.md`，是我們自己的規則；本 ADR 修改它。

## Options Considered

- **D1 最後一個 admin**：A 在停用與刪除時守住「至少一個啟用中的 admin」；B 一律禁止對自己操作；C 只加主機上的救援指令。
- **D2 稽核**：A 只寫安全事件日誌；B 日誌加上只能新增的稽核表；C 送外部稽核服務。只能新增的做法：RLS policy（Atlas 要登入）、單表收回權限（會被每次部署的權限設定蓋掉），或由 schema 宣告、`app migrate` 套用。
- **D3 刪除帳號**：A 資料庫連帶刪除，加測試要求每個指向 `users` 的 FK 寫明刪除行為；B 每個模組提供刪除函式，在同一個交易逐一呼叫；C 軟刪除加寬限期。
- **D4 節流**：A 維持只依帳號；B 依「帳號＋來源」，加帳號層級上限；C 是 B 加裝置 cookie；D 加 CAPTCHA。
- **D5 資料庫加密**：A compose 內不加密，其餘一律 `verify-full`，由設定強制；B compose 內也上 TLS；C 只寫文件。

## Decision

- **D1**：A＋C。停用與刪除都先鎖住所有啟用中的 admin，目標是唯一一個就回 409 `last_admin`；`app recover-admin`（`make recover-admin`）替既有的 admin 設新密碼、啟用、結束 session、清掉節流與信任裝置，不建立也不升級帳號。
- **D2**：B。會改變狀態的 admin 操作與 `recover-admin` 在同一個交易寫 `audit_events`；表的 comment `append-only: retain 365 days by at` 讓 `app migrate` 收回 app 角色的 UPDATE、DELETE、TRUNCATE，並刪掉超過保留期的列。登入、登出、被拒絕的 admin 操作寫安全事件日誌；compose 每個服務的日誌輪替。
- **D3**：A。`DELETE /api/users/me`（session 必須是 10 分鐘內登入的，否則 403 `reauthentication_required`；前端設定頁先用密碼重新登入）與 admin 的 `DELETE /api/users/{id}`；指向 `users` 的 FK 一律 `CASCADE` 或 `SET NULL`（`TestForeignKeysToUsersAllowDeletion`）。
- **D4**：C。每個「帳號＋來源」各自指數退避，來源是位址（IPv6 以 /64）或信任裝置；沒有該帳號裝置的來源合計連續失敗 100 次起全部拒絕，直到登入成功、admin 解鎖（`GET`／`DELETE /api/users/{id}/sign-in-lock`）或一天沒有嘗試。登入成功的新瀏覽器得到裝置 cookie（`__Host-device`，只存雜湊，180 天）。
- **D5**：A。連線一律 `sslmode=verify-full`；只有 `DATABASE_PLAINTEXT=same-host` 才接受不加密，範本只在 compose 與本機的 `.env-example` 設定。

## Rationale

- **D1**：GitHub、GitLab 都不讓組織失去最後一個擁有者；檢查與變更在同一個交易，並先鎖住 admin 列，兩個 admin 同時互相停用時只會成功一個。只有 A 時，忘記密碼的唯一 admin 仍救不回來，所以加 C。不選 B：它沒有多擋任何 A 擋不了的情況。
- **D2**：OWASP Logging Cheat Sheet 與 ASVS 5.0 第 16 章要求記錄認證事件、權限失敗與使用者管理，而且紀錄不能被改。稽核表而不只寫日誌，因為容器日誌只是輪替，不能查詢，也擋不住 app 改寫。用表的 comment 宣告，而不是在 `cmd/` 傳設定給 migrate，因為測試資料庫走同一條 `app migrate`，宣告只有 `schema.sql` 一個地方。代價：能拿到 owner 或 superuser 權限的人仍可改紀錄；保留期只在部署時執行。
- **D3**：GDPR 第 17 條；Apple App Store Review Guideline 5.1.1(v) 與 Google Play 都要求 app 內可刪帳號。選 A 而非 B，因為模組在同一個資料庫，由資料庫保證刪乾淨，新模組也忘不了；B 只有在模組把資料存在 PostgreSQL 以外時才值得。重新驗證 10 分鐘內的登入，而不是在 DELETE 帶密碼，因為 RFC 9110 沒有定義 DELETE body 的語意，前端也能沿用登入 API。代價：使用者擁有的資料隨帳號刪除；備份保留 7 天。
- **D4**：OWASP Authentication Cheat Sheet 指出鎖定不能變成阻斷服務；NIST SP 800-63B 3.2.2 的上限是連續失敗 100 次；OWASP「Slow Down Online Guessing Attacks with Device Cookies」讓本人在用過的瀏覽器不受鎖定影響。裝置 token 存雜湊，而不是 OWASP 原文的 HMAC，因為和 session 一樣、不需要新的金鑰，也能在全部登出時作廢。代價：本人在新裝置上仍可能被鎖，要等 admin 解鎖；多一張表與一個 cookie（已記在個資盤點）。
- **D5**：PostgreSQL 文件說明只有 `verify-full` 同時防竊聽與中間人；雲端代管資料庫都要求或預設 TLS。同一台主機的 Docker 網路資料不出主機，所以 compose 內不加密是可接受的取捨。用明確的設定，而不是判斷主機名稱，因為 `make rename`、別名或 IP 都會讓名稱判斷出錯。
- **錯誤碼清單**（實作時發現）：契約把 `Problem.code` 寫成封閉的 `enum`，照 api-design 規則新增 code 一定被 `make api-breaking` 擋下。改用 `x-extensible-enum`（ADR api-and-auth D3 2026-10-22 修訂）。
- **Migration**（實作時發現）：改 FK 與主鍵時 Atlas 產生的 DDL 會鎖表，ADR go-backend D2 修訂為允許手寫不鎖表的步驟。獨立 review 再指出：把 `login_throttles` 的主鍵換成兩欄，上一版 backend 的 `ON CONFLICT (account_key)` 就失效，部署空窗與回退時登入回 500。業界做法是每支 migration 都與上一版相容，拆成 expand 與 contract（Martin Fowler「Parallel Change」、GitLab「Avoiding downtime in migrations」、Stripe「Online migrations at scale」）。根因是 squawk 釘在 2.65.0：2.68.0 起的 `ban-drop-constraint` 就會擋下這支（實測）。所以升 squawk 到 2.70.0、把規則寫進 `docs/rules/database-design.md`，這次改成新表 `sign_in_throttles`，舊表在下一個 PR 刪除。不選「在 CI 用上一版的測試跑新 schema」：只有 GitLab 的未完成提案，CI 時間加倍，且會因與相容無關的原因失敗；改用多副本或零停機部署時再評估。

## Expected Outcome

- 唯一的 admin 停用或刪除自己回 409；所有 admin 都登不進來時，`make recover-admin` 救得回來。
- 每個會改變狀態的 admin 操作都有一列稽核紀錄，app 角色改不了它。
- 使用者能在設定頁刪除帳號，資料庫裡不留它擁有的列。
- 攻擊者換多少位址都鎖不住本人用過的瀏覽器。
- 衍生專案接到遠端資料庫卻沒加密時，backend 與 migrate 拒絕啟動。

## Invariants

- 永遠至少有一個啟用中的 admin，或主機上的操作者能用 `recover-admin` 救回。
- 稽核表對 app 角色只能新增與讀取；新的、會改變狀態的 admin operation 必須寫稽核（`TestAdminChangesAreAudited`）。
- 指向 `users` 的 FK 都寫明刪除行為。
- 節流以「帳號＋來源」為單位；帳號層級的鎖不擋信任裝置。
- 資料庫連線不是 `verify-full` 時，必須明確設定 `DATABASE_PLAINTEXT=same-host`。

## Revocation Triggers

- 加了 email → 鎖定期間可用重設密碼解鎖（OWASP），重新評估 D4 的 admin 解鎖與帳號層級上限。
- 加了 MFA → 重新評估 D4 的上限與裝置 cookie 的價值。
- 某個模組把使用者資料存在 PostgreSQL 以外（檔案儲存、搜尋索引）→ D3 改用逐模組刪除。
- 產品要求刪除有反悔期 → D3 改成軟刪除加排程硬刪除。
- 法規要求稽核紀錄有防竄改證明 → D2 改送外部儲存或加雜湊鏈。
- 資料庫搬到另一台主機，或威脅模型要防同一網路上被入侵的容器 → D5 compose 內也上 TLS。
- Atlas 免費版開放 policy → D2 的 append-only 改用 RLS。

## Review Notes

## Related

- 來源：2026-10-22 方法論簡報，由沒參與的 subagent 核對來源與批判（13 項發現，3 項 HIGH 已採納），repo 擁有者回覆「都照建議」。
- 修訂：ADR api-and-auth D3、D6；ADR go-backend D2；`docs/rules/database-design.md`。
