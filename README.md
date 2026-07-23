# GitFlame CodePilot

GitFlame CodePilot is an external AI integration service for GitFlame. The project demonstrates two Sprint 1 MVP flows:

- issue-to-plan workflow: GitFlame sends an issue and repository configuration, the backend generates a Markdown implementation plan, and the user can approve, correct, or reject it;
- repository recommendations workflow: the system analyzes repository context, stores a summary and recommendation cards, and shows them in the demo UI.

The project is intended to be started with Docker Compose.

## Project Structure

```text
backend/         Go backend service, API contracts, DB schema, storage models
frontend/        Vue demo UI served by nginx on port 80
ml_service/      Mock ML service for plans and recommendations
docs/            Architecture, diagrams, report materials, verification notes
infra/           Infrastructure notes
recommendations/ Recommendation ML research materials
```

## Quick Start

Prerequisites:

- Docker
- Docker Compose
- Git

Clone the repository with submodules:

```bash
git clone --recurse-submodules https://github.com/GitFlameAI/Capstone-Project-GitFlame-CodePilot.git
cd Capstone-Project-GitFlame-CodePilot
```

If the repository was already cloned, initialize the CodeRAG submodule before
starting the full Sprint 6 stack:

```bash
git submodule update --init --recursive
```

Start all services:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  up -d --build
```

Open the application:

```text
Frontend:              http://localhost/
Backend health:        http://localhost/api/health
Backend direct health: http://localhost:8000/health
ML service health:     http://localhost:8001/health
Agent Engine ready:    http://localhost:8002/ready
RAG service health:    http://localhost:8004/health
```

On the virtual machine, replace `localhost` with the VM IP:

```text
Frontend:       http://<VM_IP>/
Backend health: http://<VM_IP>/api/health
```

The frontend is exposed on port `80`, so the platform implementation link can point directly to:

```text
http://<VM_IP>/
```

## Services

The full Sprint 6 Docker Compose stack starts:

```text
frontend              Vue app + nginx, exposed on port 80
backend               Go API service, exposed on port 8000
ml-service            Python ML service, exposed on port 8001
agent-engine          Agent Engine service, exposed on port 8002
recommendation-service Recommendation service, exposed on port 8003
rag-service           CodeRAG HTTP service, exposed on port 8004
agent-worker          Background worker for queued tasks
database              PostgreSQL, exposed on port 5432
coderag-database      pgvector PostgreSQL for CodeRAG, exposed on port 5433
redis                 Redis broker, exposed on port 6379
```

Sprint 6 deployment details, submodule notes, health checks, and clean
reproducibility commands are documented in:

```text
infra/sprint6-rag-deployment-runbook.md
```

The backend receives:

```text
ML_SERVICE_URL=http://ml-service:8001
DATABASE_URL=postgresql://gitflame:gitflame@database:5432/gitflame_codepilot
```

PostgreSQL is started automatically. The initial schema is applied on first database volume creation from:

```text
backend/db/schema.sql
```

## Useful Commands

Check running containers:

```bash
docker compose ps
```

View logs:

```bash
docker compose logs
```

View logs for one service:

```bash
docker compose logs backend
docker compose logs frontend
docker compose logs ml-service
docker compose logs database
```

Stop the project:

```bash
docker compose down
```

Stop the project and remove the PostgreSQL volume:

```bash
docker compose down -v
```

Use `docker compose down -v` only when you want to recreate the database from scratch.

## Updating Deployment On The VM

From the project directory on the VM:

```bash
git checkout main
git pull origin main
docker compose up -d --build
```

If containers are already running, it is also safe to stop them first:

```bash
docker compose down
git pull origin main
docker compose up -d --build
```

After restart, verify:

```bash
curl http://localhost/
curl http://localhost/api/health
curl http://localhost:8001/health
```

## Local Development Without Docker

Backend:

```bash
cd backend
go run ./cmd/server
```

ML service:

```bash
cd ml_service
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
uvicorn app.main:app --reload --port 8001
```

Frontend:

```bash
cd frontend
npm install
npm run dev
```

For the final VM run, Docker Compose is the recommended path.
