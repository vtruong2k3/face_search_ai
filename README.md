# Face Search AI

Bộ khung monorepo cho sản phẩm Face Search AI SaaS. Giai đoạn này chỉ thiết lập ranh giới service và công cụ phát triển; chưa có auth, Event, upload, xử lý ảnh, model AI hoặc face search.

## Cấu trúc

```text
apps/web                 Next.js + TypeScript
apps/api                 Go HTTP API
services/face-ai         FastAPI internal service placeholder
workers/photo-worker     Python background worker placeholder
packages/contracts       OpenAPI contracts
infra/caddy              Local reverse proxy
docs                     Architecture and development notes
```

## Yêu cầu

- Node.js phù hợp với phiên bản Next.js trong `apps/web/package.json`
- Go 1.26+
- Python 3.11+
- `uv`
- Docker và Docker Compose

## Chạy native

```bash
cd apps/web && npm run dev
cd apps/api && go run ./cmd/api
uv run --project services/face-ai face-ai
uv run --project workers/photo-worker photo-worker
```

Cổng mặc định: web `3000`, API `8080`, face-ai `8001`.

## Chạy bằng Docker Compose

```bash
cp .env.example .env
docker compose up --build
```

Ứng dụng qua Caddy: `http://localhost:8088`.

## Kiểm tra

```bash
make check
make test
make build
docker compose config
```

## Chưa triển khai

- Authentication và authorization
- PostgreSQL migrations/domain schema
- Signed upload/MinIO integration
- Redis job consumption
- Qdrant collections và vector search
- ONNX/InsightFace models
- Event dashboard và customer gallery
