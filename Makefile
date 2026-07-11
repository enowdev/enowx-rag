.PHONY: web build dev-web dev-api mcp test vet clean placeholder

# Go binary output name
BINARY := enowx-rag

# Directories
MCP_DIR := mcp-server
WEB_DIR := mcp-server/web

# Go build flags
GO_FLAGS := -trimpath

# web: Build the React SPA into web/dist using npm ci + npm run build.
# This ensures a clean install of dependencies followed by a production build.
web:
	cd $(WEB_DIR) && npm ci && npm run build

# build: Full build. Depends on web target to ensure web/dist exists for embed.FS.
# Produces a single self-contained binary at the repository root.
build: web
	cd $(MCP_DIR) && go build $(GO_FLAGS) -o ../$(BINARY) ./cmd/mcp-server

# mcp: Build only the MCP server binary without building the web UI.
# Uses a placeholder web/dist if it doesn't exist, so embed.FS compiles.
# The resulting binary works in stdio MCP mode (no --serve UI).
mcp:
	@if [ ! -d $(WEB_DIR)/dist ]; then \
		mkdir -p $(WEB_DIR)/dist; \
		echo '<!DOCTYPE html><html><body>enowx-rag UI not built. Run make web to build the SPA.</body></html>' > $(WEB_DIR)/dist/index.html; \
		echo "Created web/dist placeholder for MCP-only build"; \
	fi
	cd $(MCP_DIR) && go build $(GO_FLAGS) -o mcp-server ./cmd/mcp-server

# dev-web: Start Vite dev server for frontend development.
dev-web:
	cd $(WEB_DIR) && npm run dev

# dev-api: Start the Go HTTP server in serve mode for backend development.
dev-api:
	cd $(MCP_DIR) && go run ./cmd/mcp-server --serve --addr :7777

# test: Run all Go tests.
test:
	cd $(MCP_DIR) && go test ./... -count=1

# vet: Run go vet for static analysis.
vet:
	cd $(MCP_DIR) && go vet ./...

# placeholder: Create a minimal web/dist placeholder so go build succeeds
# without a full web build. Useful for MCP-only development.
placeholder:
	@if [ ! -f $(WEB_DIR)/dist/index.html ]; then \
		mkdir -p $(WEB_DIR)/dist; \
		echo '<!DOCTYPE html><html><body>enowx-rag UI not built. Run make web to build the SPA.</body></html>' > $(WEB_DIR)/dist/index.html; \
		echo "Created web/dist placeholder"; \
	else \
		echo "web/dist/index.html already exists"; \
	fi

# clean: Remove build artifacts.
clean:
	rm -f $(MCP_DIR)/$(BINARY) $(BINARY)
