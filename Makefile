.PHONY: dev down test seed migrate api-test web-test worker-test fixture-test lint

dev:
	docker compose up --build -d
	@echo ""
	@echo "Meridian is starting:"
	@echo "  Web:  http://localhost:5173"
	@echo "  API:  http://localhost:8000/docs"
	@echo ""
	@echo "First time? Run 'make seed' after services are up."
	@echo "Stop with: make down"

down:
	docker compose down

migrate:
	@for f in db/migrations/*.sql; do \
		echo "Running $$f..."; \
		docker compose exec -T postgres psql -U meridian -d meridian -f /docker-entrypoint-initdb.d/migrations/$$(basename $$f) || true; \
	done

seed:
	docker compose exec -e DATABASE_URL=postgresql://meridian:meridian@postgres:5432/meridian?sslmode=disable -T api python /scripts/seed.py

test: api-test web-test worker-test

api-test:
	cd apps/api && python3 -m pytest tests/ -v

web-test:
	cd apps/web && npm test -- --run

worker-test:
	cd apps/worker && go test ./...

fixture-test:
	bash scripts/run-fixture-tests.sh

lint:
	cd apps/api && python3 -m ruff check .
	cd apps/web && npm run lint

setup-local:
	cp -n .env.example .env 2>/dev/null || true
	cd apps/api && pip install -r requirements.txt
	cd apps/web && npm install
	cd apps/worker && go mod download