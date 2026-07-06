.PHONY: \
	help \
	install workspace-install backend-install assets-install plugin-install \
	dev backend-dev backend-repl assets-dev plugin-dev \
	build backend-build assets-build plugin-build plugin-package \
	test backend-test plugin-test \
	db-migrate db-pending db-create-migration db-reset \
	clean

help:
	@echo "Mdbrain - Developer commands"
	@echo ""
	@echo "Development:"
	@echo "  make dev                           Start Bun web server in watch mode + asset watch (APP_PORT/CONSOLE_PORT)"
	@echo "  make web-dev                       Start Bun web server in watch mode (console + public app + sync)"
	@echo "  make backend-repl                  Start backend REPL (no server)"
	@echo "  make assets-dev                    Watch and rebuild Tailwind CSS (console + app)"
	@echo "  make plugin-dev                    Watch Obsidian plugin (vaults/test)"
	@echo ""
	@echo "Build:"
	@echo "  make build                         Build Bun web app + CSS + plugin"
	@echo "  make web-build                     Build Bun web app"
	@echo "  make backend-build                 Build backend uberjar"
	@echo "  make assets-build                  Build Tailwind CSS (console + app)"
	@echo "  make plugin-build                  Build Obsidian plugin to dist/"
	@echo "  make plugin-package                Package plugin zip (mdbrain-plugin.zip)"
	@echo ""
	@echo "Test:"
	@echo "  make test                          Run Bun web + plugin tests"
	@echo "  make web-test                      Run Bun web/package tests"
	@echo "  make backend-test                  Run backend tests (clojure -M:test)"
	@echo "  make plugin-test                   Run plugin tests (bunx vitest)"
	@echo ""
	@echo "Database:"
	@echo "  make db-migrate                    Run migrations (migratus)"
	@echo "  make db-pending                    List pending migrations"
	@echo "  make db-create-migration NAME=xxx  Create a new migration file"
	@echo "  make db-reset                      Delete local DB and rerun migrations"
	@echo ""
	@echo "Maintenance:"
	@echo "  make install                       Install Bun workspace dependencies"
	@echo "  make clean                         Remove build outputs"

install: workspace-install

workspace-install:
	@echo "Installing root Bun workspace dependencies..."
	@bun install

backend-install:
	@echo "Installing backend dependencies..."
	@cd server && clojure -P -M:dev:test

assets-install:
	@echo "Installing Tailwind CSS dependencies..."
	@bun install --cwd server

plugin-install:
	@echo "Installing plugin dependencies..."
	@bun install --cwd obsidian-plugin

APP_PORT ?= 8080
CONSOLE_PORT ?= 9090

dev:
	@echo "Starting Bun web server in watch mode + asset watch..."
	@echo "App Port: $(APP_PORT), Console Port: $(CONSOLE_PORT)"
	@echo "Use Ctrl+C to stop all processes"
	@set -e; \
	APP_PORT=$(APP_PORT) CONSOLE_PORT=$(CONSOLE_PORT) bun run dev:web & \
	WEB_PID=$$!; \
	bun run --cwd server watch & \
	ASSETS_PID=$$!; \
	trap 'kill $$WEB_PID $$ASSETS_PID || true' INT TERM; \
	wait $$WEB_PID $$ASSETS_PID

web-dev:
	@echo "Starting Bun web server in watch mode..."
	@bun run dev:web

backend-dev:
	@echo "Starting backend development server..."
	@echo "App Port: $(APP_PORT), Console Port: $(CONSOLE_PORT)"
	@cd server && APP_PORT=$(APP_PORT) CONSOLE_PORT=$(CONSOLE_PORT) MDBRAIN_LOG_LEVEL=DEBUG clojure -M:dev

backend-repl:
	@echo "Starting backend REPL..."
	@cd server && clojure -M:repl

assets-dev:
	@echo "Starting Tailwind CSS watch mode..."
	@bun run --cwd server watch

plugin-dev:
	@echo "Starting Obsidian plugin development mode..."
	@bun run --cwd obsidian-plugin dev

build: assets-build plugin-build web-build

backend-build:
	@echo "Building backend uberjar..."
	@cd server && clojure -T:build uberjar
	@echo "Backend built: server/target/server-standalone.jar"

assets-build:
	@echo "Building Tailwind CSS..."
	@bun run --cwd server build
	@echo "CSS built:"
	@echo "  - server/resources/publics/console/css/console.css"
	@echo "  - server/resources/publics/app/css/app.css"

plugin-build:
	@echo "Building Obsidian plugin..."
	@bun run --cwd obsidian-plugin build
	@echo "Plugin built: obsidian-plugin/dist/"

plugin-package:
	@echo "Packaging Obsidian plugin..."
	@bun run --cwd obsidian-plugin package
	@echo "Plugin packaged: obsidian-plugin/mdbrain-plugin.zip"

web-build:
	@echo "Building Bun web app..."
	@bun run build:web

test: plugin-test web-test

backend-test:
	@echo "Running backend tests..."
	@cd server && clojure -M:test

plugin-test:
	@echo "Running plugin tests..."
	@bun run --cwd obsidian-plugin test

web-test:
	@echo "Running Bun web tests..."
	@bun run test:web

# Database
db-migrate:
	@echo "Running database migrations..."
	@cd server && clojure -M -m mdbrain.migrations migrate

db-reset:
	@echo "Resetting database..."
	@rm -f data/mdbrain.db data/.secrets.edn
	@cd server && clojure -M -m mdbrain.migrations migrate
	@echo "Database reset complete"

db-pending:
	@echo "Checking pending migrations..."
	@cd server && clojure -M -m mdbrain.migrations pending

db-create-migration:
	@echo "Creating new migration..."
	@cd server && clojure -M -m mdbrain.migrations create $(NAME)

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf server/target/
	@rm -rf server/.cpcache/
	@rm -rf obsidian-plugin/dist/
	@rm -rf apps/web/dist/
	@rm -f obsidian-plugin/main.js
	@rm -f obsidian-plugin/main.js.map
	@rm -f obsidian-plugin/*.zip
	@echo "Clean complete"
