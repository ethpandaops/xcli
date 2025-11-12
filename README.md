# xcli

Local development orchestration tool for the ethPandaOps lab stack.

## Overview

`xcli` orchestrates the complete local development environment for the lab stack, including:

- **ClickHouse clusters** (CBT and Xatu)
- **CBT transformation engines** (per network)
- **cbt-api** REST API servers (per network)
- **lab-backend** API gateway and frontend server
- **lab** React frontend (Vite dev server)
- **Redis** for caching and task queues
- **Zookeeper** for ClickHouse cluster coordination

## Features

- **Full local isolation**: Complete end-to-end development environment
- **Hybrid mode**: Use production/external data with local transformations
- **Multi-network support**: Mainnet, Sepolia, and more
- **Persistent data**: Named volumes survive restarts
- **Easy service management**: Start, stop, restart individual services
- **Auto-configuration**: Discovers repositories and generates configs
- **Health monitoring**: Check service status and health

## Prerequisites

- **Docker** and **Docker Compose** v2+
- **Go** 1.23+ (for building)
- All five repositories checked out side-by-side:
  ```
  ethpandaops/
  ├── cbt/
  ├── xatu-cbt/
  ├── cbt-api/
  ├── lab-backend/
  ├── lab/
  └── xcli/  (this repo)
  ```

## Installation

### From source

```bash
# Clone the repository
git clone https://github.com/ethpandaops/xcli
cd xcli

# Build and install
make install

# Or just build locally
make build
./bin/xcli --help
```

### Using go install

```bash
go install github.com/ethpandaops/xcli/cmd/xcli@latest
```

## Quick Start

### 1. Initialize

```bash
cd xcli
xcli init
```

This will:
- Discover all required repositories in the parent directory
- Validate repository structure
- Create `.xcli.yaml` configuration file

### 2. Review Configuration

Edit `.xcli.yaml` to customize:
- Networks to enable/disable
- Operating mode (local vs hybrid)
- Port assignments
- External data source connection (for hybrid mode)

### 3. Start the Stack

```bash
xcli up
```

This will:
- Generate docker-compose configuration
- Generate service configs (CBT, cbt-api, lab-backend)
- Generate ClickHouse cluster configs
- Start all services in dependency order
- Wait for services to become healthy

### 4. Access Services

Once started, you can access:

- **Lab Frontend**: http://localhost:5173
- **Lab Backend**: http://localhost:8080
- **cbt-api (mainnet)**: http://localhost:8091
- **cbt-api (sepolia)**: http://localhost:8092
- **CBT Engine (mainnet)**: http://localhost:8081
- **CBT Engine (sepolia)**: http://localhost:8082
- **ClickHouse CBT**: http://localhost:8123
- **ClickHouse Xatu**: http://localhost:8125 (if local mode)

### 5. Monitor Status

```bash
# View service status
xcli ps

# Check health of all services
xcli status

# View logs
xcli logs                    # All services
xcli logs cbt-api-mainnet    # Specific service
xcli logs -f lab-backend     # Follow logs
```

### 6. Development Workflow

```bash
# Restart a service after making changes
xcli restart cbt-api-mainnet

# Rebuild and restart lab frontend
cd ../lab
pnpm build
cd ../xcli
xcli restart lab-backend

# Regenerate cbt-api protos
cd ../cbt-api
make proto
cd ../xcli
xcli restart cbt-api-mainnet
```

### 7. Stop and Clean

```bash
# Stop services (keeps data)
xcli down

# Stop and remove all data
xcli clean
```

## Operating Modes

### Local Mode (Default)

All services run locally:
- Local Xatu ClickHouse cluster (with sample/test data)
- Local CBT ClickHouse cluster
- All transformations and APIs

**Use case**: Fully isolated development, testing migrations and transformations

### Hybrid Mode Setup

Hybrid mode runs the entire lab stack locally EXCEPT the Xatu ClickHouse cluster, which connects to an external datasource (production or staging).

#### Use Cases
- Testing CBT transformations with production data
- Debugging issues with real-world datasets
- Developing locally without ingesting full beacon chain data

#### Configuration

1. Set mode to hybrid in `.xcli.yaml`:
   ```yaml
   mode: hybrid
   ```

2. Configure external Xatu datasource:
   ```yaml
   infrastructure:
     clickhouse:
       xatu:
         mode: external
         external_url: "https://readonly:password@prod-xatu.example.com:8443"
         external_database: "default"
         external_username: "readonly"
         external_password: "supersecret"
   ```

3. Obtain credentials:
   - Production: Contact DevOps for read-only credentials
   - Staging: Check team documentation

4. Start the stack:
   ```bash
   xcli up
   ```

#### What Runs Locally vs. Externally

| Component | Local | External |
|-----------|-------|----------|
| CBT ClickHouse Cluster | ✓ | |
| Xatu ClickHouse Cluster | | ✓ |
| Zookeeper Ensemble | ✓ | |
| Redis | ✓ | |
| CBT Engine | ✓ | |
| CBT API | ✓ | |
| Lab Backend | ✓ | |
| Lab Frontend | ✓ | |

#### Troubleshooting

**Error: "external_url is required for hybrid mode"**
- Add `external_url` field to `.xcli.yaml` under `infrastructure.clickhouse.xatu`

**Connection timeout to external Xatu**
- Verify credentials are correct
- Check network connectivity: `curl https://prod-xatu.example.com:8443`
- Confirm firewall allows outbound HTTPS on port 8443

**Services still starting local Xatu cluster**
- Verify `xatu.mode` is set to `external`
- Check logs: `xcli logs | grep "xatu-clickhouse"`
- Ensure xcli and xatu-cbt are up to date

## Network Management

### Adding a Network

Edit `.xcli.yaml`:

```yaml
networks:
  - name: mainnet
    enabled: true
    port_offset: 0
  - name: sepolia
    enabled: true
    port_offset: 1
  - name: holesky
    enabled: true
    port_offset: 2  # CBT: 8083, cbt-api: 8093
```

Then restart:

```bash
xcli down && xcli up
```

### Disabling a Network

Set `enabled: false` for any network, then restart.

## Commands

### Core Commands

- `xcli init` - Initialize configuration
- `xcli up` - Start all services
- `xcli down` - Stop services (preserve data)
- `xcli clean` - Stop services and remove volumes
- `xcli ps` - Show service status
- `xcli logs [service]` - View logs
- `xcli restart <service>` - Restart a service
- `xcli status` - Check service health
- `xcli mode <local|hybrid>` - Switch operating mode

### Configuration Commands

- `xcli config show` - Display current configuration
- `xcli config validate` - Validate configuration

### Flags

- `-c, --config <path>` - Path to config file (default: `.xcli.yaml`)
- `-l, --log-level <level>` - Log level: debug, info, warn, error (default: info)
- `-h, --help` - Show help
- `-v, --version` - Show version

### Examples

```bash
# Start in hybrid mode
xcli up --mode=hybrid

# Start without detaching (see all logs)
xcli up --detach=false

# Use custom config
xcli --config=.xcli.custom.yaml up

# Debug mode
xcli --log-level=debug up

# View logs for all cbt-api instances
xcli logs | grep cbt-api
```

## Architecture

### Data Flow

```
┌─────────────────────────────────────────────────────────┐
│ External Data (Xatu Cluster) - Hybrid Mode             │
│ OR Local Xatu Cluster - Local Mode                     │
└────────────────┬────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────┐
│ CBT Engines (per network)                              │
│ - Read external models                                  │
│ - Run transformations (fct_*, dim_*)                   │
│ - Write to CBT cluster                                 │
└────────────────┬────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────┐
│ CBT ClickHouse Cluster                                  │
│ - Transformation tables (fct_*, dim_*)                 │
│ - Admin tables (cbt_incremental, cbt_scheduled)        │
└────────────────┬────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────┐
│ cbt-api (per network)                                   │
│ - Discovers tables from ClickHouse                      │
│ - Exposes REST endpoints with filtering/pagination     │
└────────────────┬────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────┐
│ lab-backend                                             │
│ - Routes /api/v1/{network}/* to cbt-api instances      │
│ - Serves lab frontend                                   │
│ - Network and feature configuration                     │
└────────────────┬────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────┐
│ lab (Frontend)                                          │
│ - Network selection                                     │
│ - Data visualization                                    │
│ - API integration                                       │
└─────────────────────────────────────────────────────────┘
```

### Service Dependencies

Services start in this order:

1. **Infrastructure**: Zookeeper, ClickHouse clusters, Redis
2. **CBT Engines**: Transform data (depends on ClickHouse + Redis)
3. **cbt-api**: Expose REST APIs (depends on CBT ClickHouse)
4. **lab-backend**: API gateway (depends on cbt-api + Redis)
5. **lab**: Frontend dev server (independent)

## Troubleshooting

### Services won't start

```bash
# Check Docker is running
docker ps

# Check service logs
xcli logs

# Validate configuration
xcli config validate

# Try a clean restart
xcli clean
xcli up
```

### Port conflicts

Edit `.xcli.yaml` to change port assignments:

```yaml
ports:
  lab_backend: 9080  # Changed from 8080
  lab_frontend: 5174  # Changed from 5173
```

### Can't find repositories

Ensure all repos are checked out in the same parent directory:

```bash
# Should show all 5 repos
ls ../
# Expected: cbt, xatu-cbt, cbt-api, lab-backend, lab

# Re-run discovery
xcli init
```

### ClickHouse won't start

```bash
# Check if ports are available
lsof -i :8123  # Should be empty

# Check Docker resources
docker system df

# Clean up old volumes
xcli clean
```

### Hybrid mode connection fails

Verify external ClickHouse connection:

```bash
# Test connection (from external URL in config)
curl -u username:password https://external-clickhouse:8443/ping

# Check CBT engine logs
xcli logs cbt-engine-mainnet
```

## Development

### Building from Source

```bash
# Install dependencies
make deps

# Build
make build

# Run tests
make test

# Lint
make lint

# Install locally
make install
```

### Project Structure

```
xcli/
├── cmd/xcli/          # CLI entry point
├── pkg/
│   ├── commands/         # CLI commands
│   ├── config/           # Configuration management
│   ├── compose/          # Docker compose generation
│   │   └── templates/    # Compose and config templates
│   ├── discovery/        # Repository discovery
│   └── orchestrator/     # Service orchestration
├── .xcli.example.yaml # Example configuration
├── Makefile             # Build automation
└── README.md            # This file
```

### Contributing

1. Follow ethPandaOps Go standards
2. Add tests for new features
3. Update README for user-facing changes
4. Use conventional commit messages

## Configuration Reference

See [`.xcli.example.yaml`](.xcli.example.yaml) for a fully documented configuration example.

### Key Configuration Options

- **repos**: Paths to all required repositories
- **mode**: `local` or `hybrid` operating mode
- **networks**: List of networks to enable with port offsets
- **infrastructure.clickhouse**: ClickHouse cluster configuration
- **infrastructure.redis**: Redis configuration
- **infrastructure.volumes.persist**: Whether to persist data between restarts
- **ports**: Port assignments for all services
- **dev**: Development-specific features

## License

See [LICENSE](LICENSE) file.

## Support

- Issues: https://github.com/ethpandaops/xcli/issues
- Docs: https://docs.ethpandaops.io

## Related Projects

- [cbt](https://github.com/ethpandaops/cbt) - ClickHouse transformation tool
- [xatu-cbt](https://github.com/ethpandaops/xatu-cbt) - CBT models for Xatu
- [cbt-api](https://github.com/ethpandaops/cbt-api) - REST API generator
- [lab-backend](https://github.com/ethpandaops/lab-backend) - API gateway
- [lab](https://github.com/ethpandaops/lab) - Frontend application
