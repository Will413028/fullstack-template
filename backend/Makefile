.PHONY: run
run:
	uv run fastapi dev ./src/main.py

.PHONY: lint
lint:
	ruff format
	ruff check --fix

.PHONY: test
test:
	uv run pytest

.PHONY: generate_migration
generate_migration:
	@read -p "Enter migration name: " migration_name; \
	uv run alembic revision --autogenerate -m "$${migration_name}"

.PHONY: migration
migration:
	uv run alembic upgrade head

