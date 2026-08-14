FROM ghcr.io/astral-sh/uv:python3.11-bookworm-slim
WORKDIR /app
RUN apt-get update \
    && apt-get install --no-install-recommends --yes libgl1 libglib2.0-0 libxcb1 \
    && rm -rf /var/lib/apt/lists/*
COPY pyproject.toml uv.lock README.md ./
COPY src ./src
RUN uv sync --locked --no-dev
EXPOSE 8001
CMD ["uv", "run", "uvicorn", "face_ai.main:app", "--host", "0.0.0.0", "--port", "8001"]
