# 部署到 VPS

本指南說明如何將 fullstack template 部署到一台 Linux VPS（Ubuntu 22.04+）。

## Prerequisites

VPS 最低需求：
- 2 vCPU / 2GB RAM / 20GB disk
- Ubuntu 22.04 或更新版本
- SSH 存取
- 域名（可選，用於 HTTPS）

## 1. 安裝 k3s

在 VPS 上安裝 k3s（單節點 Kubernetes）：

```bash
curl -sfL https://get.k3s.io | sh -

# 驗證
sudo k3s kubectl get nodes
```

設定 kubectl alias：

```bash
echo 'alias kubectl="sudo k3s kubectl"' >> ~/.bashrc
source ~/.bashrc
```

## 2. 安裝 Docker

k3s 用 containerd，但我們需要 Docker 來 build images：

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
# 重新登入讓 group 生效
```

## 3. 上傳專案

```bash
# 本機
scp -r . user@your-vps:/opt/myproject

# 或用 git
ssh user@your-vps
cd /opt
git clone <your-repo> myproject
cd myproject
```

## 4. 設定環境變數

```bash
cp .env-example .env
nano .env
```

重要設定：
- `SECRET_KEY` — 生成安全的隨機字串：`openssl rand -base64 48`
- `POSTGRES_PASSWORD` — 改成強密碼
- `CORS_ORIGINS` — 改成你的域名：`["https://yourdomain.com"]`
- `NEXT_PUBLIC_API_URL` — API 的公開 URL：`https://api.yourdomain.com`

## 5. Build Images

```bash
docker build -t fullstack-backend:latest ./backend
docker build -t fullstack-frontend:latest ./frontend
```

Import 到 k3s 的 containerd：

```bash
docker save fullstack-backend:latest | sudo k3s ctr images import -
docker save fullstack-frontend:latest | sudo k3s ctr images import -
```

## 6. Deploy

```bash
# 安裝 envsubst（如果沒有）
sudo apt install gettext-base

# Apply manifests
kubectl apply -f k8s/namespace.yaml
set -a && source .env && set +a
envsubst < k8s/postgres/secret.yaml | kubectl apply -f -
envsubst < k8s/backend/secret.yaml | kubectl apply -f -
kubectl apply -f k8s/postgres/pvc.yaml
kubectl apply -f k8s/postgres/deployment.yaml
kubectl apply -f k8s/postgres/service.yaml
kubectl apply -f k8s/backend/deployment.yaml
kubectl apply -f k8s/backend/service.yaml
kubectl apply -f k8s/frontend/configmap.yaml
kubectl apply -f k8s/frontend/deployment.yaml
kubectl apply -f k8s/frontend/service.yaml
```

驗證：

```bash
kubectl get pods -n fullstack
kubectl logs -n fullstack deploy/backend
```

## 7. 設定 Ingress + HTTPS（可選）

k3s 內建 Traefik ingress controller。建立 ingress 資源：

```yaml
# k8s/ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app-ingress
  namespace: fullstack
  annotations:
    traefik.ingress.kubernetes.io/router.tls: "true"
    traefik.ingress.kubernetes.io/router.tls.certresolver: letsencrypt
spec:
  rules:
    - host: yourdomain.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: frontend
                port:
                  number: 3000
    - host: api.yourdomain.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: backend
                port:
                  number: 8000
```

設定 Let's Encrypt（免費 HTTPS）：

```yaml
# /var/lib/rancher/k3s/server/manifests/traefik-config.yaml
apiVersion: helm.cattle.io/v1
kind: HelmChartConfig
metadata:
  name: traefik
  namespace: kube-system
spec:
  valuesContent: |-
    certResolvers:
      letsencrypt:
        email: you@example.com
        storage: /data/acme.json
        httpChallenge:
          entryPoint: web
```

Apply：

```bash
kubectl apply -f k8s/ingress.yaml
```

## 8. 維運指令

```bash
# 查看狀態
kubectl get pods -n fullstack -o wide

# 查看 logs
kubectl logs -n fullstack deploy/backend -f
kubectl logs -n fullstack deploy/frontend -f

# Rolling restart（更新 image 後）
kubectl rollout restart deployment -n fullstack --all

# 進入 backend pod 執行指令
kubectl exec -it -n fullstack deploy/backend -- /bin/sh

# 備份資料庫
kubectl exec -n fullstack deploy/postgres -- pg_dump -U postgres fullstack > backup.sql
```

## 更新部署

```bash
cd /opt/myproject
git pull

# Rebuild
docker build -t fullstack-backend:latest ./backend
docker build -t fullstack-frontend:latest ./frontend
docker save fullstack-backend:latest | sudo k3s ctr images import -
docker save fullstack-frontend:latest | sudo k3s ctr images import -

# Restart
kubectl rollout restart deployment -n fullstack --all
```

## 防火牆設定

```bash
sudo ufw allow 22/tcp    # SSH
sudo ufw allow 80/tcp    # HTTP
sudo ufw allow 443/tcp   # HTTPS
sudo ufw allow 6443/tcp  # k3s API (如果需要遠端管理)
sudo ufw enable
```

## 注意事項

- **生產環境務必**更改 `SECRET_KEY` 和 `POSTGRES_PASSWORD`
- **Backend MODE** 設為 `prod`（關閉 Swagger docs、啟用 JSON logging）
- **資料庫備份**建議設定 cron job 定期備份
- **監控**建議加入 Prometheus + Grafana（k3s 可透過 Helm 安裝）
