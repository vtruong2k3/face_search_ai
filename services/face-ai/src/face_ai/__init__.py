from face_ai.main import app


def main() -> None:
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8001)


__all__ = ["main"]
