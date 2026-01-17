.PHONY: build run run-multi clean test deps

# Build the project
build:
	go build -o bin/file-sync main.go

# Run single server
run:
	go run main.go

# Run multiple servers for testing
run-multi:
	./examples/run-multi.sh

# Run servers in demo mode (no network)
run-demo:
	./examples/run-demo.sh

# Quick demo test
demo-test:
	@echo "Creating directories and test file..."
	@mkdir -p examples/logs examples/sync_data_server1 examples/sync_data_server2
	@echo "Demo test file" > examples/sync_data_server1/demo_test.txt
	@echo "Starting demo servers..."
	@./file-sync --demo examples/config-server1.yaml > examples/logs/demo-server1.log 2>&1 & echo $$! > examples/demo-server1.pid
	@./file-sync --demo examples/config-server2.yaml > examples/logs/demo-server2.log 2>&1 & echo $$! > examples/demo-server2.pid
	@echo "Demo servers started. Waiting 15 seconds for sync..."
	@sleep 15
	@echo "Checking results..."
	@echo "=== Server 1 files ==="
	@ls -la examples/sync_data_server1/
	@echo ""
	@echo "=== Server 2 files ==="
	@ls -la examples/sync_data_server2/
	@echo ""
	@echo "Stopping demo servers..."
	@-kill $$(cat examples/demo-server1.pid) 2>/dev/null || true
	@-kill $$(cat examples/demo-server2.pid) 2>/dev/null || true
	@rm -f examples/demo-server*.pid
	@echo "Demo test completed!"

# Demonstrate sync logic without network
test-sync:
	./examples/test-sync.sh

# Demonstrate deletion sync logic
test-deletions:
	./examples/test-deletions.sh

# Clean build artifacts
clean:
	rm -rf bin/ logs/ sync_data*/ *.db examples/logs/ examples/sync_data*/ examples/*.db examples/*.pid

# Run tests
test:
	go test ./...

# Download dependencies
deps:
	go mod tidy
	go mod download

# Create necessary directories
init:
	mkdir -p logs sync_data sync_data_server2 sync_data_server3 bin

# Show help
help:
	@echo "Available commands:"
	@echo "  build        - Build the project"
	@echo "  run          - Run single server with default config"
	@echo "  run-multi    - Run multiple servers for testing"
	@echo "  run-demo     - Run servers in demo mode (no network)"
	@echo "  demo-test    - Quick demo test with file sync simulation"
	@echo "  test-sync    - Demonstrate sync logic without network dependencies"
	@echo "  test-deletions - Demonstrate deletion sync logic"
	@echo "  clean        - Clean build artifacts"
	@echo "  test         - Run tests"
	@echo "  deps         - Download dependencies"
	@echo "  init         - Create necessary directories"
	@echo "  help         - Show this help"