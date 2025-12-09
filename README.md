# NMS Lite - Network Monitoring System

A production-grade, lightweight network monitoring system built with Go, designed for the Motadata AIOps domain.

## Features

- 🔐 Secure credential management with AES-256-GCM encryption
- 🔍 Automated device discovery via IP/CIDR scanning
- 📊 High-performance metrics collection using TimescaleDB
- 🔌 Plugin-based architecture for extensible monitoring
- ⚡ Efficient polling with worker pools and batch processing
- 🎯 RESTful API with JWT authentication

## Quick Start

### Prerequisites

- Go 1.21+
- PostgreSQL 14+ with TimescaleDB extension
- Linux/macOS (Windows support coming soon)

### Installation

1. Clone the repository:
````bash
git clone https://github.com/nmslite/nmslite.git
cd nmslite
````

2. Install dependencies:
````bash
go mod download
````

3. Set up environment variables:
````bash
cp .env.example .env
# Edit .env and set your secrets
````

4. Set up the database:
````bash
# Create database and enable TimescaleDB
createdb nms_lite
psql nms_lite -c "CREATE EXTENSION IF NOT EXISTS timescaledb;"
````

5. Run the server:
````bash
go run cmd/server/main.go
````

### Configuration

Configuration is managed through `config.yaml` with environment variable overrides.

Required environment variables:
- `NMS_AUTH_JWT_SECRET` - JWT signing key (32+ characters)
- `NMS_AUTH_ENCRYPTION_KEY` - AES-256 encryption key (exactly 32 bytes)
- `NMS_AUTH_ADMIN_PASSWORD` - Admin password

See `.env.example` for a complete list.

## Project Structure

````
nms-lite/
├── cmd/server/          # Application entry point
├── internal/            # Private application code
│   ├── config/         # Configuration management
│   ├── database/       # Database layer
│   ├── models/         # Data models
│   ├── api/            # HTTP handlers
│   ├── auth/           # Authentication & encryption
│   ├── poller/         # Polling engine
│   └── plugins/        # Plugin manager
├── plugins/            # Plugin binaries
├── config.yaml         # Configuration file
└── README.md
````

## Development Status

**Phase 1: Foundation** ✅
- [x] Project structure
- [x] Configuration loader
- [x] Main server skeleton

**Phase 2: Core Infrastructure** 🚧
- [ ] Database connection & schema
- [ ] Authentication & encryption
- [ ] Protocol registry
- [ ] API router & middleware

**Phase 3: Business Logic** 📋
- [ ] Credential profiles API
- [ ] Discovery profiles API
- [ ] Monitor provisioning API

**Phase 4: Polling System** 📋
- [ ] Plugin manager
- [ ] Polling scheduler
- [ ] Metrics writer

## API Documentation

Base URL: `/api/v1`

See [TECHNICAL_TERMS_AND_SCHEMAS.md](TECHNICAL_TERMS_AND_SCHEMAS.md) for complete API documentation.

## License

MIT License - See LICENSE file for details

## Contributing

Contributions are welcome! Please read CONTRIBUTING.md for guidelines.
