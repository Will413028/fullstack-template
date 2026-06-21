COMPOSE = docker compose
COMPOSE_DEV = $(COMPOSE) -f docker-compose.yml -f docker-compose.dev.yml

# --- Stack ---

.PHONY: up
up:
	$(COMPOSE) up -d
	@echo "\nStack started:"
	@echo "Frontend: http://localhost:3000"
	@echo "Backend:  http://localhost:8000"

.PHONY: dev
dev:
	$(COMPOSE_DEV) up --build
	@echo "\nDev stack started (hot reload enabled):"
	@echo "Frontend: http://localhost:3000"
	@echo "Backend:  http://localhost:8000"

.PHONY: down
down:
	$(COMPOSE) down

.PHONY: down-v
down-v:
	$(COMPOSE) down -v

.PHONY: build
build:
	$(COMPOSE) build

.PHONY: restart
restart:
	$(COMPOSE) restart

# --- Logs ---

.PHONY: logs
logs:
	$(COMPOSE) logs -f

.PHONY: logs-backend
logs-backend:
	$(COMPOSE) logs -f backend

.PHONY: logs-frontend
logs-frontend:
	$(COMPOSE) logs -f frontend

.PHONY: logs-postgres
logs-postgres:
	$(COMPOSE) logs -f postgres

# --- Status ---

.PHONY: status
status:
	$(COMPOSE) ps

# --- Database ---

.PHONY: migration
migration:
	$(COMPOSE) exec backend /app/.venv/bin/alembic upgrade head

.PHONY: generate-migration
generate-migration:
	$(COMPOSE) exec backend /app/.venv/bin/alembic revision --autogenerate -m "$(MSG)"

# --- Seed ---

.PHONY: seed
seed:
	@echo "Creating admin user..."
	$(COMPOSE) exec backend /app/.venv/bin/python -c "\
	import asyncio; \
	from src.core.database import async_session_factory; \
	from src.auth.models import User; \
	from src.core.security import get_password_hash; \
	from src.auth.constants import Role; \
	async def seed(): \
	    async with async_session_factory() as s: \
	        from sqlalchemy import select; \
	        exists = (await s.execute(select(User).where(User.account == 'admin'))).scalar_one_or_none(); \
	        if exists: print('Admin already exists'); return; \
	        s.add(User(account='admin', password=get_password_hash('Admin123'), role=Role.ADMIN.value)); \
	        await s.commit(); print('Admin created (account: admin, password: Admin123)'); \
	asyncio.run(seed())"

# --- Rename ---

.PHONY: rename
rename:
ifndef NAME
	$(error NAME is required. Usage: make rename NAME=my-project)
endif
	@python3 scripts/rename.py $(NAME)

.PHONY: init-env
init-env:
	@python3 scripts/init_env.py

.PHONY: reset-db
reset-db:
	@echo "Stopping stack and clearing database volumes..."
	$(COMPOSE) down -v
	@echo "Starting entire stack and executing migrations..."
	$(COMPOSE) up -d
	@echo "Waiting for backend container to boot..."
	@sleep 3
	@$(MAKE) seed
	@echo "Database has been successfully reset and seeded!"
