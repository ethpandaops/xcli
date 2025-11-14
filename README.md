# xcli

Local development orchestration tool for ethPandaOps projects.

## Prerequisites

- **Docker** and **Docker Compose** v2+
- **Go** 1.23+ (for building Go services)
- **pnpm** (for lab frontend)
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
make build      # Build from source
make install    # Install globally (optional)
```

## Quick Start

```bash
xcli lab init   # Initialize config (creates .xcli.yaml)
xcli lab up     # Start the stack
xcli lab ps     # Check status
```

Services:
- Lab Frontend: http://localhost:5173
- Lab Backend: http://localhost:8080
- cbt-api: http://localhost:8091 (mainnet)

## Commands

### Stack Management

```bash
xcli lab init                    # Initialize configuration
xcli lab up                      # Start all services
xcli lab up --rebuild            # Force rebuild before starting
xcli lab up --no-build           # Skip build (fail if binaries missing)
xcli lab down                    # Stop and remove data
xcli lab ps                      # List running services
```

### Build & Rebuild

```bash
xcli lab build                   # Build all binaries
xcli lab build -f                # Force rebuild all
xcli lab rebuild <target>        # Rebuild specific component

# Rebuild targets:
#   xatu-cbt     - Rebuild xatu-cbt (protos + binary + configs + restart)
#   cbt          - Rebuild cbt binary
#   cbt-api      - Rebuild cbt-api (protos + binary)
#   lab-backend  - Rebuild lab-backend binary
#   all          - Rebuild everything
```

### Service Control

```bash
xcli lab start <service>         # Start a service
xcli lab stop <service>          # Stop a service
xcli lab restart <service>       # Restart a service
xcli lab logs <service>          # View logs
xcli lab logs -f <service>       # Follow logs
```

Services: `lab-backend`, `lab-frontend`, `cbt-mainnet`, `cbt-api-mainnet`, etc.

### Configuration

```bash
xcli lab config show             # Show configuration
xcli lab config validate         # Validate configuration
xcli lab mode local              # Switch to local mode
xcli lab mode hybrid             # Switch to hybrid mode
```

## Development Workflow

After making code changes to a service:

```bash
# Simple restart (if binary unchanged)
xcli lab restart lab-backend

# Rebuild and restart
xcli lab rebuild lab-backend

# Full proto regeneration workflow (for xatu-cbt changes)
xcli lab rebuild xatu-cbt
```

## Configuration

`.xcli.yaml` is created by `xcli lab init`. Key settings:

```yaml
mode: local  # "local" or "hybrid"

networks:
  - name: mainnet
    enabled: true
    portOffset: 0

infrastructure:
  clickhouse:
    xatu:
      mode: local  # "local" or "external"
  volumes:
    persist: true  # Keep data between restarts
```

**Modes:**
- **local**: All services run locally (default)
- **hybrid**: External Xatu data, local transformations

See [`.xcli.example.yaml`](.xcli.example.yaml) for all options.
