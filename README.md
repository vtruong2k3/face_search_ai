# Face Search AI

Bộ khung monorepo cho Face Search AI gồm các service độc lập, AI PoC chạy cục bộ và nền tảng PostgreSQL có migration theo phiên bản. Authentication, Event, upload, xử lý nền, persistence của Go API và các luồng MVP vẫn đang được triển khai.

## Cấu trúc

```text
apps/web                 Next.js + TypeScript
apps/api                 Go HTTP API
services/face-ai         FastAPI internal AI PoC service
workers/photo-worker     Python background worker scaffold
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

## Hạ tầng đã triển khai

- Migration PostgreSQL up/down theo phiên bản cho users, organizations/memberships, events, photos, faces, outbox, sessions, searches, downloads và audit records
- Ràng buộc trạng thái, idempotency và quan hệ tenant/Event ở tầng cơ sở dữ liệu
- Compose tự chạy migration và tạo MinIO bucket theo cách idempotent trước khi khởi động API
- `make migrate-verify` kiểm tra up/down/up trên volume PostgreSQL tách biệt và dùng một lần
- Ranh giới persistence của Go API với pgx pool giới hạn, transaction bảo đảm rollback, lỗi cơ sở dữ liệu được làm sạch và kiểm tra phiên bản migration khi readiness
- Face AI PoC có pipeline InsightFace CPU và benchmark Qdrant riêng; benchmark thật vẫn chờ dataset được ủy quyền

## Chưa triển khai

- Authentication và authorization
- Signed upload/MinIO application integration
- Production Redis job consumption and orchestration
- Production multi-tenant Qdrant collections and vector search
- Event dashboard và customer gallery
