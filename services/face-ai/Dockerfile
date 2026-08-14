FROM ghcr.io/astral-sh/uv:python3.11-bookworm-slim
WORKDIR /app
COPY pyproject.toml uv.lock README.md ./
COPY src ./src
RUN uv sync --locked --no-dev
EXPOSE 8001
CMD ["uv", "run", "uvicorn", "face_ai.main:app", "--host", "0.0.0.0", "--port", "8001"]
