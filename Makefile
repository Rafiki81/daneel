.PHONY: test lint coverage build examples docker-build clean

# ── Go ───────────────────────────────────────────────────────────────────────

test:
	go test -race -timeout 120s ./...

lint:
	gofmt -s -l . | grep . && exit 1 || true
	go vet ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

build:
	go build -o bin/daneel ./cmd/daneel

# ── Examples ─────────────────────────────────────────────────────────────────

examples: \
	examples/quickstart \
	examples/multi-platform \
	examples/slack-assistant \
	examples/github-reviewer \
	examples/twitter-bot \
	examples/permissions

examples/%:
	@echo "Building $@..."
	go build ./$@/...

# ── Docker ───────────────────────────────────────────────────────────────────

docker-build:
	docker build -t daneel:latest .

docker-compose-up:
	docker compose up -d

docker-compose-down:
	docker compose down

# ── Housekeeping ──────────────────────────────────────────────────────────────

clean:
	rm -f bin/daneel coverage.out coverage.html
	rm -rf bin/
