FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    AGENT_MODEL=laguna \
    OPENAI_BASE_URL=http://127.0.0.1:8000/v1 \
    MODEL_REQUEST_TIMEOUT_SECONDS=600

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
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
