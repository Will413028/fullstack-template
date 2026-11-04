# 部署：API 在 VPS（Docker Compose），前端在 Vercel

本指南說明如何把後端用 Docker Compose 部署到一台 Linux VPS（以 Ubuntu 22.04+ 為例），前端部署到 Vercel。設計理由見 [ADR：前端改由 Vercel 部署](adr/2026-11-03-frontend-on-vercel.md)。

```
瀏覽器 ──▶ Vercel（yourdomain.com）──/api/*，帶 X-Origin-Secret──▶ caddy（api.yourdomain.com）──▶ backend:8000 ──▶ postgres
              └─頁面、SSR（SSR 也呼叫 yourdomain.com/api）
```

瀏覽器只看到 `https://yourdomain.com`：Vercel 把 `/api/*` 轉給 VPS 上的 Caddy（`frontend/vercel.json`），所以 session cookie 不帶 Domain、不需要 CORS。Caddy 只放行帶對 `X-Origin-Secret` 的請求，並把 Vercel 填的訪客 IP（`X-Real-IP`）交給 backend。只有 Caddy 發佈 port：Docker 發佈的 port 會繞過 ufw 等主機防火牆（[Docker：Packet filtering and firewalls](https://docs.docker.com/engine/network/packet-filtering-firewalls/)），所以 backend、postgres 都不對外。

## Prerequisites

VPS 最低需求：
- 1 vCPU / 1GB RAM / 20GB disk
- Ubuntu 22.04 或更新版本
- SSH 存取權限
- API 用的子網域（例如 `api.yourdomain.com`），A（與 AAAA）記錄指向 VPS：Caddy 用它自動申請 HTTPS 憑證。網站本身的網域指向 Vercel（見第 6 節）
- 對外開放 80/tcp（申請憑證與轉址）、443/tcp

---

## 1. 安裝 Docker 與 Docker Compose

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER

docker compose version
```

> [!NOTE]
> 重新登入 SSH 讓 `docker` 群組權限生效，之後不需再加 `sudo`。

---

## 2. 上傳專案程式碼

```bash
cd /opt
git clone <your-repo-url> myproject
cd myproject
```

主機上需要 `make` 與 Python 3（`make init-env` 用）；Go 與 pnpm 不需要，image 在 Docker 內建置。

---

## 3. 設定環境變數與密碼

```bash
make init-env
nano .env
```

`make init-env` 從 `.env-example` 建立 `.env`，並在 `.secrets/` 產生三個資料庫密碼檔（superuser、migrate 用的 owner 角色、backend 用的 app 角色）、`origin_secret`（Vercel 送給 Caddy 的 `X-Origin-Secret`），以及輪替時才會用到的空檔 `origin_secret_previous`。compose 把它們以 secret 掛進各自的容器，密碼不會出現在 `.env`、環境變數或 URL 裡。重跑不會覆寫既有的檔案。

> [!IMPORTANT]
> 生產環境務必在 `.env` 設定：
> - `MODE=prod`：JSON log
> - `COOKIE_SECURE=true`：cookie 名稱為 `__Host-session` 並帶 `Secure`（需要 HTTPS）
> - `SITE_ADDRESS=api.yourdomain.com`：Caddy 以 API 的網域自動申請、續約憑證；`:80` 只適合本機的純 http

> [!WARNING]
> `.secrets/` 裡的密碼在 postgres volume 第一次初始化時寫進資料庫角色。之後換掉檔案不會改資料庫裡的密碼；不要刪除 `.secrets/`，並把它和資料庫備份一起保存。

---

## 4. 啟動服務與建立管理員

```bash
# 建置並啟動（背景執行）。migrate 先套用 migration 並授權 app 角色，結束後 backend 才啟動
make up

# 建立管理員帳號；密碼從終端機讀取，不會回顯
make seed ADMIN_ACCOUNT=admin

# 管理員全被停用、被節流鎖住或忘記密碼時：替既有的管理員設新密碼、啟用並登出所有裝置
make recover-admin ADMIN_ACCOUNT=admin
```

驗證運行狀態：
```bash
make status
make logs
# Connect 的 RPC：POST /api/<package>.<Service>/<Method>，body 是 JSON 的請求訊息
curl -s -o /dev/null -w '%{http_code}\n' -X POST -H 'Content-Type: application/json' -d '{}' \
  https://api.yourdomain.com/api/app.users.v1.UserAppService/GetCurrentUser   # 沒帶密鑰應為 403
curl -s -o /dev/null -w '%{http_code}\n' -X POST -H 'Content-Type: application/json' -d '{}' \
  https://yourdomain.com/api/app.users.v1.UserAppService/GetCurrentUser       # 經 Vercel、未登入應為 401（unauthenticated；第 6 節設定完成後）
```

---

## 5. HTTPS 與 Caddy

不需要在主機上另裝 Caddy 或 Nginx。compose 內的 Caddy（設定在 `deploy/Caddyfile`）：

- 以 `SITE_ADDRESS` 自動申請、續約 Let's Encrypt 憑證，憑證存在 `caddy-data` volume；
- 送出 `Strict-Transport-Security`（HSTS；網站網域的 HSTS 由 Vercel 送）；
- 只服務 `/api/*`，其餘路徑回 404；`X-Origin-Secret` 不等於 `.secrets/origin_secret` 的請求回 403；
- 放行的請求：以 Vercel 填的 `X-Real-IP` 覆寫 `X-Forwarded-For`、以 `X-Forwarded-Host` 覆寫 `Host`（讓 CSRF 檢查比對的是網站本身的 host）、以自己產生的值覆寫 `X-Request-ID`，並拿掉密鑰 header。

backend 怎麼判斷誰是可信的 proxy：`TRUSTED_PROXIES` 預設只列 Caddy 在 compose network 的固定位址（`172.30.0.2/32`，見 `docker-compose.yml`）。per-IP 限流與 access log 的 `client_ip` 都依它解析，所以訪客 IP 由 Caddy 決定，而 Caddy 只在密鑰相符時採信 Vercel 的 `X-Real-IP`。

### 輪替 origin secret

外洩、或知道密鑰的人離開時輪替。Caddy 同時接受 `.secrets/origin_secret` 與 `.secrets/origin_secret_previous`（平常是空檔），所以輪替不會中斷服務（Vercel 文件的做法）。兩個檔都是 `0444`：改內容前先 `chmod 644`，改完 `chmod 444`。

1. 在 VPS 上把 `origin_secret` 的內容搬進 `origin_secret_previous`，在 `origin_secret` 寫入新值（`python3 -c 'import secrets; print(secrets.token_urlsafe(32))'`），然後 `docker compose restart caddy`：新舊兩個值都能通過；
2. 把新值寫進 Vercel 專案 Production 環境的 `ORIGIN_SECRET`，重新部署；
3. 新部署上線後，清空 `origin_secret_previous`（`: > .secrets/origin_secret_previous`），再 `docker compose restart caddy`：舊值從此失效。

---

## 6. 前端：Vercel

1. 在 Vercel 建立專案，Root Directory 設為 `frontend`（框架會自動偵測為 Next.js）。Node.js 的大版本由 `frontend/package.json` 的 `engines.node` 決定，它會蓋過專案設定；與 CI 的 `node-version` 一起改。
2. Function 區域（Settings → Functions）選離 VPS 最近的區域：每次 SSR 都會經 Vercel 呼叫 API。
3. 環境變數，**只設在 Production**：
   - `NEXT_PUBLIC_APP_URL=https://yourdomain.com`：網站的網址，SSR 呼叫它的 `/api`。沒設時 SSR 的請求送往不存在的 `.invalid` host，不會帶著 cookie 送到別處；
   - `API_ORIGIN=https://api.yourdomain.com`：`frontend/vercel.json` 把 `/api/*` 轉到這裡；
   - `ORIGIN_SECRET=<VPS 上 .secrets/origin_secret 的內容>`。
4. 網域：把 `yourdomain.com` 加到 Vercel 專案，DNS 照 Vercel 的指示設定。
5. preview 部署（每個分支，包括 Dependabot 的）不帶 `ORIGIN_SECRET`，所以連不到 API，也無法登入：任何分支的程式都拿不到正式環境的密鑰。開著 Vercel 的 Deployment Protection，讓 preview 只給登入 Vercel 的人看。

驗證：在網站登入後重新整理頁面，仍是登入狀態（SSR 讀得到 cookie）；backend access log 的 `client_ip` 是自己的對外 IP。

---

## 7. 更新版本

```bash
git pull
make build
make up
```

`make up` 會先跑 migrate（套用新的 migration），再重建 backend。backend 收到 SIGTERM 後最多花 30 秒處理完進行中的請求；重建期間 Caddy 最多把 API 請求留住 10 秒（`lb_try_duration`），超過的請求會失敗。單一 backend 不保證零停機。前端由 Vercel 在每次部署時建置。

---

## 8. 資料庫自動備份 (Cron Job)

請務必設定資料庫定時備份。備份以容器內的 superuser 執行（`POSTGRES_USER`，只用於初始化與維運）：

```bash
mkdir -p /opt/backups
```

建立備份腳本 `/opt/myproject/backup.sh`：

```bash
#!/bin/bash
set -euo pipefail
cd /opt/myproject
BACKUP_DIR="/opt/backups"
DATE=$(date +%Y%m%d_%H%M%S)
FILENAME="${BACKUP_DIR}/db_backup_${DATE}.dump"

# -T：不配置 TTY，輸出才不會被改寫換行；custom format 可用 pg_restore 選擇性還原
docker compose exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -Fc "$POSTGRES_DB"' > "$FILENAME"

# 僅保留最近 7 天的備份
find "$BACKUP_DIR" -type f -name "*.dump" -mtime +7 -delete

echo "Database backed up to ${FILENAME}"
```

設定執行權限並加入 crontab（每天凌晨 3 點）：
```bash
chmod +x /opt/myproject/backup.sh
crontab -e
# 加入：
0 3 * * * /opt/myproject/backup.sh >> /var/log/db_backup.log 2>&1
```

還原到同一個 stack（會覆蓋現有資料）：
```bash
docker compose exec -T postgres sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists' < /opt/backups/db_backup_<DATE>.dump
```

---

## 9. 維運常用指令

```bash
make up               # 啟動（背景）
make down             # 停止（保留資料）
make down-v           # 停止並刪除 volume（⚠️ 資料庫與 Caddy 憑證都會清空）
make logs             # 查看所有日誌
make logs-backend     # 查看後端日誌
make logs-caddy       # 查看 Caddy 日誌（被擋下的 403 在這裡）
make build            # 重新建置 image
make restart          # 重啟服務
make migration        # 手動套用 migration（make up 已會自動執行）
```

本指南假設資料庫是 compose 內的 PostgreSQL：角色由 `deploy/postgres/initdb/10-roles.sh` 在 volume 第一次初始化時建立。改用 managed PostgreSQL 時這支腳本不會執行，owner 與 app 角色要先以 superuser 照該腳本的內容建立。連線也要加密：`DATABASE_URL` 與 `MIGRATE_DATABASE_URL` 改用 `sslmode=verify-full`，並拿掉 compose 裡的 `DATABASE_PLAINTEXT=same-host`；沒加密時 backend 與 migrate 拒絕啟動。
