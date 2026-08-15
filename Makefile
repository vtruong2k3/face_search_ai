.PHONY: setup check build test compose-config migrate-up migrate-down migrate-version migrate-verify api-store-verify up down logs

setup:
	cd apps/web && npm ci
	uv sync --project services/face-ai
	uv sync --project workers/photo-worker

check:
	cd apps/web && npm run lint && npm run typecheck
	cd apps/api && go fmt ./... && go vet ./...

build:
	cd apps/web && npm run build
	cd apps/api && go build ./cmd/api

test:
	cd apps/api && go test ./...
	uv run --project services/face-ai pytest

compose-config:
	docker compose config

migrate-up:
	docker compose up --wait postgres
	docker compose run --rm migrations

migrate-down:
	@printf 'Refusing to migrate the normal development database down. Use make migrate-verify for disposable up/down verification.\n'
	@exit 1

migrate-version:
	docker compose run --rm --entrypoint /migrate migrations -path=/migrations -database="postgres://$${POSTGRES_USER}:$${POSTGRES_PASSWORD}@postgres:5432/$${POSTGRES_DB}?sslmode=disable" version

migrate-verify:
	bash scripts/verify-migrations.sh

api-store-verify:
	bash scripts/verify-api-store.sh

up:
	docker compose up --build

down:
	docker compose down

logs:
	docker compose logs -f
