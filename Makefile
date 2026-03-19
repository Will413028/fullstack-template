CLUSTER = fullstack
NAMESPACE = fullstack
BACKEND_IMAGE = fullstack-backend
FRONTEND_IMAGE = fullstack-frontend

# --- Cluster ---

.PHONY: cluster-up
cluster-up:
	k3d cluster create $(CLUSTER) \
		--port "30000:30000@server:0" \
		--port "30800:30800@server:0" \
		|| true

.PHONY: cluster-down
cluster-down:
	k3d cluster delete $(CLUSTER)

# --- Build & Import ---

.PHONY: build
build:
	docker build -t $(BACKEND_IMAGE):latest ./backend
	docker build -t $(FRONTEND_IMAGE):latest ./frontend
	k3d image import $(BACKEND_IMAGE):latest $(FRONTEND_IMAGE):latest -c $(CLUSTER)

# --- Deploy ---

.PHONY: secrets
secrets:
	@test -f .env || (echo "Error: .env not found. Copy .env-example to .env and fill in values." && exit 1)
	kubectl apply -f k8s/namespace.yaml
	@set -a && . ./.env && set +a && envsubst < k8s/postgres/secret.yaml | kubectl apply -f -
	@set -a && . ./.env && set +a && envsubst < k8s/backend/secret.yaml | kubectl apply -f -

.PHONY: deploy
deploy: secrets
	kubectl apply -f k8s/postgres/pvc.yaml
	kubectl apply -f k8s/postgres/deployment.yaml
	kubectl apply -f k8s/postgres/service.yaml
	kubectl apply -f k8s/backend/deployment.yaml
	kubectl apply -f k8s/backend/service.yaml
	kubectl apply -f k8s/frontend/configmap.yaml
	kubectl apply -f k8s/frontend/deployment.yaml
	kubectl apply -f k8s/frontend/service.yaml
	@echo "\nDeployed to namespace: $(NAMESPACE)"
	@echo "Frontend: http://localhost:30000"
	@echo "Backend:  http://localhost:30800"

.PHONY: teardown
teardown:
	kubectl delete namespace $(NAMESPACE) --ignore-not-found

# --- Logs ---

.PHONY: logs
logs:
	kubectl logs -n $(NAMESPACE) -l app --all-containers --tail=100 -f

.PHONY: logs-backend
logs-backend:
	kubectl logs -n $(NAMESPACE) -l app=backend --all-containers --tail=100 -f

.PHONY: logs-frontend
logs-frontend:
	kubectl logs -n $(NAMESPACE) -l app=frontend --all-containers --tail=100 -f

.PHONY: logs-postgres
logs-postgres:
	kubectl logs -n $(NAMESPACE) -l app=postgres --tail=100 -f

# --- Status ---

.PHONY: status
status:
	@echo "=== Cluster ==="
	@k3d cluster list 2>/dev/null || echo "k3d not found"
	@echo "\n=== Pods ==="
	@kubectl get pods -n $(NAMESPACE) -o wide 2>/dev/null || echo "Namespace not found"
	@echo "\n=== Services ==="
	@kubectl get svc -n $(NAMESPACE) 2>/dev/null || echo "Namespace not found"

.PHONY: restart
restart:
	kubectl rollout restart deployment -n $(NAMESPACE) --all

# --- Seed ---

.PHONY: seed
seed:
	@echo "Creating admin user..."
	kubectl exec -n $(NAMESPACE) deploy/backend -- /app/.venv/bin/python -c "\
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
	@echo "Renaming project to '$(NAME)'..."
	@sed -i '' 's/fullstack-backend/$(NAME)-backend/g' Makefile
	@sed -i '' 's/fullstack-frontend/$(NAME)-frontend/g' Makefile
	@sed -i '' 's/CLUSTER = fullstack/CLUSTER = $(NAME)/g' Makefile
	@sed -i '' 's/NAMESPACE = fullstack/NAMESPACE = $(NAME)/g' Makefile
	@find k8s -name '*.yaml' -exec sed -i '' 's/fullstack/$(NAME)/g' {} +
	@sed -i '' 's/fullstack-template-frontend/$(NAME)-frontend/g' frontend/package.json
	@sed -i '' 's/fastapi-template/$(NAME)-backend/g' backend/pyproject.toml
	@sed -i '' 's/Fullstack Template/$(NAME)/g' frontend/src/app/layout.tsx frontend/messages/en.json frontend/messages/zh-TW.json CLAUDE.md README.md
	@sed -i '' 's/fullstack/$(NAME)/g' .env-example
	@echo "Done! Review changes with: git diff"
	@echo "Don't forget to update .env if it exists."
