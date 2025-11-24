# Directory-Based Configuration (Systemd-Style)

This example demonstrates how to use directory-based configuration for processes and sockets, similar to systemd's approach.

## Directory Structure

```
/etc/pm/processes.d/
├── web.yaml              # Process definition
├── web.socket.yaml       # Socket definition for web
├── worker.yaml           # Another process
└── api.socket.yaml       # Socket for a different service
```

## Benefits

- **Modular**: Each process and socket in its own file
- **Systemd-compatible**: Familiar structure for sysadmins
- **Easy to manage**: Add/remove services by adding/removing files
- **Clear ownership**: One file per unit, easy to track changes

## File Naming Conventions

### Process Files
- `<name>.yaml` or `<name>.yml`
- Example: `web.yaml`, `worker.yaml`
- The filename becomes the process name (unless overridden in YAML)

### Socket Files
- `<name>.socket.yaml` or `<name>.socket.yml`
- Example: `web.socket.yaml`, `api.socket.yaml`
- The filename (without `.socket.yaml`) becomes the socket name

## Example Files

See the example files in this directory:
- `web.yaml` - A simple web server process
- `web.socket.yaml` - Socket activation for the web server
- `worker.yaml` - A background worker process

## Usage

```bash
# Start daemon with directory-based config
dd-procmgrd --config-dir /etc/pm/processes.d

# Or set as default
export DD_PM_CONFIG_PATH=/etc/pm/processes.d
dd-procmgrd
```

## Comparison with Single-File Config

### Single File (`/etc/pm/processes.yaml`)
```yaml
processes:
  web:
    command: /usr/bin/python3
    args: ["-m", "http.server"]
  worker:
    command: /usr/bin/worker

sockets:
  web-socket:
    service: web
    listen_stream: "0.0.0.0:8080"
```

### Directory-Based (`/etc/pm/processes.d/`)

**web.yaml:**
```yaml
command: /usr/bin/python3
args: ["-m", "http.server"]
```

**web.socket.yaml:**
```yaml
service: web
listen_stream: "0.0.0.0:8080"
```

**worker.yaml:**
```yaml
command: /usr/bin/worker
```

## When to Use Directory vs. Single File

### Use Directory-Based When:
- Managing many services (5+)
- Services are managed by different teams/repos
- You want systemd-style organization
- Services are added/removed frequently

### Use Single-File When:
- Small number of services (< 5)
- All services are tightly coupled
- You want to see everything in one place
- Simpler deployment (single file to manage)

