# File Sync - Quick Start Guide

## 🚀 Fast Start

### 1. Build the project
```bash
go build
```

### 2. Test with multiple servers
```bash
make run-multi
```

### 3. Or use Makefile commands
```bash
make build    # Build project
make run      # Run single server
make clean    # Clean artifacts
make help     # Show all commands
```

## 📁 Project Structure

```
├── main.go              # Main application
├── config/              # Configuration handling
├── db/                  # SQLite database layer
├── p2p/                 # Peer-to-peer networking
├── sync/                # File synchronization logic
├── server/              # Web server and API
├── web/                 # Web interface (HTML/CSS/JS)
├── examples/            # Example configurations and scripts
├── config.yaml          # Default configuration
├── Makefile             # Build automation
└── README.md            # Full documentation
```

## 🔧 Examples Directory

The `examples/` directory contains ready-to-use configurations:

```bash
cd examples
./run-multi.sh  # Start 2 servers for testing
```

### Available Examples:
- `config-server1.yaml` - Server 1 configuration
- `config-server2.yaml` - Server 2 configuration
- `run-multi.sh` - Multi-server launcher script
- `README.md` - Examples documentation

## 🌐 Web Interface

After starting servers, access:
- **Server 1**: http://localhost:8081
- **Server 2**: http://localhost:8083

Features:
- 📁 File browser
- 📊 Sync status dashboard
- 📝 File history viewer
- ⬇️ File download

## 🔍 Troubleshooting

### Check logs
```bash
# Server logs
tail -f examples/logs/server1.log
tail -f examples/logs/server2.log
```

### Common issues
1. **Port conflicts**: Make sure ports 8080-8083 are free
2. **Permissions**: Ensure write access to sync directories
3. **Dependencies**: Run `go mod tidy` if build fails

### Detailed logging
The application provides detailed logs with prefixes:
- `[MAIN]` - Startup information
- `[P2P]` - Network communications
- `[SYNC]` - File synchronization
- `[SERVER]` - Web server operations

## 📚 Full Documentation

See `README.md` for complete documentation including:
- Architecture overview
- API reference
- Configuration options
- Development guide