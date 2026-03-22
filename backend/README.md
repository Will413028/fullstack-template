# fastapi-template

## Quick Start

### 1. Install Pre-commit
```bash
pre-commit install
```
### 2. Install Dependencies

```bash
uv sync
```

### 3. Run Server

```bash
make run
```

## Code lint

```bash
make lint
```

## Run Tests

```bash
make test
```

## Export Requirements
```bash
make lock
```

## Deploy to remote server
```bash
make deploy
```

## Database Operations

### Auto-generate Migration

```bash
alembic revision --autogenerate -m "migration message"
```

### Database Migration

```bash
alembic upgrade head
```

## Docker

### Build Image

```bash
sudo docker build -t <image_name>:<tag> .
```

### Run Docker Compose

```bash
sudo docker-compose up -d --build
```

### Add Dependency
```bash
uv add <library_name>
```

### Add Dependency to dev
```bash
uv add --dev <library_name>
```





高流量取資料
後端server  -> request 拿 iot 設備的資料 ->  寫入  mq ->  同一個 server 背景 process 收 mq (一分鐘一次)批量寫入db


低流量取資料
後端server  -> request 拿 iot 設備的資料 ->  寫入 db





生報表

存資料讓其他公司取用
分析是另外一個公司
