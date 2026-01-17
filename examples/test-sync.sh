#!/bin/bash

# Test script to demonstrate file synchronization logic
# This simulates what happens during actual sync without network dependencies

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "=== File Sync Logic Demonstration ==="
echo "Project root: $PROJECT_ROOT"
echo "Examples dir: $SCRIPT_DIR"
echo ""

echo "=== Server 1 File Scan ==="
echo "Scanning: $SCRIPT_DIR/sync_data_server1"
ls -la "$SCRIPT_DIR/sync_data_server1/"
echo ""

echo "=== Server 2 File Scan ==="
echo "Scanning: $SCRIPT_DIR/sync_data_server2"
ls -la "$SCRIPT_DIR/sync_data_server2/"
echo ""

echo "=== Synchronization Logic ==="
echo "Server 1 would broadcast:"
find "$SCRIPT_DIR/sync_data_server1" -type f -exec basename {} \; | while read file; do
    echo "  - $file"
done
echo ""

echo "Server 2 would broadcast:"
find "$SCRIPT_DIR/sync_data_server2" -type f -exec basename {} \; | while read file; do
    echo "  - $file"
done
echo ""

echo "=== Sync Results ==="
echo "Files that would be downloaded to Server 2:"
find "$SCRIPT_DIR/sync_data_server1" -type f -exec basename {} \; | while read file; do
    if [ ! -f "$SCRIPT_DIR/sync_data_server2/$file" ]; then
        echo "  + $file (new file)"
    fi
done
echo ""

echo "Files that would be downloaded to Server 1:"
find "$SCRIPT_DIR/sync_data_server2" -type f -exec basename {} \; | while read file; do
    if [ ! -f "$SCRIPT_DIR/sync_data_server1/$file" ]; then
        echo "  + $file (new file)"
    fi
done
echo ""

echo "=== Test: Create a new file ==="
echo "Creating test file on Server 1..."
echo "This is a test file created at $(date)" > "$SCRIPT_DIR/sync_data_server1/test_sync_$(date +%s).txt"
echo "New file created: $(ls -la "$SCRIPT_DIR/sync_data_server1/" | tail -1)"
echo ""

echo "=== Updated Sync Status ==="
echo "Server 1 now has:"
find "$SCRIPT_DIR/sync_data_server1" -type f -exec basename {} \; | nl
echo ""
echo "Server 2 now has:"
find "$SCRIPT_DIR/sync_data_server2" -type f -exec basename {} \; | nl
echo ""

echo "=== Next Sync Would Transfer ==="
find "$SCRIPT_DIR/sync_data_server1" -type f -exec basename {} \; | while read file; do
    if [ ! -f "$SCRIPT_DIR/sync_data_server2/$file" ]; then
        echo "  -> $file to Server 2"
    fi
done
echo ""

echo "=== Demonstration Complete ==="
echo "This shows the file synchronization logic without network dependencies."
echo "In a real scenario, these file transfers would happen automatically every 30 seconds."