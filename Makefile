.PHONY: up down worker register smoke test lint validate tidy

# Start dagnats (engine + embedded NATS + API + console) and the worker.
up:
	docker compose up --build

down:
	docker compose down

# Run the worker locally against a local `dagnats serve`.
worker:
	go run ./cmd/worker

# Validate then register all example workflows.
register:
	./scripts/register-workflows.sh

# End-to-end smoke path (register + start run + print status).
smoke:
	./scripts/smoke-test.sh

test:
	go test ./... -timeout 300s

lint:
	golangci-lint run ./... 2>/dev/null || go vet ./...

validate:
	@for f in workflows/example-client/*.json workflows/_templates/*.json; do \
		echo "== $$f"; \
		dagnats workflow validate "$$f" || exit 1; \
	done

tidy:
	go mod tidy
