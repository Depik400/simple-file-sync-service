#!/bin/bash

# Demo script to run File Sync servers in demo mode (no network)
# This allows testing the sync logic without network restrictions

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

# Function to start a demo server
start_demo_server() {
    local server_num=$1
    local config_file="$SCRIPT_DIR/config-server${server_num}.yaml"
    local log_file="$SCRIPT_DIR/logs/demo-server${server_num}.log"

    if [ ! -f "$config_file" ]; then
        print_error "Config file not found: $config_file"
        return 1
    fi

    print_status "Starting Demo Server $server_num..."
    print_info "  Config: $config_file"
    print_info "  Log: $log_file"

    # Change to project root directory
    cd "$PROJECT_ROOT"

    # Start server in demo mode
    print_info "  Starting: ./file-sync --demo $config_file"
    ./file-sync --demo "$config_file" > "$log_file" 2>&1 &
    local pid=$!
    echo $pid > "$SCRIPT_DIR/demo-server${server_num}.pid"

    print_status "Demo Server $server_num started with PID $pid"
    sleep 3

    # Check if server is still running
    if ! kill -0 $pid 2>/dev/null; then
        print_error "Demo Server $server_num failed to start. Check logs: $log_file"
        print_info "  Last few lines of log:"
        tail -5 "$log_file" 2>/dev/null || echo "    No log file found"
        return 1
    fi

    print_status "Demo Server $server_num is running successfully"
    return 0
}

# Function to show demo status
show_demo_status() {
    echo ""
    print_info "=== File Sync Demo Servers Running ==="
    echo "Demo Server 1:"
    echo "  Config: examples/config-server1.yaml"
    echo "  Sync Dir: examples/sync_data_server1"
    echo "  Log: examples/logs/demo-server1.log"
    if [ -f "$SCRIPT_DIR/demo-server1.pid" ]; then
        echo "  PID: $(cat "$SCRIPT_DIR/demo-server1.pid")"
    fi

    echo ""
    echo "Demo Server 2:"
    echo "  Config: examples/config-server2.yaml"
    echo "  Sync Dir: examples/sync_data_server2"
    echo "  Log: examples/logs/demo-server2.log"
    if [ -f "$SCRIPT_DIR/demo-server2.pid" ]; then
        echo "  PID: $(cat "$SCRIPT_DIR/demo-server2.pid")"
    fi

    echo ""
    print_info "=== Demo Features ==="
    echo "✓ No network required"
    echo "✓ File scanning and hashing"
    echo "✓ Database operations"
    echo "✓ Sync logic simulation"
    echo "✓ Automatic sync every 30 seconds"
    echo ""
    print_info "=== Testing ==="
    echo "1. Create files in examples/sync_data_server1/"
    echo "2. Watch server logs for sync messages"
    echo "3. Files are processed and logged locally"
    echo ""
    print_warning "Press Ctrl+C to stop demo servers"
}

# Cleanup function
cleanup() {
    print_warning "Stopping demo servers..."
    if [ -f "$SCRIPT_DIR/demo-server1.pid" ]; then
        kill $(cat "$SCRIPT_DIR/demo-server1.pid") 2>/dev/null || true
        rm -f "$SCRIPT_DIR/demo-server1.pid"
    fi
    if [ -f "$SCRIPT_DIR/demo-server2.pid" ]; then
        kill $(cat "$SCRIPT_DIR/demo-server2.pid") 2>/dev/null || true
        rm -f "$SCRIPT_DIR/demo-server2.pid"
    fi
    print_status "Demo servers stopped"
}

# Set trap for cleanup
trap cleanup EXIT INT TERM

# Main execution
main() {
    print_status "Starting File Sync Demo Servers (No Network Mode)"
    print_info "Project root: $PROJECT_ROOT"
    print_info "Examples dir: $SCRIPT_DIR"

    # Start demo servers
    print_info "Starting servers in demo mode..."

    if start_demo_server 1 && start_demo_server 2; then
        # Show status
        show_demo_status

        # Wait for user interrupt
        print_status "Demo servers started successfully!"
        print_info "Watch the logs for sync activity..."
        print_info "Create files in examples/sync_data_server1/ to test sync logic"

        # Keep running until interrupted
        while true; do
            sleep 1

            # Check if servers are still running
            if [ -f "$SCRIPT_DIR/demo-server1.pid" ] && ! kill -0 $(cat "$SCRIPT_DIR/demo-server1.pid") 2>/dev/null; then
                print_error "Demo Server 1 stopped unexpectedly"
                exit 1
            fi
            if [ -f "$SCRIPT_DIR/demo-server2.pid" ] && ! kill -0 $(cat "$SCRIPT_DIR/demo-server2.pid") 2>/dev/null; then
                print_error "Demo Server 2 stopped unexpectedly"
                exit 1
            fi
        done
    else
        print_error "Failed to start demo servers"
        return 1
    fi
}

# Run main function
main "$@"