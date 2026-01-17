# Contributing to File Sync

Thank you for your interest in contributing to File Sync! This document provides guidelines and information for contributors.

## Development Setup

### Prerequisites
- Go 1.21 or later
- Git

### Getting Started
```bash
# Clone the repository
git clone <repository-url>
cd file-sync

# Install dependencies
go mod tidy

# Build the project
go build

# Run tests
go test ./...

# Run demo
make demo-test
```

## Project Structure

```
├── config/          # Configuration handling
├── db/             # SQLite database operations
├── p2p/            # Peer-to-peer networking
├── server/         # HTTP server and API
├── sync/           # File synchronization logic
├── web/            # Web interface (HTML/CSS/JS)
├── examples/       # Example configurations and scripts
├── main.go         # Application entry point
├── Makefile        # Build automation
└── README.md       # Documentation
```

## Development Guidelines

### Code Style
- Follow standard Go formatting (`go fmt`)
- Use `gofmt -s` for additional simplifications
- Run `go vet` to check for common errors
- Use `golint` for style checking

### Testing
```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific test
go test -run TestName ./package
```

### Commit Messages
Use clear, descriptive commit messages:
```
feat: add new synchronization algorithm
fix: resolve connection timeout issue
docs: update API documentation
test: add unit tests for config package
```

### Pull Requests
- Create a feature branch from `main`
- Ensure all tests pass
- Update documentation if needed
- Add tests for new functionality
- Follow the existing code style

## Adding New Features

### 1. Plan Your Changes
- Discuss major changes in an issue first
- Break down complex features into smaller PRs
- Consider backward compatibility

### 2. Implementation
- Write clean, readable code
- Add appropriate error handling
- Include unit tests
- Update documentation

### 3. Testing
- Test in demo mode first (`make demo-test`)
- Test with real servers (`make run-multi`)
- Verify in different environments

## Configuration Files

When adding new configuration options:
1. Update the config struct in `config/config.go`
2. Add YAML tags
3. Update example configurations in `examples/`
4. Document in README.md

## API Endpoints

When adding new API endpoints:
1. Add route in `server/server.go`
2. Implement handler function
3. Add appropriate middleware
4. Update API documentation
5. Add tests

## File Synchronization

When modifying sync logic:
1. Test with various file types and sizes
2. Consider network interruptions
3. Handle file conflicts appropriately
4. Update demo scripts if needed

## Reporting Issues

When reporting bugs:
- Use the issue template
- Include Go version (`go version`)
- Provide steps to reproduce
- Include relevant logs
- Specify your OS and architecture

## License

By contributing to this project, you agree that your contributions will be licensed under the same license as the project.