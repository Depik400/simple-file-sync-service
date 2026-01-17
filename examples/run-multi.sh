#!/bin/bash

# Multi-server testing script for File Sync
# This script runs multiple file sync servers for testing synchronization

set -e  # Exit on any error

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

# Check if binary exists
if [ ! -f "$PROJECT_ROOT/file-sync" ]; then
    print_error "Binary not found: $PROJECT_ROOT/file-sync"
    print_info "Please build the project first:"
    echo "  cd $PROJECT_ROOT && go build"
    exit 1
fi

# Create necessary directories
print_status "Creating directories..."
mkdir -p "$SCRIPT_DIR/logs"
mkdir -p "$SCRIPT_DIR/sync_data_server1"
mkdir -p "$SCRIPT_DIR/sync_data_server2"

# Cleanup function
cleanup() {
    print_warning "Stopping servers..."
    if [ -f "$SCRIPT_DIR/server1.pid" ]; then
        kill $(cat "$SCRIPT_DIR/server1.pid") 2>/dev/null || true
        rm -f "$SCRIPT_DIR/server1.pid"
    fi
    if [ -f "$SCRIPT_DIR/server2.pid" ]; then
        kill $(cat "$SCRIPT_DIR/server2.pid") 2>/dev/null || true
        rm -f "$SCRIPT_DIR/server2.pid"
    fi
    print_status "Servers stopped"
}

# Set trap for cleanup
trap cleanup EXIT INT TERM

# Function to start a server
start_server() {
    local server_num=$1
    local config_file="$SCRIPT_DIR/config-server${server_num}.yaml"
    local log_file="$SCRIPT_DIR/logs/server${server_num}.log"
    local pid_file="$SCRIPT_DIR/server${server_num}.pid"

    if [ ! -f "$config_file" ]; then
        print_error "Config file not found: $config_file"
        return 1
    fi

    print_status "Starting Server $server_num..."
    print_info "  Config: $config_file"
    print_info "  Log: $log_file"

    # Clean up any existing PID file
    rm -f "$pid_file"

    # Change to project root directory so binary can find relative paths
    cd "$PROJECT_ROOT"

    print_info "  Working directory: $(pwd)"
    print_info "  Binary exists: $([ -f ./file-sync ] && echo 'YES' || echo 'NO')"

    # Start server in background
    ./file-sync "$config_file" > "$log_file" 2>&1 &
    local pid=$!
    echo $pid > "$pid_file"

    print_status "Server $server_num started with PID $pid"

    # Don't wait here - servers will be checked later in main()
    return 0
}

# Function to show server status
show_servers() {
    echo ""
    print_info "=== File Sync Servers Running ==="
    echo "Server 1:"
    echo "  Web UI: http://localhost:8081"
    echo "  P2P Port: 8080"
    echo "  Sync Dir: $SCRIPT_DIR/sync_data_server1"
    echo "  Logs: $SCRIPT_DIR/logs/server1.log"
    if [ -f "$SCRIPT_DIR/server1.pid" ]; then
        echo "  PID: $(cat "$SCRIPT_DIR/server1.pid")"
    fi

    echo ""
    echo "Server 2:"
    echo "  Web UI: http://localhost:8083"
    echo "  P2P Port: 8082"
    echo "  Sync Dir: $SCRIPT_DIR/sync_data_server2"
    echo "  Logs: $SCRIPT_DIR/logs/server2.log"
    if [ -f "$SCRIPT_DIR/server2.pid" ]; then
        echo "  PID: $(cat "$SCRIPT_DIR/server2.pid")"
    fi

    echo ""
    print_info "=== Testing ==="
    echo "1. Create files in sync_data_server1/"
    echo "2. Watch logs for synchronization"
    echo "3. Check files appear in sync_data_server2/"
    echo ""
    print_warning "Press Ctrl+C to stop all servers"
}

# Main execution
main() {
    print_status "File Sync Multi-Server Testing"
    print_info "Project root: $PROJECT_ROOT"
    print_info "Examples dir: $SCRIPT_DIR"

    # Try to start servers in parallel
    print_info "Attempting to start servers in parallel..."

    # Start both servers simultaneously
    start_server 1 &
    SERVER1_PID=$!

    start_server 2 &
    SERVER2_PID=$!

    # Wait a bit for both servers to initialize
    print_info "Waiting for servers to initialize..."
    sleep 5

    # Check if both servers started successfully
    if [ -f "$SCRIPT_DIR/server1.pid" ] && [ -f "$SCRIPT_DIR/server2.pid" ] &&
       kill -0 $(cat "$SCRIPT_DIR/server1.pid") 2>/dev/null &&
       kill -0 $(cat "$SCRIPT_DIR/server2.pid") 2>/dev/null; then

        # Show status
        show_servers

        # Wait for user interrupt
        print_status "All servers started successfully!"
        print_info "Waiting for Ctrl+C to stop..."
        print_info "Watch the logs for synchronization messages!"

        # Keep running until interrupted
        while true; do
            sleep 1

            # Check if servers are still running
            if [ -f "$SCRIPT_DIR/server1.pid" ] && ! kill -0 $(cat "$SCRIPT_DIR/server1.pid") 2>/dev/null; then
                print_error "Server 1 stopped unexpectedly"
                exit 1
            fi
            if [ -f "$SCRIPT_DIR/server2.pid" ] && ! kill -0 $(cat "$SCRIPT_DIR/server2.pid") 2>/dev/null; then
                print_error "Server 2 stopped unexpectedly"
                exit 1
            fi
        done
    else
        print_error "Failed to start servers"
        show_demo_info
        return 1
    fi
}

show_demo_info() {
    echo ""
    print_info "=== Demo Mode - Manual Server Startup ==="
    echo ""
    print_info "To run servers manually (in different terminals):"
    echo ""
    echo "Terminal 1 - Server 1:"
    echo "  cd $PROJECT_ROOT"
    echo "  ./file-sync examples/config-server1.yaml"
    echo ""
    echo "Terminal 2 - Server 2:"
    echo "  cd $PROJECT_ROOT"
    echo "  ./file-sync examples/config-server2.yaml"
    echo ""
    print_info "Server URLs when running:"
    echo "  Server 1 Web UI: http://localhost:9101"
    echo "  Server 2 Web UI: http://localhost:9103"
    echo ""
    print_info "Test synchronization:"
    echo "  1. Create files in examples/sync_data_server1/"
    echo "  2. Watch server logs for sync messages"
    echo "  3. Files should appear in examples/sync_data_server2/"
    echo ""
    print_info "Example test files are already created:"
    echo "  examples/sync_data_server1/welcome.txt"
    echo "  examples/sync_data_server1/data.json"
}

# Run main function
main "$@"