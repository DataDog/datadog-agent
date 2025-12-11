# Host Profiler with Datadog Agent - Docker Compose

Runs the Host Profiler in Agent-integrated mode, leveraging the Agent's infrastructure tagging and metadata enrichment.

## Prerequisites

- Docker & Docker Compose
- Linux (eBPF support, kernel 4.15+)
- Datadog API and APP keys

## Quick Start

```bash
export DD_API_KEY="your_api_key"
export DD_APP_KEY="your_app_key"

docker-compose up --build
```

## Configuration

**Environment variables:**
- `DD_SITE` - Datadog site (default: datadoghq.com)
- `DD_LOG_LEVEL` - Log level (default: DEBUG)
- `VERSION` - Version tag (default: local-dev)
- `DO_NOT_START_PROFILER` - Set to prevent auto-start for manual testing

**Host Profiler config:** `dist/host-profiler-config.yaml`

## Usage

```bash
# View logs
docker-compose logs -f host-profiler

# Stop
docker-compose down

# Development mode
export DO_NOT_START_PROFILER=1
docker-compose up -d
docker exec -it host-profiler /bin/bash
```

## Architecture

The host-profiler shares the datadog-agent's network namespace and communicates via a shared IPC volume at `/etc/datadog-agent`. The profiler runs with `pid: host` for eBPF access to host processes.

## Troubleshooting

- **No profiles appearing:** Check `docker-compose logs host-profiler` for errors
- **Permission errors:** Ensure `/sys/kernel/debug` is mounted and accessible
- **eBPF issues:** Verify kernel version with `uname -r` (needs 4.15+)
