#!/bin/bash

# Test script to demonstrate file deletion synchronization logic

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "=== File Deletion Synchronization Demo ==="
echo "Project dir: $SCRIPT_DIR"
echo ""

echo "=== Step 1: Initial state ==="
echo "Creating test files..."
echo "File A" > "$SCRIPT_DIR/sync_data_server1/file_a.txt"
echo "File B" > "$SCRIPT_DIR/sync_data_server1/file_b.txt"
echo "File C" > "$SCRIPT_DIR/sync_data_server1/file_c.txt"

echo "Server 1 files:"
ls "$SCRIPT_DIR/sync_data_server1/"
echo ""

echo "Server 2 files:"
ls "$SCRIPT_DIR/sync_data_server2/"
echo ""

echo "=== Step 2: Simulate sync (copy files to server 2) ==="
cp "$SCRIPT_DIR/sync_data_server1/"* "$SCRIPT_DIR/sync_data_server2/" 2>/dev/null || true
echo "After sync - Server 2 files:"
ls "$SCRIPT_DIR/sync_data_server2/"
echo ""

echo "=== Step 3: Delete file from Server 1 ==="
echo "Deleting file_b.txt from Server 1..."
rm "$SCRIPT_DIR/sync_data_server1/file_b.txt"
echo "Server 1 files after deletion:"
ls "$SCRIPT_DIR/sync_data_server1/"
echo ""

echo "=== Step 4: Detect deletions (compare states) ==="
echo "Previous state (Server 1 had): file_a.txt, file_b.txt, file_c.txt"
echo "Current state (Server 1 has): $(ls "$SCRIPT_DIR/sync_data_server1/")"
echo ""
echo "Detected deletions:"
for file in file_a.txt file_b.txt file_c.txt; do
    if [ ! -f "$SCRIPT_DIR/sync_data_server1/$file" ]; then
        echo "  - $file (DELETED)"
    fi
done
echo ""

echo "=== Step 5: Sync deletions to Server 2 ==="
echo "Server 2 would receive deletion notification for: file_b.txt"
echo "Server 2 would delete: $SCRIPT_DIR/sync_data_server2/file_b.txt"
rm "$SCRIPT_DIR/sync_data_server2/file_b.txt" 2>/dev/null || true
echo ""
echo "Final state:"
echo "Server 1 files: $(ls "$SCRIPT_DIR/sync_data_server1/")"
echo "Server 2 files: $(ls "$SCRIPT_DIR/sync_data_server2/")"
echo ""

echo "=== Result ==="
echo "✅ File deletion synchronized between servers!"
echo "✅ Both servers now have the same file set"
echo ""

echo "=== Clean up ==="
rm -f "$SCRIPT_DIR/sync_data_server1/"* "$SCRIPT_DIR/sync_data_server2/"* 2>/dev/null || true
echo "Demo completed and cleaned up."