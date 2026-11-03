---
title: 前端改由 Vercel 部署，Caddy 只當 API 的入口，以共用密鑰認 Vercel
date: 2026-11-03
review-date: 2027-05-03
status: active
tags: [template-go-nextjs, decision, deploy, vercel, caddy, security]
---

# 前端改由 Vercel 部署，Caddy 只當 API 的入口，以共用密鑰認 Vercel

## Context

[API 與認證基準](2026-10-18-api-and-auth-baseline.md) D1 讓 compose 內的 Caddy 同時服務頁面與 `/api/*`，瀏覽器只看到一個來源。repo 擁有者決定前端改用 Vercel 部署，不再保留自架前端。後端仍在 VPS 的 compose 裡。

同源仍然必要：`__Host-` session cookie、SSR 讀得到 cookie、CSP `connect-src 'self'`、不需要 CORS，都以它為前提（D1 Rationale）。

**約束**：

- `external`：Vercel 沒有固定的出口 IP（Static IPs 是 Pro 方案的加購，只涵蓋 Functions 與 build），所以後端無法用 IP 認出 Vercel。
- `external`：Vercel 會覆寫 `X-Forwarded-For` 與 `X-Real-IP`，不轉送 client 帶的值（[Request headers](https://vercel.com/docs/headers/request-headers)；hawkr 2026-11-01 在預覽部署實測：帶偽造的 `X-Real-IP: 7.7.7.7` 時讀到的仍是真實出口 IP）。
- `external`：Vercel 的外部 rewrite 會原樣轉回多個 `Set-Cookie` 與 3xx（hawkr 2026-11-01 實測，`next.config.ts` rewrites）。
- `external`：Vercel 要經公開網路連到後端，後端需要對外的 HTTPS 入口。
- `inherited`：後端只信任 Caddy 的固定 IP（`TRUSTED_PROXIES`）。這是我們自己的設計，不限制選項；本 ADR 保留它，理由見 D2。

## Options Considered

- **基準**：Vercel 文件的做法。外部 rewrite 把 `/api/*` 轉給後端（瀏覽器仍同源），由 Vercel 在轉送的請求上設定一個密鑰 header，origin 拒絕沒帶的請求（[Rewrites：Restricting your origin to Vercel traffic](https://vercel.com/docs/routing/rewrites)）。
- **D1 TLS 入口**：A 保留 Caddy，只轉 `/api/*`（選用）；B Cloudflare Tunnel；C 後端自己做 TLS（certmagic）。
- **D2 誰檢查密鑰、誰決定 client IP**：A Caddy 檢查密鑰，密鑰相符時把 Vercel 填的 `X-Real-IP` 放進 `X-Forwarded-For`，後端不變（選用）；B 後端檢查密鑰並在相符時讀 `X-Real-IP`。
- **D3 SSR 怎麼呼叫 API**：A 經自己網站的 `/api`，也就是經過 Vercel edge（選用）；B 直接打 API 網域並帶密鑰。
- **D4 API 子網域加 CORS**：不同源，`__Host-` cookie 無法跨 host，需重寫認證。不選。

## Decision

- **D1**：保留 Caddy，只服務 API 網域的 `/api/*`，其餘路徑回 404。`lb_try_duration`、覆寫 `X-Request-ID`、HSTS 照舊。頁面的 HSTS 由 Vercel 負責。
- **D2**：Caddy 檢查 `X-Origin-Secret` 是否等於 `/run/secrets/origin_secret`（`.secrets/origin_secret`，由 `make init-env` 產生），並要求它至少 43 個 URL-safe 字元。不符就回 403。相符時以 `X-Real-IP` 覆寫 `X-Forwarded-For`、把 `Host` 換成 `X-Forwarded-Host`，再拿掉密鑰 header。Caddy 的 access log 會記錄請求 header，所以要用 filter 刪掉 `X-Origin-Secret`（2026-11-03 實測：不加 filter 時密鑰會出現在日誌）。後端維持只信任 Caddy、照舊走 `X-Forwarded-For`。
- **D3**：SSR 與瀏覽器一樣呼叫 `NEXT_PUBLIC_APP_URL` 的 `/api`。Next 的程式碼不持有密鑰。
- **Vercel 端**：`frontend/vercel.json` 的 `routes` 把 `/api/(.*)` 轉到 `${API_ORIGIN}/api/$1`，並用 request header transform 設定 `X-Origin-Secret: $ORIGIN_SECRET`。`ORIGIN_SECRET` 只放在 Vercel 的 Production 環境，所以 preview 部署連不到 API。
- **dev**：不跑 Caddy。backend 只綁 `127.0.0.1:8000`，在 host 上跑 `pnpm dev`，由 `next.config.ts` 的 dev rewrite 轉送 `/api`。

## Rationale

- **D1**：Caddy 原本的職責裡，只有「轉送頁面」隨前端搬走，TLS 憑證、重啟時暫留請求、固定 IP 都仍需要。B 多信任一層 Cloudflare 的 header，C 在應用程式裡加 TLS 依賴，且重啟空檔沒有東西暫留請求。
- **D2**：密鑰證明這一跳是 Vercel，而 Vercel 會覆寫 `X-Real-IP`，所以只有在密鑰相符時才能採信它。這件事屬於部署層。放在 Caddy，後端不需要新設定或分支，信任規則仍只有一條（只信任 Caddy）。兩種做法都只有 compose CI 能驗證，所以「Go 測試比較容易蓋到」不是選 B 的理由。Caddy 的字串比對不是固定時間比對，但對 43 字元的隨機值，遠端計時攻擊不實際（推論）。長度檢查讓密鑰檔是空的時候也會擋下所有請求，不會放行（2026-11-03 用 caddy 2.11.7 實測：空檔與空 header 會相符，加上 `header_regexp` 後不再相符）。`Host` 換成網站的 host，是因為 `http.CrossOriginProtection` 在沒有 `Sec-Fetch-Site` 的舊瀏覽器上，會改比對 `Origin` 與 `Host`。
- **D3**：B 讓 Next 的程式碼持有能決定 client IP 的密鑰，等於推翻 D1 Rationale 的「不信任 Next」：防止 client 自選限流 key，又只剩下「Next 不轉送 `X-Real-IP`」這條慣例。走 edge 時，`X-Real-IP` 一律由 Vercel 填寫（SSR 呼叫時是 Function 的出口 IP，`GET /users/me` 不受限流影響）。代價是每次 SSR 多經過一次 Vercel edge。
- **preview**：Vercel 會替每個分支建 preview，包括 Dependabot 的分支。密鑰若放在 Preview 環境，任何分支的程式都能拿到它。
- **dev**：next dev 的 rewrite 不能加 header，所以 dev 無法走密鑰路徑。dev 與正式環境的差距由 compose CI job 補上：它驗證密鑰、`X-Real-IP` 與 `Host` 的處理。

## Expected Outcome

- 沒帶或帶錯 `X-Origin-Secret` 的請求在 Caddy 回 403；帶對的請求，後端的 `client_ip` 是 Vercel 填的 `X-Real-IP`。
- 在 Vercel 正式環境登入後重新整理，SSR 仍是登入狀態。
- compose 只有 Caddy 發佈 port。

## Invariants

- 只有 Caddy 能決定後端看到的 client IP；Next 的程式碼不持有 `ORIGIN_SECRET`。
- `ORIGIN_SECRET` 只放在 Vercel 的 Production 環境與 VPS 的 `.secrets/origin_secret`。
- API 回應帶 `Cache-Control: no-store`（`securityHeaders`），所以 Vercel 不會快取外部 rewrite 的回應。

## Revocation Triggers

- Vercel 提供能驗證來源的機制（mTLS 或簽章）→ 取代共用密鑰。
- 需要 preview 部署能登入 → 加一個 staging API 與它自己的密鑰，不共用正式環境的密鑰。
- 密鑰外洩，或有知道密鑰的人離開 → 照 `docs/deploy-vps.md` 輪替；VPS 與 Vercel 對不上時，請求會短暫回 403。
- 前端要回到自架 → 重新評估 D1（Caddy 再服務頁面）與 D3。

## Review Notes

## Related

- 修訂：[API 與認證基準](2026-10-18-api-and-auth-baseline.md) D1（Caddy 只服務 API；dev 不走 Caddy）、D8（`lb_try_duration` 只涵蓋 API）。
- 來源：2026-11-03 repo 擁有者決定「保留 Caddy 給 API，只用 Vercel」；設計經一個沒參與實作的 subagent 審查。
