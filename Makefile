.PHONY: all build run generate tidy test clean

all: build

build:
	@echo "Building NMSlite server..."
	@go build -v -o bin/nmslite ./cmd/server

run: build
	@echo "Starting NMSlite server..."
	@./bin/nmslite

generate:
	@echo "Generating sqlc code..."
	@sqlc generate

tidy:
	@echo "Running go mod tidy..."
	@go mod tidy

test:
	@echo "Running tests..."
	@go test -v ./...

clean:
	@echo "Cleaning up build artifacts..."
	@rm -f bin/nmslite
	@rm -rf internal/database/db_gen

# ==============================================================================
# Plugin Build Targets
# ==============================================================================

# Build WinRM plugin (separate Go module)
.PHONY: build-plugin-winrm
build-plugin-winrm:
	@echo "Building windows-winrm plugin..."
	@mkdir -p plugin_bins/windows-winrm
	cd plugins/windows-winrm && go mod tidy && go build -o ../../plugin_bins/windows-winrm/windows-winrm .
	cp plugins/windows-winrm/manifest.json plugin_bins/windows-winrm/manifest.json
	@echo "Plugin built: plugin_bins/windows-winrm/"

# Build all plugins
.PHONY: build-plugins
build-plugins: build-plugin-winrm

# Clean plugin binaries
.PHONY: clean-plugins
clean-plugins:
	rm -rf plugin_bins/