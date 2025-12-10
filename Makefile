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
