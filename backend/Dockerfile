FROM golang:1.22-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal

RUN go build -o /bin/gitflame-backend ./cmd/server

FROM alpine:3.20

WORKDIR /app

COPY --from=builder /bin/gitflame-backend /app/gitflame-backend

EXPOSE 8000
CMD ["/app/gitflame-backend"]
