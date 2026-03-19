FROM python:3.12-slim

RUN apt-get update && apt-get install -y --no-install-recommends gcc libpq-dev && rm -rf /var/lib/apt/lists/*

COPY --from=ghcr.io/astral-sh/uv:latest /uv /uvx /bin/

COPY . /app

WORKDIR /app

RUN uv sync --frozen --no-cache

CMD ["sh", "-c", "/app/.venv/bin/alembic upgrade head && /app/.venv/bin/fastapi run src/main.py --port 8000 --host 0.0.0.0"]
