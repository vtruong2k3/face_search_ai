from fastapi import FastAPI, Response, status
from prometheus_client import make_asgi_app

from face_ai.routes.inference import router as inference_router
from face_ai.runtime import runtime_status
from face_ai.settings import get_settings


def create_app() -> FastAPI:
    app = FastAPI(title="Face AI Internal Service", version="0.1.0")
    app.mount("/metrics", make_asgi_app())
    app.include_router(inference_router)

    @app.get("/health/live")
    async def live() -> dict[str, str]:
        return {"status": "ok", "service": "face-ai"}

    @app.get("/health/ready")
    async def ready(response: Response) -> dict[str, object]:
        runtime = runtime_status(get_settings())
        if not runtime["ready"]:
            response.status_code = status.HTTP_503_SERVICE_UNAVAILABLE
        return {"status": "ready" if runtime["ready"] else "not_ready", "service": "face-ai", "runtime": runtime}

    @app.get("/internal/version")
    async def version() -> dict[str, object]:
        return {"service": "face-ai", "version": "0.1.0", "runtime": runtime_status(get_settings())}

    return app


app = create_app()
