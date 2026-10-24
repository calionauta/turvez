.PHONY: up down worker register smoke test lint validate tidy new-ops-repo new-client validate-ops register-ops

# Start dagnats (engine + embedded NATS + API + console) and the worker.
up:
	docker compose up --build

down:
	docker compose down

# Run the worker locally against a local `dagnats serve`.
worker:
	go run ./cmd/worker

# Validate then register example workflows (all clients, or one).
register:
	./scripts/register-workflows.sh $(CLIENT)

# Ops repo (private data): validate/register against WORKFLOWS_DIR.
#   make validate-ops TURVEZ_OPS_DIR=../turvez-ops
validate-ops:
	: "${TURVEZ_OPS_DIR:?set TURVEZ_OPS_DIR to the private ops checkout}"
	WORKFLOWS_DIR=${TURVEZ_OPS_DIR}/workflows go test ./workflows/ -timeout 300s

register-ops:
	: "${TURVEZ_OPS_DIR:?set TURVEZ_OPS_DIR to the private ops checkout}"
	WORKFLOWS_DIR=${TURVEZ_OPS_DIR}/workflows ./scripts/register-workflows.sh $(CLIENT)

new-ops-repo:
	./scripts/new-ops-repo.sh $(DEST)

new-client:
	./scripts/new-client.sh $(CLIENT)

# Safe decrypt-edit-encrypt cycle for one client's secrets.
edit-secrets:
	./scripts/edit-secrets.sh $(CLIENT)

# End-to-end smoke path (register + start run + print status).
smoke:
	./scripts/smoke-test.sh

test:
	go test ./... -timeout 300s

lint:
	golangci-lint run ./... 2>/dev/null || go vet ./...

validate:
	@for f in workflows/*/*.json; do \
		echo "== $$f"; \
		dagnats workflow validate "$$f" || exit 1; \
	done

tidy:
	go mod tidy
