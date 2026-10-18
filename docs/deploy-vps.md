# 部署到 VPS (Docker Compose)

本指南說明如何用 Docker Compose 把專案部署到一台 Linux VPS（以 Ubuntu 22.04+ 為例）。整個 stack 都在 compose 裡，包括負責 HTTPS 的 Caddy：

```
瀏覽器 ──443──▶ caddy ──/api/*──▶ backend:8000 ──▶ postgres
                      └─其餘────▶ frontend:3000
```

前端與 API 同源（`https://yourdomain.com` 與 `https://yourdomain.com/api`），所以 session cookie 不帶 Domain、不需要 CORS，也沒有 `api.` 子網域。只有 Caddy 發佈 port：Docker 發佈的 port 會繞過 ufw 等主機防火牆（[Docker：Packet filtering and firewalls](https://docs.docker.com/engine/network/packet-filtering-firewalls/)），所以 backend、frontend、postgres 都不對外。

## Prerequisites

VPS 最低需求：
- 1 vCPU / 1GB RAM / 20GB disk
- Ubuntu 22.04 或更新版本
- SSH 存取權限
- 域名，A（與 AAAA）記錄指向 VPS：Caddy 用它自動申請 HTTPS 憑證
- 對外開放 80/tcp、443/tcp、443/udp（HTTP/3）

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

`make init-env` 從 `.env-example` 建立 `.env`，並在 `.secrets/` 產生三個資料庫密碼檔（superuser、migrate 用的 owner 角色、backend 用的 app 角色）。compose 把它們以 secret 掛進各自的容器，服務經 `*_FILE` 讀取，密碼不會出現在 `.env`、環境變數或 URL 裡。重跑不會覆寫既有的檔案。

> [!IMPORTANT]
> 生產環境務必在 `.env` 設定：
> - `MODE=prod`：JSON log，不提供 `/api/openapi.yaml` 與 `/api/docs`
> - `COOKIE_SECURE=true`：cookie 名稱為 `__Host-session` 並帶 `Secure`（需要 HTTPS）
> - `SITE_ADDRESS=yourdomain.com`：Caddy 以此網域自動申請、續約憑證；`:80` 只適合本機的純 http

> [!WARNING]
> `.secrets/` 裡的密碼在 postgres volume 第一次初始化時寫進資料庫角色。之後換掉檔案不會改資料庫裡的密碼；不要刪除 `.secrets/`，並把它和資料庫備份一起保存。

---

## 4. 啟動服務與建立管理員

```bash
# 建置並啟動（背景執行）。migrate 先套用 migration 並授權 app 角色，結束後 backend 才啟動
make up

# 建立管理員帳號；密碼從終端機讀取，不會回顯
make seed ADMIN_ACCOUNT=admin
```

驗證運行狀態：
```bash
make status
make logs
curl -s -o /dev/null -w '%{http_code}\n' https://yourdomain.com/api/users/me   # 未登入應為 401
```

---

## 5. HTTPS 與 Caddy

不需要在主機上另裝 Caddy 或 Nginx。compose 內的 Caddy（設定在 `deploy/Caddyfile`）：

- 以 `SITE_ADDRESS` 自動申請、續約 Let's Encrypt 憑證，憑證存在 `caddy-data` volume；
- 送出 `Strict-Transport-Security`（HSTS）；
- 把 `/api/*` 轉給 backend，其餘轉給 Next；
- 以自己產生的值覆寫 `X-Request-ID`，backend 只信任 Caddy 送來的 `X-Forwarded-For` 與 `X-Request-ID`。

backend 怎麼判斷誰是可信的 proxy：`TRUSTED_PROXIES` 預設只列 Caddy 在 compose network 的固定位址（`172.30.0.2/32`，見 `docker-compose.yml`）。per-IP 限流與 access log 的 `client_ip` 都依它解析。

### 改用 compose 外的 proxy

如果前面還有另一層 proxy（例如主機上的 Nginx，或 CDN），或要用它取代 Caddy，自己要處理：

- **取代 Caddy**：外部 proxy 必須維持同源路由（`/api/*` 到 backend、其餘到 frontend）、覆寫 `X-Request-ID`、設定 `X-Forwarded-For`；backend 與 frontend 只綁 `127.0.0.1` 發佈；`TRUSTED_PROXIES` 改成 backend 看到的 proxy 來源位址。這個位址取決於 Docker 的網路設定（經發佈的 port 進來時通常是 compose network 的 gateway），設定前先看 backend access log 裡的 `client_ip`，確認它是真實的用戶端 IP，而不是 proxy 的位址。
- **在 Caddy 前面再加一層**：兩處都要列出外層 proxy 的位址。Caddy 在 global options 的 `servers { trusted_proxies static <CIDR> }` 列出，才會保留外層送來的 `X-Forwarded-For`；`TRUSTED_PROXIES` 也要加上同一個 CIDR（例如 `172.30.0.2/32,<CIDR>`），因為 backend 取 `X-Forwarded-For` 由右往左第一個不可信的位址。少列任一處，外層 proxy 就會被當成用戶端，所有人共用同一個限流 bucket。

---

## 6. 更新版本

```bash
git pull
make build
make up
```

`make up` 會先跑 migrate（套用新的 migration），再重建 backend。backend 收到 SIGTERM 後最多花 30 秒處理完進行中的請求；重建期間 Caddy 最多把請求留住 10 秒（`lb_try_duration`），超過的請求會失敗。單一 backend 不保證零停機。

---

## 7. 資料庫自動備份 (Cron Job)

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

## 8. 維運常用指令

```bash
make up               # 啟動（背景）
make down             # 停止（保留資料）
make down-v           # 停止並刪除 volume（⚠️ 資料庫與 Caddy 憑證都會清空）
make logs             # 查看所有日誌
make logs-backend     # 查看後端日誌
make build            # 重新建置 image
make restart          # 重啟服務
make migration        # 手動套用 migration（make up 已會自動執行）
```

本指南假設資料庫是 compose 內的 PostgreSQL：角色由 `deploy/postgres/initdb/10-roles.sh` 在 volume 第一次初始化時建立。改用 managed PostgreSQL 時這支腳本不會執行，owner 與 app 角色要先以 superuser 照該腳本的內容建立。
