.PHONY: setup check build test up down logs

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

up:
	docker compose up --build

down:
	docker compose down

logs:
	docker compose logs -f
