# 部署到 VPS (Docker Compose)

本指南說明如何使用 Docker Compose 將專案部署到一台 Linux VPS（以 Ubuntu 22.04+ 為例）。

## Prerequisites

VPS 最低需求：
- 1 vCPU / 1GB RAM / 20GB disk
- Ubuntu 22.04 或更新版本
- SSH 存取權限
- 域名（用於 HTTPS 憑證自動申請）

---

## 1. 安裝 Docker 與 Docker Compose

在 VPS 上執行官方安裝指令：

```bash
# 安裝 Docker
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER

# 驗證安裝
docker compose version
```

> [!NOTE]
> 重新登入 SSH 可以讓 `docker` 群組權限生效，不需再加 `sudo`。

---

## 2. 上傳專案程式碼

您可以透過 Git clone 或 `scp` 上傳專案到 VPS：

```bash
# 在 VPS 上
cd /opt
git clone <your-repo-url> myproject
cd myproject
```

---

## 3. 設定環境變數

```bash
make init-env
nano .env
```

> [!TIP]
> `make init-env` 指令會自動生成高強度的隨機 `SECRET_KEY` 以及 `POSTGRES_PASSWORD` 並填充進 `.env` 檔案。

> [!IMPORTANT]
> 生產環境（HTTPS）務必在 `.env` 設定：
> - `MODE=prod`（啟用 placeholder secret 防呆）
> - `COOKIE_SECURE=true`（auth cookie 才會在 HTTPS 下送出）
> - `CORS_ORIGINS=["https://yourdomain.com"]`
> - `NEXT_PUBLIC_API_URL=https://api.yourdomain.com` — 此值在 **build 時**編進前端 bundle，
>   修改後必須 `make build` 重新建置才會生效（僅 `make up` 無效）。

---

## 4. 啟動服務與資料庫初始化

使用 `Makefile` 指令一鍵啟動與建置 Docker 鏡像：

```bash
# 1. 以生產模式啟動所有服務（背景執行）
make up

# 2. 執行資料庫遷移 (Alembic)
make migration

# 3. 建立預設的管理員帳號 (admin / Admin123)
make seed
```

驗證運行狀態：
```bash
make status
make logs
```

---

## 5. 設定 Reverse Proxy 與 HTTPS

建議在 VPS 上使用 **Caddy** 或 **Nginx** 作為反向代理。

### 方案 A：使用 Caddy (推薦，自動管理 SSL 憑證)

Caddy 的設定極度簡單，並且會自動申請、續約 Let's Encrypt 免費 SSL 憑證。

1. **安裝 Caddy：**
   ```bash
   sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https
   curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
   curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
   sudo apt update
   sudo apt install caddy
   ```

2. **配置 `/etc/caddy/Caddyfile`：**
   ```caddy
   yourdomain.com {
       reverse_proxy localhost:3000
   }

   api.yourdomain.com {
       reverse_proxy localhost:8000
   }
   ```

3. **重啟 Caddy：**
   ```bash
   sudo systemctl restart caddy
   ```

---

### 方案 B：使用 Nginx + Certbot (傳統方案)

1. **安裝 Nginx 與 Certbot：**
   ```bash
   sudo apt update
   sudo apt install -y nginx certbot python3-certbot-nginx
   ```

2. **配置 Nginx 虛擬主機 (`/etc/nginx/sites-available/myproject`)：**
   ```nginx
   server {
       server_name yourdomain.com;
       location / {
           proxy_pass http://localhost:3000;
           proxy_set_header Host $host;
           proxy_set_header X-Real-IP $remote_addr;
           proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
           proxy_set_header X-Forwarded-Proto $scheme;
       }
   }

   server {
       server_name api.yourdomain.com;
       location / {
           proxy_pass http://localhost:8000;
           proxy_set_header Host $host;
           proxy_set_header X-Real-IP $remote_addr;
           proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
           proxy_set_header X-Forwarded-Proto $scheme;
       }
   }
   ```

3. **啟用設定並申請 SSL 憑證：**
   ```bash
   sudo ln -s /etc/nginx/sites-available/myproject /etc/nginx/sites-enabled/
   sudo nginx -t
   sudo systemctl restart nginx

   # 透過 Certbot 申請憑證並自動設定 HTTPS
   sudo certbot --nginx -d yourdomain.com -d api.yourdomain.com
   ```

---

## 6. 資料庫自動備份 (Cron Job)

請務必設定資料庫定時備份。在 VPS 上新增一個備份腳本：

```bash
# 新增備份目錄
mkdir -p /opt/backups
```

建立備份腳本 `/opt/myproject/backup.sh`：

```bash
#!/bin/bash
BACKUP_DIR="/opt/backups"
DATE=$(date +\%Y\%m\%d_\%H\%M\%S)
FILENAME="${BACKUP_DIR}/db_backup_${DATE}.sql"

# 從 Postgres 容器中導出資料
docker compose exec -t postgres pg_dump -U postgres fullstack > $FILENAME

# 僅保留最近 7 天的備份
find $BACKUP_DIR -type f -name "*.sql" -mtime +7 -delete

echo "Database backed up to ${FILENAME}"
```

設定腳本執行權限：
```bash
chmod +x /opt/myproject/backup.sh
```

新增至 crontab，設定為每天凌晨 3 點自動備份：
```bash
# 開啟 crontab 編輯器
crontab -e

# 加入以下設定
0 3 * * * /opt/myproject/backup.sh >> /var/log/db_backup.log 2>&1
```

---

## 7. 維運常用指令

```bash
make up               # 啟動（背景）
make down             # 停止（保留資料）
make down-v           # 停止並清空資料（⚠️ 會清空資料庫卷，請小心使用）
make logs             # 查看所有日誌
make logs-backend     # 查看後端日誌
make build            # 重新構建鏡像
make restart          # 重啟服務
```
