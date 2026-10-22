---
title: API 與認證照業界基準：同源 /api、REST 形狀、RFC 9457、opaque session 與 CSRF 防護
date: 2026-10-18
review-date: 2027-04-18
status: active
tags: [fullstack-template, decision, rest, auth, security]
---

# API 與認證照業界基準：同源 /api、REST 形狀、RFC 9457、opaque session 與 CSRF 防護

## Context

[改寫成 Go](2026-10-18-go-backend.md) 時，第一版計畫保留了舊 FastAPI 的契約：`{data}` 外殼、`/auth/login` 這類動詞路徑、建立與刪除都回 200、offset 分頁、數字 ID，認證則是 JWT 加 refresh token。從零對照業界基準後，舊設計有兩個實際缺口，另有幾處違反標準：

- **cookie 在正式環境收不到**：前端在 `yourdomain.com`、API 在 `api.yourdomain.com`，cookie 沒帶 Domain，所以只屬於 `api.` 這個 host，前端的 server-side 認證檢查收不到它。這是依 cookie 規範推論的，沒有實測。
- **沒有 CSRF 防護**：只靠 `SameSite=Lax`。
- **密碼規則違反 NIST 800-63B-4**：要求字元組合，最短長度也不夠。
- **role 從來沒有被檢查過**。

**約束**：

- `external`：只有一台 VPS，前面是 Caddy／Nginx，跑 Docker Compose。
- `external`：oapi-codegen v2.8.0 對 OpenAPI 3.1 的支援仍標示為「initial」，所以契約維持 3.0.3。
- `inherited`：前端已經依舊契約寫好。現在仍成立的理由：沒有外部理由。前端是我們自己的程式，會隨這次改寫一起改，所以不限制任何選項。

## Options Considered

- **基準. 業界標準組合**：
  - 拓樸：同源（Caddy 把 `/api/*` 轉給後端）。
  - 回應：直接回資源，不包外殼；建立回 201 加 `Location`，刪除回 204（[RFC 9110](https://www.rfc-editor.org/rfc/rfc9110.html)）。
  - 分頁：cursor（[Zalando #160](https://opensource.zalando.com/restful-api-guidelines/)、[AIP-158](https://google.aip.dev/158)）。
  - 部分更新：JSON Merge Patch（RFC 7396）。
  - 錯誤：[RFC 9457](https://www.rfc-editor.org/rfc/rfc9457.html)。
  - Session：`__Host-` cookie（[OWASP Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)）。
  - CSRF：用 Fetch Metadata 檢查，實作為 Go 1.25 起內建的 `http.CrossOriginProtection`（[OWASP CSRF](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)）。
  - 密碼：至少 15 字、不強制組合規則、比對外洩清單（[NIST SP 800-63B-4](https://pages.nist.gov/800-63-4/sp800-63b.html)）。
  - 暴力破解防護：依帳號節流（[OWASP Credential Stuffing](https://cheatsheetseries.owasp.org/cheatsheets/Credential_Stuffing_Prevention_Cheat_Sheet.html)）。
- **A. 保留舊契約，只換語言**（第一版計畫）：前端幾乎不用改。代價是上面四個缺口全部留下來，每個下游專案都會繼承。
- **B. 照基準**（選用）。
- 子選項：
  - 分頁：cursor（選用）或 offset 加總數。
  - 登入識別：username（選用）或驗證過的 email。
  - 外洩密碼清單：內嵌清單（選用）或只查 HIBP。

## Decision

- **D1 拓樸**：同源。Caddy 跑在 compose 內，把 `/api/*` 轉給後端，而且只有 Caddy 發佈 port；compose 的 dev 也走同一個 Caddy。`TRUSTED_PROXIES` 只列 Caddy 的固定 IP，不列整個 subnet（2026-10-18 修訂）。`next dev` 的 `rewrites` 只用於在 host 上直接跑 `pnpm dev`。不需要 CORS，也不再用 `NEXT_PUBLIC_API_URL`。
- **D2 API 形狀**：
  - URL 是資源導向：`/api/users`、`/api/sessions`、`/api/sessions/current`、`/api/users/me`、`/api/items`。
  - 單一資源直接回傳；集合回傳 `{<資源複數>, next_cursor}`。
  - 建立回 201 加 `Location`，刪除回 204。`Location` 指向契約裡有 GET 的正式網址；有 PATCH、PUT 或 DELETE 的資源一定也有 GET（D9）。
  - 分頁用 keyset 的 `limit`/`cursor`，不提供總數。
  - PATCH 收 `application/merge-patch+json`。
  - URL 不帶版本。
  - 契約是 spec-first 的 `openapi.yaml`，是唯一來源：後端用 validator 執行，前端用生成的型別（openapi-typescript 加 openapi-fetch）。
- **D3 錯誤**：RFC 9457 problem。每種錯誤有固定的 `type` URI（`/problems/<slug>`）；驗證錯誤附 `errors: [{pointer, code, detail}]`。前端依 `type` 和 `code` 翻譯，不解析 `detail`。
- **D4 Session**：
  - opaque token，DB 只存 SHA-256。
  - cookie：`__Host-session`，屬性 `HttpOnly; Secure; SameSite=Lax`，不帶 Domain。
  - 有效期：absolute 7 日、idle 1 日。
  - 登入一律發新 token。停用帳號時，撤銷該使用者的所有 session；另提供「登出所有裝置」。之後新增改 role 或改密碼的功能時，同樣要撤銷 session（寫進 API 規則）。
  - 不接受 Bearer。
- **D5 CSRF**：整個 mux 外層包 `http.CrossOriginProtection`。
- **D6 密碼與登入**：
  - 密碼長度 15–128，不強制組合規則；比對內嵌的常見與外洩密碼清單；HIBP 為可選。
  - 雜湊用 argon2id（m=19 MiB、t=2、p=1），並限制同時進行的雜湊數量。
  - 依帳號節流：不論帳號是否存在都記錄失敗；連續失敗第 5 次之後，退避時間依指數成長，上限 1 小時。另有 per-IP 限流，IP 依 `TRUSTED_PROXIES` 解析。
  - 登入名稱是 username：契約接受大小寫，存入前轉小寫，所以唯一性不分大小寫。密碼在雜湊前做 NFKC 正規化（NIST 寫的是 SHOULD）。
  - 長度與格式只寫在契約裡：HTTP 由 validator 執行，`seed-admin` 用同一份內嵌的 spec 驗證；前端不重複實作。
- **D7 授權**：role 只有 `admin` 和 `user`，註冊預設 `user`，admin 只能用 seed 建立。查詢一律帶 owner 條件，非 owner 回 404。admin 可列出、讀取、停用使用者（2026-10-18 由「只有停用」擴大，見 D9）；一般使用者只能讀自己。
- **D9 資源都要有讀取端**（2026-10-18 追加）：新增 `GET /api/users/{id}`（本人或 admin）、`GET /api/users`（admin）；sessions 有公開 id，新增 `GET /api/sessions`、`GET` 與 `DELETE /api/sessions/{id}`（只限自己）、`GET /api/sessions/current`；`POST /api/sessions` 回 `Session`，`Location: /api/sessions/{id}`。測試以兩道機制守住：收到 201 就 GET `Location`、契約測試檢查可寫的 path 都有 GET。
- **D8 shutdown 與 proxy**（2026-10-18 追加）：SIGTERM 後直接 `http.Server.Shutdown`，上限 30s；compose `stop_grace_period` 35s；不做 readiness drain。Caddy 的 `reverse_proxy` 設 `lb_try_duration`，不設 `health_uri`。

## Rationale

- **選 B 而非 A**：A 的唯一收益是前端少改，但前端是我們自己的程式。A 的代價是讓每個下游專案繼承兩個安全缺口與違反標準的設計。這次本來就要重寫，這時候改最便宜。
- **D1**：同源是 `__Host-` cookie 能運作、SSR 拿得到 cookie、CSP 可以只寫 `connect-src 'self'` 的前提。Caddy 放在 compose 內，而不是裝在 host 上，因為 backend 這樣才能完全不對外發佈 port，Caddy 也才能有固定的 IP 給 `TRUSTED_PROXIES`。只信任 Caddy 而不信任整個 subnet，因為 Next 的 server 端呼叫也在 subnet 內：信任它，就只能靠「Next 不轉送 `X-Forwarded-For`」這條規則防止 client 自選限流的 key（MDN「X-Forwarded-For」Security and privacy concerns：只信任已知 proxy 的位址）。dev 也走 Caddy，讓 dev 與正式環境是同一套 proxy（The Twelve-Factor App「Dev/prod parity」）。若 Caddy 裝在 host，backend 看到的來源 IP 是 docker gateway，per-IP 限流會變成全站共用一個 bucket。代價：TLS 憑證存在 compose volume 裡；改用外部 proxy 時，要自己設定 `TRUSTED_PROXIES`。
- **D2**：選 cursor 而非 offset，因為資料在分頁途中有新增時，keyset 結果仍然穩定，而且不用每次都算 `COUNT(*)`。代價：無法直接跳到第 N 頁。需要頁碼的後台表格，可以另外用 offset 實作。
- **D3**：`type` 與 `code` 的可用值列在契約裡（2026-10-18 修訂），前端的翻譯檔與後端送出的值都由測試對照契約，新增一個 code 卻漏了翻譯時 CI 會失敗。清單用 `x-extensible-enum` 而不是 `enum`（2026-10-22 修訂）：清單會增加，前端對不認得的 code 退回 `type` 的訊息；寫成 `enum` 等於宣告封閉清單，oasdiff 會把每次新增當成 breaking change，照 api-design 規則新增 code 一定讓 CI 失敗（Zalando RESTful API Guidelines 第 112 條）。前端由 `scripts/gen-api.mjs` 把清單生成成字串聯集型別，型別檢查照舊。選擇帶 `code` 的陣列格式，而不沿用舊的 `{field: [msg]}` map，因為 RFC 9457 明說用戶端「SHOULD NOT parse detail」，而前端需要穩定的機器碼才能做 next-intl 翻譯。
- **D4–D5**：OWASP 說 SameSite「does not replace a proper CSRF defense」；`api.` 子網域和主網站屬於同一個 site，SameSite 擋不到。`CrossOriginProtection` 是標準庫內建，也是 OWASP 點名的做法。不接受 Bearer，因為目前沒有非瀏覽器的 client（YAGNI）。
- **D6**：NIST 規定「SHALL NOT」強制組合規則，而且「SHALL」比對外洩清單。只用 per-IP 限流擋不住分散式的 credential stuffing，所以加上依帳號節流。2026-10-22 修訂：只依帳號節流會讓任何人把本人鎖在門外，改成依「帳號＋來源」節流，加上只擋陌生來源的帳號層級上限與裝置 cookie（ADR `2026-10-22-account-abuse-and-lifecycle` D4）。內嵌清單可以離線運作，代價是 binary 會變大（約 1 MB）。選 username 而非 email，因為 email 需要寄信服務。代價：註冊時無法隱藏「這個名稱已被使用」，也沒有找回密碼的流程。
- **D7**：範本要示範授權模型，所以保留有測試的 admin 操作，不把 role 整個拿掉。只有「停用」時 admin 拿不到使用者 id，PATCH 形同無法使用，所以補上列出與讀取（D9）。
- **D9**：RFC 9110 §9.3.3 要求 201 的 `Location` 指向建立的資源，§15.3.2 規定沒有 `Location` 時建立的資源就是 target URI，所以不送 `Location` 會把 `/api/users` 誤指成新資源；Google AIP-121 寫「A resource must support at minimum Get」。原設計只開了寫入端：註冊與登入回傳的網址打不開。sessions 的列表與撤銷單一 session 成對提供，同 allauth headless 的 `/auth/sessions` 與 Better Auth 的 `listSessions`／`revokeSession`。不選「`Location` 改成 `/users/me`」：它是依 cookie 而定的別名，`/sessions/current` 與 `/users/{id}` 的問題仍在。代價：多 6 個 operation，sessions 表多一欄。
- **D8**：drain（readiness 先回 503、等幾秒再關）的前提是 load balancer 依 readiness 把流量轉給其他 instance（Kubernetes「Termination of Pods」）。單一 backend 時沒有可轉的 instance，drain 只會讓每次停機多等。Kamal 與 docker-rollout 把零停機做在 proxy 層（先等新容器 healthy 再切流量），不是在 app 裡 sleep。Caddy 的 `lb_try_duration` 在 backend 重建的空檔把請求留住，單一 upstream 也有效；`health_uri` 在單一 upstream 下只會讓 Caddy 提早回錯誤。代價：plain `docker compose up` 重建 backend 時仍有數秒空檔，超過 `lb_try_duration` 的請求會失敗。

## Expected Outcome

- 跨來源的 POST 回 403；同源的請求正常通過。
- 走 Caddy 同源部署時，重新整理頁面後 SSR 仍是登入狀態。
- 在 `openapi.yaml` 改了欄位卻沒有重新生成程式碼時，CI 會失敗。

## Invariants

- `openapi.yaml` 是 API 形狀與限制的唯一來源。
- 認證憑證只放在 `HttpOnly` cookie；所有狀態變更都經過 `CrossOriginProtection`。
- 使用者擁有的資源，查詢一律帶 owner 條件。

## Revocation Triggers

- 需要找回密碼或 email 通知 → 加 email 驗證，重新評估 D6 的登入識別。
- 需要 MFA、SSO 或第三方登入 → 重新評估 D4（考慮代管認證）。
- 出現原生 app 或第三方 API 消費者 → 重新評估 Bearer 與 API 版本策略。
- backend 超過一個 replica、某個下游要求部署不能停機，或改用 Kamal 或 Swarm → per-IP 限流改用 DB 或 proxy 層；零停機部署改用 docker-rollout 模式（Caddy 動態 upstream 加 `health_uri`、backend 不設 `ports`、shutdown 前加回 readiness drain）（D8）。
- oapi-codegen 宣布 3.1 支援穩定 → 升級到 OpenAPI 3.1。

## Review Notes

## Related

- 來源：2026-10-18 一個沒參與規劃的 subagent 從零查證業界基準（來源見上方各連結），repo 擁有者回覆「照建議」拍板。本 ADR 即原始紀錄。
- 同批決定：[後端改寫成 Go](2026-10-18-go-backend.md)。
