# xcli

Local development orchestration tool for ethPandaOps projects.

## Prerequisites

- **Docker** and **Docker Compose** v2+
- **Go** 1.23+ (for building)
- All required repositories checked out side-by-side:
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

```bash
# Build from source
make build

# Or install globally
make install
```

## Quick Start

```bash
# 1. Initialize (discovers repos, creates .xcli.yaml)
xcli lab init

# 2. Start everything
xcli lab up

# 3. Access services
# Lab Frontend:  http://localhost:5173
# Lab Backend:   http://localhost:8080
# cbt-api:       http://localhost:8091 (mainnet), 8092 (sepolia)
```

## Commands

### Lab Stack Management

```bash
xcli lab init                # Initialize lab configuration
xcli lab up                  # Start the lab stack
xcli lab down                # Stop and remove data
xcli lab ps                  # List running services
xcli lab build               # Build all repositories
```

### Service Management

```bash
# View logs
xcli lab logs                    # View all logs
xcli lab logs lab-backend        # View specific service logs
xcli lab logs -f lab-backend     # Follow logs

# Start/stop individual services
xcli lab start lab-backend       # Start a specific service
xcli lab stop lab-frontend       # Stop a specific service
xcli lab start cbt-mainnet       # Start CBT engine for mainnet
xcli lab stop cbt-api-sepolia    # Stop cbt-api for sepolia

# Restart services
xcli lab restart lab-backend     # Restart a specific service
xcli lab restart cbt-api-mainnet # Restart mainnet cbt-api
```

### Configuration

```bash
xcli lab config show             # Show lab configuration
xcli lab config validate         # Validate configuration
xcli lab mode local              # Switch to local mode
xcli lab mode hybrid             # Switch to hybrid mode

xcli config show                 # Show all stack configs
xcli config validate             # Validate all stacks
```

### Build Options

```bash
xcli lab up --rebuild            # Force rebuild all binaries
xcli lab up --no-build           # Skip build (fail if binaries missing)
xcli lab up -v                   # Verbose mode (show build output)
xcli lab build -f                # Force rebuild
```

## Common Workflows

### Start/Stop Individual Services

Manage services independently without affecting the whole stack:

```bash
# Stop a service temporarily
xcli lab stop lab-frontend

# Start it again later
xcli lab start lab-frontend

# Useful for debugging - stop resource-intensive services
xcli lab stop cbt-mainnet
xcli lab stop cbt-api-mainnet
```

### Restart a Single Service

After making code changes to a specific service:

```bash
# Example: After editing lab-backend code
xcli lab restart lab-backend

# Example: After editing CBT transformation
xcli lab restart cbt-mainnet

# Note: Restart only works if service is already running
# If service crashed, use 'start' instead
xcli lab start cbt-mainnet
```

### Regenerate Protos

When proto definitions change in the CBT schema:

```bash
# 1. Ensure infrastructure is running (needed for proto generation)
xcli lab ps

# 2. If not running, start infrastructure
xcli lab up

# 3. Rebuild (which regenerates protos)
xcli lab build -f

# 4. Restart cbt-api services
xcli lab restart cbt-api-mainnet
xcli lab restart cbt-api-sepolia
```

### Work with Multiple Networks

Enable or disable networks in `.xcli.yaml`:

```yaml
networks:
  - name: mainnet
    enabled: true
    portOffset: 0
  - name: sepolia
    enabled: true
    portOffset: 1
  - name: holesky
    enabled: true
    portOffset: 2  # CBT: 8083, cbt-api: 8093
```

Then restart:

```bash
xcli lab down
xcli lab up
```

### Switch Between Local and Hybrid Mode

**Local mode**: Everything runs locally (default)
**Hybrid mode**: Connect to external Xatu data, run transformations locally

```bash
# Switch to hybrid mode
xcli lab mode hybrid

# Edit .xcli.yaml to add external connection
# infrastructure:
#   clickhouse:
#     xatu:
#       mode: external
#       externalUrl: "https://user:pass@host:8443"
#       externalDatabase: "default"

# Restart services
xcli lab down && xcli lab up
```

### View Service Logs

```bash
# All services
xcli lab logs

# Specific service
xcli lab logs lab-backend
xcli lab logs cbt-mainnet
xcli lab logs cbt-api-sepolia

# Follow logs (live tail)
xcli lab logs -f lab-backend
```

### Check Service Status

```bash
# List running services and their status
xcli lab ps
```

### Clean Restart

```bash
# Stop and remove all data
xcli lab down

# Start fresh
xcli lab up
```

## Configuration

The `.xcli.yaml` file is created by `xcli lab init` and can be customized:

```yaml
lab:
  repos:
    cbt: ../cbt
    xatuCbt: ../xatu-cbt
    cbtApi: ../cbt-api
    labBackend: ../lab-backend
    lab: ../lab

  mode: local  # or "hybrid"

  networks:
    - name: mainnet
      enabled: true
      portOffset: 0
    - name: sepolia
      enabled: true
      portOffset: 1

  ports:
    labBackend: 8080
    labFrontend: 5173
    cbtBase: 8081      # Base port for CBT engines
    cbtApiBase: 8091  # Base port for cbt-api services

  infrastructure:
    clickhouse:
      xatu:
        mode: local  # or "external" for hybrid
      cbt:
        mode: local
    redis:
      port: 6380
    volumes:
      persist: true  # Keep data between restarts
```

See [`.xcli.example.yaml`](.xcli.example.yaml) for full options.
