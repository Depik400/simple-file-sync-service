# File Sync Examples

This directory contains example configurations and scripts for running multiple File Sync servers.

## Available Examples

### Basic Multi-Server Setup

**Files:**
- `config-server1.yaml` - Configuration for Server 1
- `config-server2.yaml` - Configuration for Server 2
- `run-multi.sh` - Script to run both servers

**How to run:**
```bash
cd examples
./run-multi.sh     # Start both servers for testing
./test-sync.sh     # Demonstrate sync logic without network
```

This will start two servers:
- Server 1: http://localhost:9101 (P2P port: 9100)
- Server 2: http://localhost:9103 (P2P port: 9102)

### Server Configuration

Each server has its own:
- Sync directory (sync_data_server1, sync_data_server2)
- Database file (file_sync_server1.db, file_sync_server2.db)
- Log files in logs/ directory
- Unique ports for web interface and P2P communication

### Testing Synchronization

1. Start both servers using `run-multi.sh`
2. Create files in one server's sync directory
3. Watch the logs to see automatic synchronization
4. Check the other server's sync directory for replicated files

### Adding More Servers

To add a third server:
1. Copy `config-server2.yaml` to `config-server3.yaml`
2. Update ports (web_port: 8085, port: 8084)
3. Update server name to "server-3"
4. Update sync_dir to "./sync_data_server3"
5. Update database path to "./file_sync_server3.db"
6. Add new peer configurations to all server configs
7. Update `run-multi.sh` to start the third server

### Logs and Debugging

- Server logs are saved in `logs/` directory
- Each server has its own log file
- Use the detailed logging to troubleshoot sync issues