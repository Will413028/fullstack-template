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
