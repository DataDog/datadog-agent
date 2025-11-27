# Directory-Based Configuration (Systemd-Style)

The process manager uses **directory-based configuration only**. Each process and socket is defined in its own YAML file, similar to systemd's approach.

## Directory Structure

```
/etc/pm/processes.d/
├── web.yaml              # Process definition (name: "web")
├── web.socket.yaml       # Socket definition for web
├── worker.yaml           # Another process (name: "worker")
└── api.socket.yaml       # Socket for a different service
```

## Benefits

- **Modular**: Each process and socket in its own file
- **Systemd-compatible**: Familiar structure for sysadmins
- **Safe updates**: Modifying one process doesn't risk breaking others
- **Easy to manage**: Add/remove services by adding/removing files
- **Clear ownership**: One file per unit, easy to track changes

## File Naming Conventions

### Process Files
- `<name>.yaml` or `<name>.yml`
- Example: `web.yaml`, `worker.yaml`
- **The filename becomes the process name**

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
DD_PM_CONFIG_DIR=/etc/pm/processes.d dd-procmgrd

# Or let it auto-detect (defaults to /etc/pm/processes.d if it exists)
dd-procmgrd
```

## Example Process Configuration

**web.yaml:**
```yaml
# Process name is "web" (from filename)
command: /usr/bin/python3
args: ["-m", "http.server"]
restart: always
```

**web.socket.yaml:**
```yaml
service: web
listen_stream: "0.0.0.0:8080"
```

**worker.yaml:**
```yaml
# Process name is "worker" (from filename)
command: /usr/bin/worker
restart: on-failure
```

