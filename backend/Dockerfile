FROM golang:1.22-alpine AS builder

WORKDIR /src

# Build identity. Pass them from docker-compose (see docker-compose.yml) so that
# GET /version on a running container reports exactly what was built:
#   docker compose build --build-arg GIT_COMMIT=$(git rev-parse --short HEAD)
ARG VERSION=unknown
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal

RUN LDFLAGS="-X gitflame-codepilot/backend/internal/observability.Version=${VERSION} \
    -X gitflame-codepilot/backend/internal/observability.Commit=${GIT_COMMIT} \
    -X gitflame-codepilot/backend/internal/observability.BuildTime=${BUILD_TIME}" \
    && go build -ldflags "${LDFLAGS}" -o /bin/gitflame-backend ./cmd/server \
    && go build -ldflags "${LDFLAGS}" -o /bin/agent-worker ./cmd/agent-worker

FROM alpine:3.20

WORKDIR /app

COPY --from=builder /bin/gitflame-backend /app/gitflame-backend
COPY --from=builder /bin/agent-worker /app/agent-worker

EXPOSE 8000
CMD ["/app/gitflame-backend"]
