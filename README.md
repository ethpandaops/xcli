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
xcli lab logs                    # View all logs
xcli lab logs lab-backend        # View specific service logs
xcli lab logs -f lab-backend     # Follow logs

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

### Restart a Single Service

After making code changes to a specific service:

```bash
# Example: After editing lab-backend code
xcli lab restart lab-backend

# Example: After editing CBT transformation
xcli lab restart cbt-engine-mainnet
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
#       external_url: "https://user:pass@host:8443"

# Restart services
xcli lab down && xcli lab up
```

### View Service Logs

```bash
# All services
xcli lab logs

# Specific service
xcli lab logs lab-backend
xcli lab logs cbt-engine-mainnet
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
    xatu_cbt: ../xatu-cbt
    cbt_api: ../cbt-api
    lab_backend: ../lab-backend
    lab: ../lab

  mode: local  # or "hybrid"

  networks:
    - name: mainnet
      enabled: true
      port_offset: 0
    - name: sepolia
      enabled: true
      port_offset: 1

  ports:
    lab_backend: 8080
    lab_frontend: 5173
    cbt_base: 8081      # Base port for CBT engines
    cbt_api_base: 8091  # Base port for cbt-api services

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

## Troubleshooting

### Services won't start

```bash
# Check logs
xcli lab logs

# Validate config
xcli lab config validate

# Clean restart
xcli lab down
xcli lab up
```
