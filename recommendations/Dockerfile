FROM python:3.12-slim

ARG OLLAMA_VERSION=0.18.2

ENV OLLAMA_VERSION=${OLLAMA_VERSION} \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    OLLAMA_HOST=127.0.0.1:11434 \
    OLLAMA_MODELS=/models \
    RECOMMENDATION_MODEL=qwen2.5-coder:1.5b \
    OLLAMA_BASE_URL=http://127.0.0.1:11434 \
    MODEL_REQUEST_TIMEOUT_SECONDS=600

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl zstd \
    && curl -fsSL https://ollama.com/install.sh | sh \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY pyproject.toml README.md ./
COPY src ./src
COPY recommendation_schema.json ./
COPY space_start.sh ./

RUN sed -i 's/\r$//' space_start.sh && chmod +x space_start.sh
RUN pip install --no-cache-dir .

EXPOSE 7860
CMD ["./space_start.sh"]
