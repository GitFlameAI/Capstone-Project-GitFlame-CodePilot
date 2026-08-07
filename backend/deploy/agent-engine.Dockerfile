FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app

COPY recommendations/pyproject.toml ./
COPY recommendations/src ./src

RUN pip install --no-cache-dir \
        "fastapi>=0.115,<1" \
        "httpx>=0.28,<1" \
        "prometheus-client>=0.21,<1" \
        "pydantic>=2.10,<3" \
        "pyyaml>=6.0,<7" \
        "uvicorn>=0.34,<1" \
    && pip install --no-cache-dir --no-deps .

EXPOSE 8001
CMD ["gitflame-agent-engine"]
