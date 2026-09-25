SHELL := /bin/sh

GO ?= go
GO_CACHE_ROOT ?= /tmp/cognigo-go
GOCACHE ?= $(GO_CACHE_ROOT)/build
GOMODCACHE ?= $(GO_CACHE_ROOT)/mod
GOPATH ?= $(GO_CACHE_ROOT)/path
export GOCACHE GOMODCACHE GOPATH

BACKEND_BIN ?= bin/cognigo
MCP_BIN ?= common/mcp/bin/cognigo-mcp
PORT ?= 9090
MCP_ADDR ?= :8081

.PHONY: setup doctor test vet build build-mcp models run run-mcp frontend-install frontend-dev frontend-build infra-up infra-down clean

setup:
	@mkdir -p "$(GOCACHE)" "$(GOMODCACHE)" "$(GOPATH)" bin common/mcp/bin
	$(GO) mod download

doctor:
	@sh scripts/doctor.sh

test: setup
	$(GO) test ./...

vet: setup
	$(GO) vet ./...

build: setup
	$(GO) build -o "$(BACKEND_BIN)" .

models:
	sh scripts/download-models.sh

build-mcp:
	@mkdir -p "$(dir $(MCP_BIN))"
	cd common/mcp && GOCACHE="$(GOCACHE)" GOMODCACHE="$(GOMODCACHE)" GOPATH="$(GOPATH)" $(GO) mod download
	cd common/mcp && GOCACHE="$(GOCACHE)" GOMODCACHE="$(GOMODCACHE)" GOPATH="$(GOPATH)" $(GO) build -o "../../$(MCP_BIN)" .

run: setup
	set -a; [ ! -f .env ] || . ./.env; set +a; COGNIGO_PORT="$${COGNIGO_PORT:-$(PORT)}" $(GO) run .

run-mcp: build-mcp
	"$(MCP_BIN)" -mode server -http-addr "$(MCP_ADDR)"

frontend-install:
	npm ci --prefix vue-frontend

frontend-dev:
	set -a; [ ! -f .env ] || . ./.env; set +a; npm run serve --prefix vue-frontend

frontend-build: frontend-install
	npm run build --prefix vue-frontend

infra-up:
	docker compose up -d mysql redis rabbitmq

infra-down:
	docker compose down

clean:
	@for directory in bin common/mcp/bin vue-frontend/dist; do \
		[ ! -d "$$directory" ] || find "$$directory" -mindepth 1 -delete; \
	done
