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

This will start three servers:
- Server 1: http://localhost:7071 (P2P port: 7071)
- Server 2: http://localhost:8090 (P2P port: 8090)
- Server 3: http://localhost:7073 (P2P port: 7073)

### Server Configuration

Each server has its own:
- Sync directory (sync_data_server1, sync_data_server2, sync_data_server3)
- Database file (file_sync_server1.db, file_sync_server2.db, file_sync_server3.db)
- Log files in logs/ directory
- Unique ports for web interface and P2P communication

### Testing Synchronization

1. Start both servers using `run-multi.sh`
2. Create files in one server's sync directory
3. Watch the logs to see automatic synchronization
4. Check the other server's sync directory for replicated files

### Adding More Servers

To add a fourth server:
1. Copy `config-server3.yaml` to `config-server4.yaml`
2. Update ports (web_port: 8087, port: 8086)
3. Update server name to "server-4"
4. Update sync_dir to "./sync_data_server4"
5. Update database path to "./file_sync_server4.db"
6. Add new peer configurations to all server configs
7. Update `run-multi.sh` to start the fourth server

### Logs and Debugging

- Server logs are saved in `logs/` directory
- Each server has its own log file
- Use the detailed logging to troubleshoot sync issues