.PHONY: \
	help \
	install backend-install assets-install plugin-install \
	dev backend-dev backend-repl assets-dev plugin-dev \
	build backend-build assets-build plugin-build plugin-package \
	test backend-test plugin-test \
	db-migrate db-pending db-create-migration db-reset \
	clean

APP_PORT ?= 8080
CONSOLE_PORT ?= 9090
DATA_PATH ?= server/data
BACKEND_DATA_PATH := $(abspath $(DATA_PATH))

help:
	@echo "Mdbrain - Developer commands"
	@echo ""
	@echo "Development:"
	@echo "  make dev                           Start backend + app/console watch (APP_PORT/CONSOLE_PORT)"
	@echo "  make backend-repl                  Print Go backend debugging hint (no REPL)"
	@echo "  make assets-dev                    Watch and rebuild Tailwind CSS (console + app)"
	@echo "  make plugin-dev                    Watch Obsidian plugin (vaults/test)"
	@echo ""
	@echo "Build:"
	@echo "  make build                         Build backend + CSS + plugin"
	@echo "  make backend-build                 Build backend binaries"
	@echo "  make assets-build                  Build Tailwind CSS (console + app)"
	@echo "  make plugin-build                  Build Obsidian plugin to dist/"
	@echo "  make plugin-package                Package plugin zip (mdbrain-plugin.zip)"
	@echo ""
	@echo "Test:"
	@echo "  make test                          Run backend + plugin tests"
	@echo "  make backend-test                  Run backend tests (go test ./...)"
	@echo "  make plugin-test                   Run plugin tests (pnpm test)"
	@echo ""
	@echo "Database:"
	@echo "  make db-migrate                    Run migrations"
	@echo "  make db-pending                    List pending migrations"
	@echo "  make db-create-migration NAME=xxx  Generate a new versioned migration from ent schema"
	@echo "  make db-reset                      Delete local DB and rerun migrations"
	@echo ""
	@echo "Maintenance:"
	@echo "  make install                       Install backend + assets + plugin dependencies"
	@echo "  make clean                         Remove build outputs"
	@echo ""
	@echo "Notes:"
	@echo "  - Plugin tasks require pnpm (Node.js 25 does not ship Corepack)."
	@echo "    Install: npm install -g pnpm@10.17.1"

install: backend-install assets-install plugin-install

backend-install:
	@echo "Installing backend dependencies..."
	@cd server-go && go mod download

assets-install:
	@echo "Installing Tailwind CSS dependencies..."
	@cd server && npm install

plugin-install:
	@echo "Installing plugin dependencies..."
	@cd obsidian-plugin && pnpm install --frozen-lockfile

dev:
	@echo "Starting backend development server + asset watches..."
	@echo "App Port: $(APP_PORT), Console Port: $(CONSOLE_PORT)"
	@echo "Data Path: $(BACKEND_DATA_PATH)"
	@echo "Use Ctrl+C to stop all processes"
	@cd server-go && DATA_PATH=$(BACKEND_DATA_PATH) go run ./cmd/mdbrain-migrate migrate
	@set -e; \
	( cd server-go && DATA_PATH=$(BACKEND_DATA_PATH) APP_PORT=$(APP_PORT) CONSOLE_PORT=$(CONSOLE_PORT) go run ./cmd/mdbrain ) & \
	BACKEND_PID=$$!; \
	( cd server && npm run watch:console ) & \
	CONSOLE_WATCH_PID=$$!; \
	( cd server && npm run watch:app ) & \
	APP_WATCH_PID=$$!; \
	trap 'kill $$BACKEND_PID $$CONSOLE_WATCH_PID $$APP_WATCH_PID || true' INT TERM; \
	wait $$BACKEND_PID $$CONSOLE_WATCH_PID $$APP_WATCH_PID

backend-dev:
	@echo "Starting backend development server..."
	@echo "App Port: $(APP_PORT), Console Port: $(CONSOLE_PORT)"
	@echo "Data Path: $(BACKEND_DATA_PATH)"
	@cd server-go && DATA_PATH=$(BACKEND_DATA_PATH) go run ./cmd/mdbrain-migrate migrate
	@cd server-go && DATA_PATH=$(BACKEND_DATA_PATH) APP_PORT=$(APP_PORT) CONSOLE_PORT=$(CONSOLE_PORT) go run ./cmd/mdbrain

backend-repl:
	@echo "Go backend has no REPL."
	@echo "Use 'make backend-dev' for a live server or 'cd server-go && go test ./...' for targeted debugging."

assets-dev:
	@echo "Starting Tailwind CSS watch mode..."
	@cd server && npm run watch

plugin-dev:
	@echo "Starting Obsidian plugin development mode..."
	@cd obsidian-plugin && pnpm dev

build: backend-build assets-build plugin-build

backend-build:
	@echo "Building backend binaries..."
	@mkdir -p server-go/target
	@cd server-go && go build -o ./target/mdbrain ./cmd/mdbrain
	@cd server-go && go build -o ./target/mdbrain-migrate ./cmd/mdbrain-migrate
	@echo "Backend built:"
	@echo "  - server-go/target/mdbrain"
	@echo "  - server-go/target/mdbrain-migrate"

assets-build:
	@echo "Building Tailwind CSS..."
	@cd server && npm run build
	@echo "CSS built:"
	@echo "  - server/resources/publics/console/css/console.css"
	@echo "  - server/resources/publics/app/css/app.css"

plugin-build:
	@echo "Building Obsidian plugin..."
	@cd obsidian-plugin && pnpm build
	@echo "Plugin built: obsidian-plugin/dist/"

plugin-package:
	@echo "Packaging Obsidian plugin..."
	@cd obsidian-plugin && pnpm package
	@echo "Plugin packaged: obsidian-plugin/mdbrain-plugin.zip"

test: backend-test plugin-test

backend-test:
	@echo "Running backend tests..."
	@cd server-go && go test ./...

plugin-test:
	@echo "Running plugin tests..."
	@cd obsidian-plugin && pnpm test

# Database
db-migrate:
	@echo "Running database migrations..."
	@cd server-go && DATA_PATH=$(BACKEND_DATA_PATH) go run ./cmd/mdbrain-migrate migrate

db-reset:
	@echo "Resetting database..."
	@rm -f $(BACKEND_DATA_PATH)/mdbrain.db $(BACKEND_DATA_PATH)/.secrets.edn $(BACKEND_DATA_PATH)/.health-token
	@cd server-go && DATA_PATH=$(BACKEND_DATA_PATH) go run ./cmd/mdbrain-migrate migrate
	@echo "Database reset complete"

db-pending:
	@echo "Checking pending migrations..."
	@cd server-go && DATA_PATH=$(BACKEND_DATA_PATH) go run ./cmd/mdbrain-migrate pending

db-create-migration:
	@echo "Generating versioned migration from ent schema..."
	@test -n "$(NAME)" || (echo "Usage: make db-create-migration NAME=your_migration_name" && exit 1)
	@cd server-go && DATA_PATH=$(BACKEND_DATA_PATH) go run ./cmd/mdbrain-migrate create "$(NAME)"

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf server/target/
	@rm -rf server/.cpcache/
	@rm -rf server-go/target/
	@rm -rf obsidian-plugin/dist/
	@rm -f obsidian-plugin/main.js
	@rm -f obsidian-plugin/main.js.map
	@rm -f obsidian-plugin/*.zip
	@echo "Clean complete"
