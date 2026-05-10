.PHONY: \
	help \
	install backend-install assets-install plugin-install \
	dev backend-dev backend-repl assets-dev plugin-dev \
	build backend-build dotnet-build assets-build plugin-build plugin-package \
	test backend-test dotnet-test plugin-test \
	db-reset \
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
	@echo "  make dotnet-build                  Build experimental .NET backend"
	@echo "  make assets-build                  Build Tailwind CSS (console + app)"
	@echo "  make plugin-build                  Build Obsidian plugin to dist/"
	@echo "  make plugin-package                Package plugin zip (mdbrain-plugin.zip)"
	@echo ""
	@echo "Test:"
	@echo "  make test                          Run backend + plugin tests"
	@echo "  make backend-test                  Run backend tests (go test ./...)"
	@echo "  make dotnet-test                   Run experimental .NET backend tests"
	@echo "  make plugin-test                   Run plugin tests (pnpm test)"
	@echo ""
	@echo "Database:"
	@echo "  make db-reset                      Delete local DB; schema is recreated on next start"
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
	@echo "Building backend binary..."
	@mkdir -p server-go/target
	@cd server-go && go build -o ./target/mdbrain ./cmd/mdbrain
	@echo "Backend built: server-go/target/mdbrain"

dotnet-build:
	@echo "Building experimental .NET backend..."
	@mise exec dotnet@10.0.203 -- dotnet build server-dotnet/src/Mdbrain/Mdbrain.csproj --no-restore
	@mise exec dotnet@10.0.203 -- dotnet build server-dotnet/tests/Mdbrain.Tests/Mdbrain.Tests.csproj --no-restore

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

dotnet-test:
	@echo "Running experimental .NET backend tests..."
	@mise exec dotnet@10.0.203 -- dotnet build server-dotnet/tests/Mdbrain.Tests/Mdbrain.Tests.csproj --no-restore
	@mise exec dotnet@10.0.203 -- dotnet server-dotnet/tests/Mdbrain.Tests/bin/Debug/net10.0/Mdbrain.Tests.dll

plugin-test:
	@echo "Running plugin tests..."
	@cd obsidian-plugin && pnpm test

db-reset:
	@echo "Resetting database..."
	@rm -f $(BACKEND_DATA_PATH)/mdbrain.db $(BACKEND_DATA_PATH)/.secrets.edn $(BACKEND_DATA_PATH)/.health-token
	@echo "Database reset complete"

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf server/target/
	@rm -rf server/.cpcache/
	@rm -rf server-go/target/
	@rm -rf server-dotnet/**/bin/
	@rm -rf server-dotnet/**/obj/
	@rm -rf server-dotnet/.testdata/
	@rm -rf obsidian-plugin/dist/
	@rm -f obsidian-plugin/main.js
	@rm -f obsidian-plugin/main.js.map
	@rm -f obsidian-plugin/*.zip
	@echo "Clean complete"
