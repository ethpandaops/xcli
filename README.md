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

## Shell Completion

Enable tab completion for commands, services, and arguments:

```bash
# Bash
source <(xcli completion bash)

# To load completions for each session (Linux):
xcli completion bash > /etc/bash_completion.d/xcli

# To load completions for each session (macOS):
xcli completion bash > $(brew --prefix)/etc/bash_completion.d/xcli

# Zsh
echo "autoload -U compinit; compinit" >> ~/.zshrc  # Enable completion (once)
xcli completion zsh > "${fpath[1]}/_xcli"

# Fish
xcli completion fish | source

# To load completions for each session:
xcli completion fish > ~/.config/fish/completions/xcli.fish
```

## Quick Start

```bash
xcli lab init     # Initialize config (creates .xcli.yaml)
xcli lab check    # Verify environment is ready
xcli lab up       # Start the stack
xcli cc           # Launch web dashboard (Command Center)
xcli lab status   # Check status (CLI)
```

Services:

- Lab Frontend: <http://localhost:5173>
- Lab Backend: <http://localhost:8080>
- cbt-api: <http://localhost:8091> (mainnet)

## Command Center (Web Dashboard)

Launch a web-based dashboard for monitoring and controlling the lab stack:

```bash
xcli cc           # Opens http://localhost:19280 in your browser
xcli cc -p 8888   # Custom port
xcli cc --no-open # Don't auto-open browser
```

Features:

- Real-time service status and health monitoring via SSE
- Live log streaming from all services
- Stack controls: boot, stop, restart the entire stack
- Per-service controls: start, stop, restart, rebuild
- Lab config editor with save and restart
- Service config viewer with per-file override support
- CBT model overrides editor
- Infrastructure and observability status
- Git status for all repositories
- Service links that open directly to the right URL (CBT API opens `/docs`, ClickHouse opens `/play`)

The Command Center runs as a single binary with an embedded SPA frontend. The stack can be booted, stopped, and fully managed without leaving the browser.

### Developing the CC Frontend

The frontend lives in `pkg/cc/frontend/` (React + Vite + Tailwind). For development with hot module replacement:

```bash
make cc-dev
```

This runs the Go backend (`xcli cc --no-open`) and Vite dev server in parallel. Open `http://localhost:5173` for live reloading — the Vite dev server proxies API requests to the Go backend.

To rebuild the embedded frontend assets (required before `make build`):

```bash
make cc-frontend
```

## Interactive TUI Dashboard

Launch a terminal-based dashboard (alternative to the web UI):

```bash
xcli lab tui
```

Features:

- Real-time service status with health indicators
- Live log streaming from all services
- Interactive controls (start/stop/restart with single key)
- Infrastructure monitoring
- Vim-style keyboard navigation

Keyboard shortcuts:

- `↑/↓` or `j/k`: Navigate services
- `s`: Start service
- `t`: Stop service
- `r`: Restart service
- `Tab`: Switch panels
- `PgUp/PgDown`: Scroll logs
- `q`: Quit

**Note**: Requires interactive terminal. For non-interactive use (CI/scripts), use `xcli lab status` instead.

## Commands

### Stack Management

```bash
xcli lab init                    # Initialize configuration
xcli lab check                   # Verify environment (repos, Docker, config)
xcli lab up                      # Start all services (always rebuilds)
xcli lab down                    # Stop and remove containers/volumes
xcli lab clean                   # Remove all containers, volumes, and build artifacts
xcli lab status                  # Show service status
```

### Build & Rebuild

```bash
# Build (CI/CD, pre-building without starting services)
xcli lab build                   # Build all binaries

# Rebuild (development - rebuilds and auto-restarts services)
xcli lab rebuild <target>        # Rebuild specific component + restart

# Rebuild targets:
#   xatu-cbt      - Full model update (protos → cbt-api → configs → restart → frontend)
#   cbt           - Rebuild CBT binary + restart services
#   cbt-api       - Regenerate protos + rebuild + restart
#   lab-backend   - Rebuild + restart
#   lab-frontend  - Regenerate API types + restart
#   all           - Rebuild everything + restart all

# Use 'xcli lab build --help' or 'xcli lab rebuild --help' for detailed differences
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
# Lab-specific config
xcli lab config show             # Show lab configuration
xcli lab config validate         # Validate lab configuration
xcli lab config regenerate       # Regenerate service configs

# Global config (all stacks)
xcli config show                 # Show all stack configurations
xcli config show --stack=lab     # Show only lab stack
xcli config validate             # Validate all stacks
xcli config validate --stack=lab # Validate only lab stack

# Mode switching
xcli lab mode local              # Switch to local mode (all local services)
xcli lab mode hybrid             # Switch to hybrid mode (external Xatu ClickHouse)
```

## Development Workflow

### After making code changes

```bash
# Simple restart (if binary unchanged)
xcli lab restart lab-backend

# Rebuild and restart (most common)
xcli lab rebuild lab-backend

# Full model update workflow (when you modify xatu-cbt models)
xcli lab rebuild xatu-cbt
```

### Troubleshooting

```bash
# Verify environment is ready
xcli lab check

# View service status
xcli lab status

# View logs
xcli lab logs lab-backend -f

# Complete cleanup (removes all containers, volumes, build artifacts)
xcli lab clean
```

### Getting Help

All commands have detailed help text:

```bash
xcli lab --help              # List all lab commands
xcli lab build --help        # Detailed build command help
xcli lab rebuild --help      # See all rebuild targets and workflows
xcli lab mode --help         # Understand local vs hybrid modes
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

- **local**: Complete local development environment
  - All services (ClickHouse, Redis) run locally in Docker
  - No external dependencies required
  - Best for: Isolated development, testing, demos

- **hybrid**: Mixed local processing with external production data
  - External Xatu ClickHouse (production data source)
  - Local CBT ClickHouse (local processing and storage)
  - Best for: Testing against production data, debugging live issues

See [`.xcli.example.yaml`](.xcli.example.yaml) for all options.

Run `xcli lab mode --help` for detailed mode descriptions.

### CBT Overrides

Create `.cbt-overrides.yaml` to customize CBT configuration. This file is deep-merged on top of xcli-generated defaults, so you can override any CBT setting.

```yaml
models:
  env:
    EXTERNAL_MODEL_MIN_BLOCK: "0"
  overrides:
    fct_block:
      enabled: false
```

See [`.cbt-overrides.example.yaml`](.cbt-overrides.example.yaml) for more examples.

## xatu-cbt Test Data

Generate seed data parquet files for xatu-cbt tests:

```bash
# Interactive mode
xcli lab xatu-cbt generate-seed-data

# Scripted mode
xcli lab xatu-cbt generate-seed-data \
  --model consensus_engine_api_new_payload \
  --network mainnet \
  --spec fusaka \
  --range-column slot \
  --from 1000000 \
  --to 1001000 \
  --filter "status = VALID" \
  --upload
```

Requires hybrid mode (`xcli lab mode hybrid`) for external ClickHouse access.

### S3 Upload

Set R2/S3 credentials for upload:

```bash
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"
```

Defaults to ethpandaops R2 bucket. Override with `S3_ENDPOINT` and `S3_BUCKET` env vars.
