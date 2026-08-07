# Convenience wrapper. Everything here is a plain docker compose command — the
# Makefile exists so nobody has to remember the build arguments that make
# GET /version meaningful.
#
#   make up          start the stack
#   make ops         show health, queue depth and recent tasks
#   make trace ID=…  follow one request across every service

COMPOSE ?= docker compose
STACK_FILES = -f docker-compose.yml -f docker-compose.observability.yml

export VERSION      ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
export GIT_COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
export BUILD_TIME   ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

.PHONY: help up down build logs ps restart ops trace observability observability-down check-alerts test

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

build: ## Build every image, stamping the git commit into /version
	$(COMPOSE) build

up: ## Build and start the whole stack
	$(COMPOSE) $(STACK_FILES) up -d --build --remove-orphans
	@echo "frontend: http://localhost:$${FRONTEND_PORT:-80}   ops: http://localhost:$${FRONTEND_PORT:-80}/ops"

down: ## Stop the stack (volumes are kept)
	$(COMPOSE) down

ps: ## Show container status
	$(COMPOSE) ps

logs: ## Follow the logs of every service
	$(COMPOSE) logs -f --tail 100

restart: ## Restart one service, e.g. make restart SERVICE=agent-worker
	$(COMPOSE) restart $(SERVICE)

ops: ## Print the deployment's build identity and readiness
	@curl -fsS localhost:$${BACKEND_PORT:-8000}/version; echo
	@curl -fsS localhost:$${BACKEND_PORT:-8000}/ready; echo

trace: ## Follow one request across all services: make trace ID=<request_id>
	@test -n "$(ID)" || (echo "usage: make trace ID=<request_id>"; exit 1)
	$(COMPOSE) logs --since 24h | grep "$(ID)"

observability: ## Start Prometheus, Alertmanager and Grafana (opt-in)
	$(COMPOSE) $(STACK_FILES) --profile observability up -d
	@echo "grafana: http://localhost:$${GRAFANA_PORT:-3000}   prometheus: http://localhost:$${PROMETHEUS_PORT:-9090}"

observability-down: ## Stop the monitoring stack only
	$(COMPOSE) $(STACK_FILES) --profile observability stop prometheus alertmanager grafana

check-alerts: ## Validate the alerting rules and run their unit tests
	docker run --rm --entrypoint promtool -v "$(PWD)/infra/observability:/rules" prom/prometheus:v2.55.1 \
		check rules /rules/alerts.yml
	docker run --rm --entrypoint promtool -v "$(PWD)/infra/observability:/rules" prom/prometheus:v2.55.1 \
		test rules /rules/tests/alerts_test.yml

test: ## Run the backend, Python and frontend test suites
	cd backend && gofmt -l internal cmd && go vet ./... && go test -race ./...
	cd recommendations && PYTHONPATH=src python -m pytest tests -q
	cd frontend && npm ci && npm run build
