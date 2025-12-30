# Process Manager

A production-ready process manager with gRPC API, written in Rust. Features systemd-style restart policies, exponential backoff, start limits, process supervision, comprehensive logging, and YAML configuration support.

## ✨ Key Features

### Core Process Management
- 🔗 **Dependencies** - systemd-like `after`, `before`, `requires`, `wants`, `binds_to`, `conflicts` with automatic resolution
- 🔄 **Restart Policies** - `always`, `on-failure`, `on-success`, `never` (systemd-style)
- 📦 **Process Types** - `simple`, `oneshot`, `forking`, `notify` (systemd `Type=`)
- 🚀 **Auto-Start** - Create and start processes in one command
- ⏱️ **Exponential Backoff** - Intelligent restart delays with configurable limits
- 🛡️ **Start Limit Protection** - Prevents infinite restart loops
- 🎯 **Process Supervision** - Background monitoring and automatic restarts
- 📝 **Run Counter** - Track restart frequency

### Advanced Features
- 🏥 **Health Checks** - HTTP, TCP, and Exec probes (Docker/K8s-style)
  - Configurable intervals, timeouts, retries
  - `restart_after` for K8s liveness probe behavior
  - Automatic restart on consecutive failures
- 🔌 **Socket Activation** - systemd-compatible socket activation
  - TCP and Unix domain sockets
  - Accept=no mode (listening socket passed to service)
  - Socket re-activation after crashes
  - Zero-downtime on-demand startup
- 💾 **Resource Limits** - K8s-style resource management
  - CPU limits (millicores: 500m = 0.5 cores)
  - Memory limits (requests & limits)
  - PID limits (prevent fork bombs)
  - cgroups v2 + rlimit fallback
- 📊 **Resource Monitoring** - Real-time usage statistics
  - Memory usage (current/limit)
  - CPU time (user/system)
  - PID count
  - Via `dd-procmgr stats` command

### Configuration & Integration
- ⚙️ **YAML Configuration** - Declarative process management
- 📁 **Environment Files** - Load env vars from files (systemd `EnvironmentFile=`)
- 📌 **PID Files** - Write/track PIDs (systemd `PIDFile=`)
- 👤 **User/Group Switching** - Run processes as different users (privilege separation)
- ✅ **Conditional Starting** - systemd `ConditionPathExists` with AND/OR/NOT logic
- 📂 **Runtime Directories** - Auto-create `/run/` directories with proper permissions (systemd `RuntimeDirectory=`)
- 🔐 **Ambient Capabilities** - Linux capabilities for fine-grained privileges (systemd `AmbientCapabilities=`)
- 🔍 **Detailed Process Info** - `dd-procmgr describe` shows everything
- 🌐 **gRPC API** - Easy integration with other services
- 🗑️ **Safe Deletion** - Remove processes with optional force flag
- 📝 **Comprehensive Logging** - Structured logs for easy troubleshooting

## 🏗️ Architecture

This workspace contains three crates plus client examples:

- **`engine`** — Core process manager library with Rust APIs and C FFI wrappers
- **`daemon`** — gRPC server (listens on `127.0.0.1:50051`)
- **`cli`** — Command-line client for interacting with the daemon
- **`go-client`** — Go client using CGO/FFI (no network overhead!)

## 🚀 Quick Start

### Build Everything

```bash
cargo build --release
```

Binaries will be in `target/release/`:
- `daemon` - gRPC server (4.7MB optimized)
- `cli` - Command-line client (4.0MB optimized)

### Start the Daemon

```bash
# Unix socket mode (default - local connections only)
./target/release/dd-procmgrd

# Dual mode (Unix socket + TCP - for Docker/remote access)
./target/release/dd-procmgrd --tcp --grpc-port 50051

# With config directory (one file per process, process name derived from filename)
./target/release/dd-procmgrd --config-dir /etc/pm/processes.d

# Using environment variable (useful for custom paths)
DD_PM_CONFIG_DIR=/custom/path/processes.d ./target/release/dd-procmgrd

# With debug logging (recommended for development - silences gRPC noise)
RUST_LOG=debug,h2=off,tower=off ./target/release/dd-procmgrd --config-dir /etc/pm/processes.d

# Show help
./target/release/dd-procmgrd --help
```

**Connection Modes:**
- **Unix Socket** (default): `/var/run/process-manager.sock` - Fast, secure, local only
- **TCP** (`--tcp` flag): `0.0.0.0:50051` - Remote access, network-based
- **Dual Mode** (`--tcp` flag): Both Unix socket AND TCP simultaneously - Best for Docker containers

**Configuration Precedence** (first match wins):
1. `--config-dir` CLI flag
2. `DD_PM_CONFIG_DIR` environment variable
3. `/etc/pm/processes.d/` (if directory exists)
4. No config (start empty, warn)

**Log Levels:** `trace` (most verbose) > `debug` > `info` (default) > `warn` > `error` (least verbose)

### Use the CLI

```bash
# Create a process (name and command are required)
./target/release/cli create myapp python3 -m http.server 8080

# Start a process
./target/release/cli start <process-id>

# OR create and start in one command
./target/release/cli create myapp python3 -m http.server 8080 --auto-start

# List all processes
./target/release/cli list

# View detailed process information
./target/release/cli describe <process-id>

# Stop a process
./target/release/cli stop <process-id>

# Delete a process
./target/release/cli delete <process-id>

# Reload configuration
./target/release/cli reload-config
```

## 📋 CLI Commands

### `create <name> <command> [args...]`
Creates a new process entry (doesn't start it by default). Both name and command are required.

**Process names must be unique** (like Docker container names). If you try to create a process with an existing name, you'll get an error. You can delete the existing process first or choose a different name.

```bash
./target/release/cli create webserver python3 -m http.server 8080
./target/release/cli create worker ./my-worker --threads 4
./target/release/cli create database redis-server --port 6379

# Create and start immediately (new in v0.2!)
./target/release/cli create api ./server --auto-start
```

**Auto-Start:** Use `--auto-start` to create and start the process in one command. This is useful for processes that should begin immediately, like web servers in containers or systemd-style services.

Returns a UUID that you'll use to manage the process. You can reference processes by either their **name** or **UUID** in all commands (`start`, `stop`, `describe`, `delete`, etc.).

### `start <process-id>`
Starts a previously created process.

```bash
./target/release/cli start 93b3a49a-ccb7-4b0f-91e3-075839376acc
```

Can restart processes in `stopped`, `crashed`, or `exited` states.

### `stop <process-id>`
Sends SIGTERM to stop a running process.

```bash
./target/release/cli stop 93b3a49a-ccb7-4b0f-91e3-075839376acc
```

**Note:** Manually stopped processes won't auto-restart (systemd behavior).

### `delete <process-id> [--force]`
Removes a process from the manager.

```bash
# Delete a stopped process
./target/release/cli delete 93b3a49a-ccb7-4b0f-91e3-075839376acc

# Force delete a running process (stops it first)
./target/release/cli delete 93b3a49a-ccb7-4b0f-91e3-075839376acc --force
```

**Aliases:** `remove`

### `list`
Shows all processes with detailed information.

```bash
./target/release/cli list
```

Output includes:
- **NAME**: Human-readable process name
- **ID**: Process UUID
- **PID**: System process ID (or `-` if not running)
- **STATE**: Current process state
- **RUNS**: Number of times the process has been started
- **COMMAND**: Command being executed
- **CREATED**: Creation timestamp
- **STARTED**: Last start timestamp
- **ENDED**: Last end timestamp
- **EXIT**: Exit code or signal

### `describe <process-id>`
Shows comprehensive information about a single process.

```bash
./target/release/cli describe 93b3a49a-ccb7-4b0f-91e3-075839376acc
```

Displays:
- Basic information (name, ID, command, args)
- Current state and PID
- Restart policy
- Run count and lifecycle timestamps
- Exit information (code, signal)
- Configuration (working directory, environment variables)
- Health check status (if configured)
- Resource limits (if configured)

### `stats <process-id>`
Shows real-time resource usage statistics for a process.

```bash
./target/release/cli stats my-app
```

Displays:
- **Memory Usage**: Current memory usage and limit (percentage)
- **CPU Time**: User and system CPU time
- **PID Count**: Current number of processes/threads and limit
- **Resource Status**: Whether limits are being approached

Example output:
```
Resource Usage Statistics for 'datadog-agent' (PID: 1234)
─────────────────────────────────────────────────────────

Memory:
  Current: 128.5 MB
  Limit:   512 MB (25.1% used)

CPU:
  User time:   5.23s
  System time: 1.45s
  Total:       6.68s

PIDs:
  Current: 12
  Limit:   100 (12.0% used)
```

### `reload-config`
Reloads processes from the configuration file.

```bash
./target/release/cli reload-config
```

### `update <process-id> [options...]`
Update process configuration dynamically without recreating the process. Changes are intelligently categorized as **hot updates** (applied immediately) or **restart-required** (need process restart to take effect).

#### Basic Examples

```bash
# Update restart policy (hot update - no restart needed)
./target/release/cli update my-app --restart always

# Update environment variable (requires restart to take effect)
./target/release/cli update my-app --env NEW_VAR=value

# Update and auto-restart to apply changes immediately
./target/release/cli update my-app --env KEY=value --restart-process

# Update multiple fields at once
./target/release/cli update my-app \
  --restart on-failure \
  --timeout-stop-sec 30 \
  --cpu-limit 2

# Validate changes without applying (dry run)
./target/release/cli update my-app --restart always --dry-run
```

#### Hot-Update Fields (No Restart Required)

These changes are applied immediately, even to running processes:

| Option | Description | Example |
|--------|-------------|---------|
| `--restart <policy>` | Restart policy: `never`, `always`, `on-failure`, `on-success` | `--restart always` |
| `--timeout-stop-sec <seconds>` | Graceful shutdown timeout | `--timeout-stop-sec 30` |
| `--restart-sec <seconds>` | Delay between restarts | `--restart-sec 5` |
| `--restart-max <seconds>` | Maximum restart delay (exponential backoff) | `--restart-max 300` |
| `--cpu-limit <cores>` | CPU limit in cores (e.g., "2" or "0.5") | `--cpu-limit 1.5` |
| `--memory-limit <bytes>` | Memory limit (supports K, M, G suffixes) | `--memory-limit 512M` |
| `--pids-limit <count>` | Maximum number of PIDs/threads | `--pids-limit 100` |

**Hot Update Example:**
```bash
# Increase resource limits on a running production service
./target/release/cli update web-api --cpu-limit 4 --memory-limit 2G
# [OK] Process configuration updated
# Updated fields:
#   - resources
```

#### Restart-Required Fields

These changes require a process restart to take effect. Use `--restart-process` to restart automatically:

| Option | Description | Example |
|--------|-------------|---------|
| `--env KEY=VALUE` | Environment variable (can specify multiple) | `--env LOG_LEVEL=debug` |
| `--env-file <path>` | Load environment from file | `--env-file /etc/app/.env` |
| `--working-dir <path>` | Working directory | `--working-dir /app/data` |
| `--user <username>` | Run as specific user | `--user appuser` |
| `--group <groupname>` | Run as specific group | `--group appgroup` |
| `--runtime-directory <dir>` | Runtime directory under `/run/` | `--runtime-directory myapp` |
| `--ambient-capability <CAP>` | Linux capability (e.g., `CAP_NET_BIND_SERVICE`) | `--ambient-capability CAP_NET_BIND_SERVICE` |
| `--kill-mode <mode>` | How to kill child processes: `control-group`, `process-group`, `process`, `mixed` | `--kill-mode control-group` |
| `--kill-signal <signal>` | Signal to send on stop: `SIGTERM`, `SIGKILL`, etc. | `--kill-signal SIGINT` |
| `--pidfile <path>` | PID file location | `--pidfile /var/run/app.pid` |

**Restart-Required Example:**
```bash
# Update environment and restart automatically
./target/release/cli update api-server \
  --env DATABASE_URL=postgres://newhost/db \
  --env LOG_LEVEL=info \
  --restart-process

# Output:
# [OK] Process configuration updated
# Updated fields:
#   - env
# The following fields require restart to take effect:
#   - env
# ✓ Process restarted successfully
```

#### Advanced Usage

**Dry Run (Validation):**
```bash
# Test changes without applying them
./target/release/cli update my-app \
  --restart always \
  --cpu-limit 4 \
  --env NEW_VAR=value \
  --dry-run

# Output shows what would be updated:
# [OK] Dry run - validation successful (no changes applied)
# Updated fields:
#   - restart_policy
#   - resources
#   - env
# The following fields require restart to take effect:
#   - env
```

**Mixed Hot and Restart-Required Updates:**
```bash
# Update both types of fields
./target/release/cli update worker \
  --restart on-failure \        # Hot update
  --cpu-limit 2 \               # Hot update
  --env THREADS=8 \             # Restart required
  --restart-process             # Auto-restart to apply env change

# The hot updates apply immediately, then the process restarts for env change
```

**Incremental Resource Scaling:**
```bash
# Gradually increase resources for a running service
./target/release/cli update high-load-service --cpu-limit 2
# Monitor performance...
./target/release/cli update high-load-service --cpu-limit 4
# Monitor performance...
./target/release/cli update high-load-service --memory-limit 4G
```

#### Flags

- `--restart-process` - Automatically restart the process after applying changes that require restart
- `--dry-run` - Validate the update without applying any changes (useful for testing)

#### Best Practices

1. **Use Dry Run First**: Always test with `--dry-run` before applying changes to production processes
2. **Hot Updates for Running Services**: Prefer hot-update fields (restart policy, resource limits) for zero-downtime changes
3. **Batch Updates**: Update multiple fields in one command rather than multiple sequential updates
4. **Monitor After Updates**: Check process status with `dd-procmgr describe` or `dd-procmgr stats` after updates
5. **Restart-Required Changes**: Plan restarts during maintenance windows or use `--restart-process` for immediate application

#### Common Use Cases

**Adjust Restart Behavior:**
```bash
# Change from never restarting to always restarting
./target/release/cli update flaky-service --restart always

# Add exponential backoff to prevent restart storms
./target/release/cli update unstable-app \
  --restart on-failure \
  --restart-sec 5 \
  --restart-max 300
```

**Scale Resources:**
```bash
# Increase limits for a memory-intensive process
./target/release/cli update data-processor \
  --memory-limit 8G \
  --cpu-limit 4

# Add PID limit to prevent fork bombs
./target/release/cli update user-script --pids-limit 50
```

**Update Configuration:**
```bash
# Point to new config file
./target/release/cli update app \
  --env CONFIG_PATH=/etc/app/new-config.yaml \
  --restart-process

# Switch to debug logging
./target/release/cli update service \
  --env LOG_LEVEL=debug \
  --env RUST_LOG=debug \
  --restart-process
```

**Change Process Ownership:**
```bash
# Run as different user (requires restart)
./target/release/cli update web-server \
  --user www-data \
  --group www-data \
  --restart-process
```

## 🔄 Process States

- **`created`** - Process defined but not started
- **`starting`** - Process is being started
- **`running`** - Process is currently running
- **`stopping`** - Stop signal sent, waiting for exit
- **`stopped`** - Process was manually stopped
- **`crashed`** - Process exited with non-zero code or hit start limit
- **`exited`** - Process exited successfully (code 0)
- **`unknown`** - State couldn't be determined
- **`zombie`** - Process is in zombie state

## ⚙️ Restart Policies (systemd-style)

### `never` (default)
Process never restarts automatically.

### `always`
Process restarts on any exit (success or failure).

**Use case:** Long-running services that should always be available.

```yaml
processes:
  web_server:
    command: "python3"
    args: ["-m", "http.server", "8080"]
    restart: "always"
```

### `on-failure`
Process restarts only when it crashes (non-zero exit code).

**Use case:** Services that might exit cleanly but should recover from errors.

```yaml
processes:
  api_server:
    command: "./api-server"
    restart: "on-failure"
```

### `on-success`
Process restarts only when it exits successfully (zero exit code).

**Use case:** Periodic jobs that should run continuously.

```yaml
processes:
  worker:
    command: "./worker"
    restart: "on-success"
```

### Restart Behavior

- **Exponential backoff** between restarts (prevents rapid restart loops)
- **Configurable delays** with `restart_sec` and `restart_max_delay`
- **Start limit protection** prevents infinite restart loops
- **Run counter increments** on each restart
- **Manual stops don't trigger restart** - systemd-like behavior (use `stop` to actually stop)
- **Stopped processes won't restart** until manually started again

## 🛡️ Exponential Backoff & Start Limits

### Exponential Backoff

When a process keeps failing, restart delays increase exponentially to avoid hammering the system:

```yaml
processes:
  api_server:
    command: "./api-server"
    restart: "on-failure"
    restart_sec: 2              # Base delay: 2 seconds
    restart_max_delay: 60       # Cap at 60 seconds
```

**Delay progression:**
- 1st restart: 2 seconds
- 2nd restart: 4 seconds
- 3rd restart: 8 seconds
- 4th restart: 16 seconds
- 5th restart: 32 seconds
- 6th+ restart: 60 seconds (capped)

**Defaults:** `restart_sec: 1`, `restart_max_delay: 60`

### Start Limit Protection

Prevents infinite restart loops by limiting restart attempts within a time window:

```yaml
processes:
  worker:
    command: "./worker"
    restart: "always"
    start_limit_burst: 5        # Max 5 restarts
    start_limit_interval: 300   # Within 300 seconds (5 minutes)
```

**Behavior:**
- If process restarts 5 times in 5 minutes → stops trying
- Process enters `crashed` state
- Logs show "Start limit exceeded"
- Manual intervention required to restart

**Defaults:** `start_limit_burst: 5`, `start_limit_interval: 300`

## 📝 Configuration Directory

The process manager uses directory-based configuration. Each process is defined in its own YAML file within the configuration directory. The process name is derived from the filename (without the `.yaml` extension).

### Directory Structure

```
/etc/pm/processes.d/
├── web-server.yaml         # Process name: "web-server"
├── background-job.yaml     # Process name: "background-job"
└── api-service.yaml        # Process name: "api-service"
```

### Full Example (YAML)

```yaml
# /etc/pm/processes.d/web-server.yaml
# Process name derived from filename: "web-server"

command: "python3"
args: ["-m", "http.server", "8080"]
auto_start: true                    # Start on daemon startup
restart: "always"                   # Restart policy

# Restart delay with exponential backoff
restart_sec: 2                      # Wait 2 seconds before first restart
restart_max_delay: 60               # Cap exponential backoff at 60 seconds

# Start limits (prevents infinite restart loops)
start_limit_burst: 5                # Max 5 restarts
start_limit_interval: 300           # Within 300 seconds (5 minutes)

working_dir: "/var/www"             # Working directory (optional)
env:                                # Environment variables (optional)
  DEBUG: "true"
  PORT: "8080"
```

```yaml
# /etc/pm/processes.d/background-job.yaml
command: "sleep"
args: ["60"]
auto_start: false
restart: "on-failure"

# Custom restart settings
restart_sec: 5
restart_max_delay: 120

# More lenient start limits
start_limit_burst: 10
start_limit_interval: 600
```

```yaml
# /etc/pm/processes.d/api-service.yaml
command: "./my-service"
auto_start: true
restart: "always"
working_dir: "/opt/app"
env:
  LOG_LEVEL: "info"
  DATABASE_URL: "postgres://localhost/mydb"
```

### Configuration Fields

#### Required
- **`command`**: Command to execute

#### Optional
- **`args`** (default: `[]`): Array of command arguments
- **`auto_start`** (default: `false`): Start process when daemon starts
- **`restart`** (default: `"never"`): Restart policy (`never`, `always`, `on-failure`, `on-success`)
- **`restart_sec`** (default: `1`): Base delay before restart in seconds
- **`restart_max_delay`** (default: `60`): Maximum restart delay cap in seconds
- **`start_limit_burst`** (default: `5`): Maximum number of restart attempts
- **`start_limit_interval`** (default: `300`): Time window for start limit in seconds
- **`working_dir`**: Working directory for the process
- **`env`**: Environment variables as key-value pairs
- **`environment_file`**: Load environment variables from file (KEY=VALUE format, prefix with `-` to ignore if missing)
- **`pidfile`**: Write process PID to file on start, remove on stop

## 📊 Comprehensive Logging

All operations are logged with structured context for easy troubleshooting.

### Log Levels

```bash
# Info level (default) - normal operations
./target/release/dd-procmgrd --config-dir /etc/pm/processes.d

# Debug level - detailed diagnostics
RUST_LOG=debug ./target/release/dd-procmgrd --config-dir /etc/pm/processes.d

# Specific module
RUST_LOG=pm_engine=debug ./target/release/dd-procmgrd --config-dir /etc/pm/processes.d
```

### What Gets Logged

- ✅ Process creation, start, stop, deletion
- ✅ State transitions
- ✅ Restart decisions and delays
- ✅ Start limit tracking and violations
- ✅ Exponential backoff calculations
- ✅ Configuration changes
- ✅ Errors and warnings

### Example Log Output

```
INFO  pm_engine: Creating new process id=abc123 name=web_server command=python3
INFO  pm_engine: Starting process id=abc123 name=web_server run_count=1
INFO  pm_engine: Process started successfully id=abc123 pid=12345
INFO  pm_engine: Process exited id=abc123 exit_code=1 success=false
INFO  pm_engine: Calculated restart delay delay_secs=2 consecutive_failures=1
INFO  pm_engine: Automatic restart succeeded id=abc123 name=web_server
WARN  pm_engine: Start limit exceeded name=web_server starts=5 limit=5
ERROR pm_engine: Start limit exceeded - marking as crashed id=abc123
```

See **`LOGGING_GUIDE.md`** for complete documentation.

## 🔌 gRPC API

The daemon exposes a gRPC service on `127.0.0.1:50051`.

### Service Definition

```protobuf
service ProcessManager {
  rpc Create (CreateRequest) returns (CreateResponse);
  rpc Start (StartRequest) returns (StartResponse);
  rpc Stop (StopRequest) returns (StopResponse);
  rpc List (ListRequest) returns (ListResponse);
  rpc Describe (DescribeRequest) returns (DescribeResponse);
  rpc Delete (DeleteRequest) returns (DeleteResponse);
  rpc ReloadConfig (ReloadConfigRequest) returns (ReloadConfigResponse);
  rpc SetRestartPolicy (SetRestartPolicyRequest) returns (SetRestartPolicyResponse);
}
```

Proto files are in `proto/process_manager.proto`.

**Auto-Start Support:** The `CreateRequest` message includes an `auto_start` boolean field. When set to `true`, the process will be started immediately after creation. The `CreateResponse` will reflect the actual process state (`Created` or `Running`).

## 🔗 FFI Client (Go Example)

For applications that need direct library integration without network overhead, the engine provides C FFI bindings that can be called from Go, Python (ctypes), C/C++, and other languages.

### Go Client via CGO

See `go-client/` for a complete example. The Go client uses CGO to call the Rust FFI directly:

```bash
cd go-client
make build
make run
```

**Example Usage:**

```go
client := NewProcessManager()

// Traditional: Create then start
id, _ := client.Create("my-app", "/usr/bin/app", []string{"--port", "8080"})
client.Start(id)

// OR create and start in one call (auto-start)
id, _ := client.CreateWithAutoStart("my-app", "/usr/bin/app", []string{"--port", "8080"}, true)

// List all processes
processes, _ := client.List()
for _, proc := range processes {
    fmt.Printf("%s: %s (PID: %d)\n", proc.Name, proc.State, proc.PID)
}

// Stop the process
client.Stop(id)
```

**Performance Benefits:**
- 🚀 **10-100x faster** than gRPC (no serialization/network)
- 💾 **Lower memory** overhead
- 🎯 **Direct** function calls
- ⚡ **Sub-microsecond** latency for create/list operations

**Trade-offs:**
- ⚠️ Same-process only (can't manage remote processes)
- ⚠️ Language-specific bindings needed
- ⚠️ Shared library must be installed

See `go-client/README.md` for detailed documentation and build instructions.

## 🧪 Examples

### Example 1: Always-Running Web Server

```bash
# Create with restart policy and start
./target/release/cli create webserver python3 -m http.server 8080 --restart-policy always
# Copy the ID from output
./target/release/cli start <id>

# Server will keep running and restart if it crashes
# Watch it in real-time:
watch -n 1 './target/release/cli list'
```

### Example 2: Self-Healing Service with Smart Backoff

```bash
# Create a process that might fail with on-failure restart policy
./target/release/cli create myapi ./my-api-server --restart-policy on-failure
./target/release/cli start <id>

# If it crashes:
# - 1st restart: ~1 second delay
# - 2nd restart: ~2 seconds delay
# - 3rd restart: ~4 seconds delay
# - Delays keep doubling until reaching max

# Check the RUNS column to see how many times it's restarted
./target/release/cli list

# View detailed info
./target/release/cli describe <id>
```

### Example 3: Testing Start Limit Protection

```bash
# Create a process that always crashes with always restart policy
./target/release/cli create crasher bash -c "exit 1" --restart-policy always
./target/release/cli start <id>

# Watch it restart with increasing delays
# After hitting the limit (default: 5 restarts in 5 minutes):
# - State becomes "crashed"
# - Auto-restart stops
# - Logs show "Start limit exceeded"

# Manual restart is required:
./target/release/cli start <id>  # Resets the counter
```

### Example 4: Using Configuration File

```yaml
# my-services.yaml
processes:
  nginx:
    command: "nginx"
    args: ["-g", "daemon off;"]
    auto_start: true
    restart: "always"
    restart_sec: 2
    restart_max_delay: 60
    start_limit_burst: 5
    start_limit_interval: 300

  redis:
    command: "redis-server"
    auto_start: true
    restart: "always"

  worker:
    command: "./worker"
    auto_start: true
    restart: "on-failure"
    env:
      QUEUE_URL: "redis://localhost:6379"
```

```bash
# Start daemon with config
./target/release/daemon my-services.yaml

# All processes with auto_start: true will start automatically
./target/release/cli list
```

### Example 5: Process Lifecycle Management

```bash
# Create
./target/release/cli create myapp ./app --port 8080

# View details
./target/release/cli describe <id>

# Start
./target/release/cli start <id>

# Monitor
./target/release/cli list

# Stop
./target/release/cli stop <id>

# Delete (must be stopped first)
./target/release/cli delete <id>

# Or force delete while running
./target/release/cli delete <id> --force
```

## 🔧 Advanced Features

### Dependencies (systemd-like)

Control process startup order with **true systemd semantics**:

- **`requires`** - Hard dependency: **auto-starts** dependency, **fails** if it can't start
- **`wants`** - Soft dependency: **auto-starts** dependency, **continues** if it fails
- **`binds_to`** - Strong binding: **auto-starts** dependency, **stops this process** if dependency stops/crashes
- **`conflicts`** - Mutual exclusion: **stops conflicting process** before starting (systemd-style)
- **`after`** - Ordering only: does NOT auto-start, only ensures start order
- **`before`** - Reverse ordering hint (for other processes to reference)

> **💡 Key Insight**: `--requires` and `--wants` auto-start dependencies. `--after` only defines ordering!
>
> **💡 BindsTo Insight**: `--binds-to` creates a lifecycle coupling - if the target stops (manually or crash), this process is automatically stopped too. Perfect for tightly coupled services (e.g., trace agent → main agent).
>
> **💡 Conflicts Insight**: `--conflicts` implements systemd's **bidirectional** mutual exclusion - if A declares `conflicts=B`, then starting A stops B **AND** starting B stops A. Only ONE process needs to declare the conflict. If stopping a conflicting process fails, the new process won't start (strict enforcement). Perfect for stable vs. experimental variants that can't run simultaneously.

#### Via Config File

```yaml
processes:
  database:
    command: /usr/bin/postgres
    auto_start: false

  cache:
    command: /usr/bin/redis-server
    requires: [database]     # Auto-starts database (hard dependency)
    after: [database]        # Ensures database starts first

  api:
    command: /usr/bin/api-server
    requires: [database, cache]  # Auto-starts both (must succeed)
    after: [database, cache]     # Ensures proper start order

  frontend:
    command: /usr/bin/nginx
    wants: [api]             # Auto-starts api (soft - continues if fails)
    after: [api]             # Ensures api starts first if it's going to

  # BindsTo example - tightly coupled services
  main-agent:
    command: /opt/agent/bin/agent run
    auto_start: true

  trace-agent:
    command: /opt/agent/bin/trace-agent
    binds_to: [main-agent]   # If main-agent stops/crashes, this stops too
    after: [main-agent]      # Ensures main-agent starts first

  # Conflicts example - mutually exclusive variants
  # Only ONE needs to declare the conflict (systemd's bidirectional behavior)
  datadog-agent:
    command: /opt/datadog/bin/agent run
    auto_start: true

  datadog-agent-exp:
    command: /opt/datadog/bin/agent-exp run
    conflicts: [datadog-agent]  # Only exp declares conflict
    auto_start: false
    # Bidirectional: Starting exp stops stable, starting stable ALSO stops exp
```

#### Via CLI

```bash
# Create processes
cli create database /usr/bin/postgres
cli create cache /usr/bin/redis-server --requires database --after database
cli create api /usr/bin/api-server --requires database --requires cache --after database --after cache

# When you start 'api', it will:
# 1. See 'requires' directives → auto-start 'database' and 'cache'
# 2. See 'after' directives → ensure proper start order
# 3. Start 'api' after dependencies are running
# 4. Fail if any required dependency can't start
cli start api

# BindsTo example (systemd-like strong binding)
# If main-agent stops/crashes, trace-agent will be automatically stopped too
cli create main-agent /opt/agent/bin/agent run
cli create trace-agent /opt/agent/bin/trace-agent --binds-to main-agent --after main-agent

# Start main agent → auto-starts trace-agent (due to binds-to)
cli start main-agent
cli start trace-agent

# Stop main agent → trace-agent automatically stops too
cli stop main-agent  # trace-agent will cascade-stop

# Conflicts example (systemd-like mutual exclusion - BIDIRECTIONAL)
# Only ONE process needs to declare the conflict (systemd behavior)
cli create datadog-agent /opt/datadog/bin/agent run
cli create datadog-agent-exp /opt/datadog/bin/agent-exp run --conflicts datadog-agent

# Start stable agent
cli start datadog-agent

# Start experimental → automatically stops stable first, then starts exp
cli start datadog-agent-exp  # Stops datadog-agent, starts datadog-agent-exp

# Start stable again → automatically stops exp first, then starts stable (BIDIRECTIONAL!)
cli start datadog-agent  # Stops datadog-agent-exp, starts datadog-agent

# Note: Conflicts is BIDIRECTIONAL even if only ONE declares it
# This matches systemd's behavior: "starting A stops B AND starting B stops A"

# Strict enforcement: If stopping the conflicting process fails, the new process won't start
# This ensures the mutual exclusion contract is never violated
```

#### Behavior Details (True Systemd Semantics)

| Directive  | Auto-Start? | Fails if can't start? | Stops Conflicts? | Cascade Stop? | Purpose |
|------------|-------------|----------------------|------------------|---------------|---------|
| `requires` | ✅ Yes      | ✅ Yes (hard fail)    | ❌ No            | ❌ No         | Must have this dependency |
| `wants`    | ✅ Yes      | ❌ No (continues)     | ❌ No            | ❌ No         | Nice to have this dependency |
| `binds_to` | ✅ Yes      | ✅ Yes (hard fail)    | ❌ No            | ✅ Yes        | Tightly coupled - lifecycle bound |
| `conflicts`| ❌ No       | ❌ No                 | ✅ Yes           | ❌ No         | Mutual exclusion - stops conflicting processes |
| `after`    | ❌ No       | ❌ No                 | ❌ No            | ❌ No         | Just defines start order |
| `before`   | ❌ No       | ❌ No                 | ❌ No            | ❌ No         | Reverse ordering hint |

**Common Patterns:**

```bash
# Pattern 1: Hard dependency with ordering (most common)
cli create app ... --requires dep --after dep

# Pattern 2: Soft dependency with ordering
cli create app ... --wants dep --after dep

# Pattern 3: Hard dependency without ordering (rare)
cli create app ... --requires dep
# Starts dep but doesn't guarantee order

# Pattern 4: Just ordering, no dependency (rare)
cli create app ... --after dep
# Only enforces order if both are being started
```

**What changed from our previous implementation:**
- ❌ OLD: `--after` auto-started, `--requires` only validated
- ✅ NEW: `--requires` auto-starts (systemd way), `--after` only orders

**Circular Dependencies**: Automatically detected and rejected with an error.

The engine now uses **true systemd semantics** with topological sorting for correct startup order!

### Process Types (`Type=`)

Control how the service is started and when it's considered "ready", following systemd's `Type=` directive:

| Type | Behavior | Use Case |
|------|----------|----------|
| `simple` | Ready immediately after fork (default) | Long-running services |
| `oneshot` | Waits for completion, doesn't spawn monitor | Setup tasks, migrations |
| `forking` | Waits for parent to exit (traditional daemons) | Legacy daemons that fork |
| `notify` | Waits for sd_notify (future) | Services with explicit readiness signals |

**Via Config File:**
```yaml
processes:
  # Simple type - default behavior
  web-server:
    command: /usr/bin/nginx
    args: ["-g", "daemon off;"]
    process_type: simple

  # Oneshot - runs once and exits
  db-migration:
    command: /usr/bin/migrate
    args: ["up"]
    process_type: oneshot

  # Forking - traditional daemon
  legacy-daemon:
    command: /usr/sbin/some-daemon
    process_type: forking
```

**Behavior:**
- **simple**: Process starts → immediately marked as "running" → monitor thread tracks it
- **oneshot**: Process starts → waits for completion → marks as "exited" or "crashed" → no monitor
- **forking**: Process starts → waits for parent to exit → marks as "running" (without PID tracking)
- **notify**: Currently falls back to simple (sd_notify support planned)

**Exit States:**
- `simple`: Ends in "exited" (success) or "crashed" (failure) when process exits
- `oneshot`: Synchronous - ends in "exited" (exit code 0) or "crashed" (non-zero) immediately
- `forking`: Cannot track exit (no PID after fork), stays "running"

### Run Counter
Tracks how many times each process has been started. Useful for monitoring restart frequency and detecting problems.

### PID Tracking
Stores and displays the system PID for running processes. Enables proper signal-based stopping even when the child handle is moved to a background thread.

### Process Supervision
Background threads monitor process exit and handle automatic restarts based on policy. Each process has its own dedicated monitor thread.

### State Transitions
Proper state machine ensures processes move through states correctly:
- `Created` → `Starting` → `Running` → `Exited/Crashed/Stopped`
- Stopped processes can be restarted: `Stopped` → `Starting` → `Running`
- Failed restarts: `Crashed` (if start limit exceeded)

### Graceful Shutdown
Uses SIGTERM for clean process shutdown on Unix systems. The stopping state ensures proper state tracking during shutdown.

### Consecutive Failure Tracking
Tracks consecutive failures for exponential backoff. Resets to 0 on successful start.

### 🔌 Socket Activation

systemd-compatible socket activation for zero-downtime on-demand service startup.

**Key Features:**
- Create listening sockets before service starts
- Service inherits socket file descriptors
- On-demand startup on first connection
- Socket re-activation after crashes
- TCP and Unix domain socket support

**Configuration:**
```yaml
processes:
  web-server:
    command: /usr/bin/web-server
    auto_start: false  # Socket activation starts it
    restart: never     # Socket handles restarts

sockets:
  web:
    listen_stream: "0.0.0.0:8080"  # TCP socket
    service: web-server
    accept: false  # Pass listening socket to service
```

**How it Works:**
1. Process Manager creates listening socket
2. Socket waits for connections
3. On first connection, service starts automatically
4. Service inherits socket via `LISTEN_FDS` environment variable
5. If service crashes, socket remains open
6. Next connection triggers automatic restart

**Environment Variables:**
- `LISTEN_FDS` - Number of inherited sockets
- `LISTEN_PID` - PID to receive sockets
- `LISTEN_FDNAMES` - Socket names (colon-separated)

**Example: DataDog Trace Agent**
```yaml
processes:
  trace-agent:
    command: /opt/datadog-agent/embedded/bin/trace-agent
    auto_start: false
    restart: never  # Socket re-activation handles this
    env:
      DD_APM_ENABLED: "true"

sockets:
  apm-tcp:
    listen_stream: "0.0.0.0:8126"
    service: trace-agent
    accept: false
```

See **[SOCKET_ACTIVATION.md](SOCKET_ACTIVATION.md)** for complete documentation and **[docker/QUICKSTART.md](docker/QUICKSTART.md)** for live examples.

### 💾 Resource Limits

Kubernetes-style resource management with cgroups v2 support.

**CPU Limits** (millicores format):
```yaml
processes:
  my-app:
    command: /usr/bin/my-app
    resources:
      requests:
        cpu: 500m      # 0.5 CPU cores baseline
      limits:
        cpu: 2000m     # 2 CPU cores maximum
```

**Memory Limits:**
```yaml
processes:
  my-app:
    resources:
      requests:
        memory: 128M   # Soft limit (throttling)
      limits:
        memory: 512M   # Hard limit (OOM kill)
```

**PID Limits** (prevent fork bombs):
```yaml
processes:
  my-app:
    resources:
      limits:
        pids: 100      # Max 100 processes/threads
```

**How it Works:**
- Uses Linux cgroups v2 for enforcement
- Falls back to rlimit on systems without cgroups
- `requests` sets soft limits (throttling)
- `limits` sets hard limits (enforcement)

**Monitoring:**
```bash
# Check resource usage
dd-procmgr stats my-app

# Output shows:
# - Current memory vs limit
# - CPU time (user + system)
# - PID count vs limit
```

**Implementation:**
- CPU: Uses `cpu.max` cgroup controller (CFS quota)
- Memory: Uses `memory.max` (hard) and `memory.high` (soft)
- PIDs: Uses `pids.max` cgroup controller

See **[RESOURCE_LIMITS.md](RESOURCE_LIMITS.md)** for complete documentation with examples and troubleshooting.

## 🛠️ Development

### Build for Development

```bash
cargo build
# or
make build-debug
```

### Run Tests

Comprehensive test suite with 32+ tests covering all features:

```bash
# Run all tests
make test

# Run specific suites
make test-engine    # Engine integration tests (fast)
make test-e2e       # E2E tests with daemon + CLI
make test-quick     # Quick smoke test

# Full CI pipeline
make ci             # format + clippy + all tests
```

**Test Coverage:**
- ✅ 17 engine integration tests (restart policies, dependencies, process types)
- ✅ 15 e2e tests (CLI + daemon integration)
- ✅ All major features covered
- ✅ CI/CD ready with GitHub Actions

See **[TESTING.md](TESTING.md)** and **[TEST_GUIDE.md](TEST_GUIDE.md)** for details.

```bash
cargo test
```

### Build with Optimizations

```bash
cargo build --release
```

### Build C FFI Library

The engine can be built as a shared library for use from other languages:

```bash
cd engine
cargo build --release
# Shared library: target/release/libpm_engine.so (Linux)
```

C FFI functions available:
- `pm_create(name, command, args, args_len)`
- `pm_start(id)`
- `pm_stop(id)`
- `pm_list()`
- `pm_free_string(ptr)`

## 📊 Monitoring

### Real-time Monitoring

Use `watch` to continuously monitor processes:

```bash
watch -n 1 './target/release/cli list'
```

### Logging

Enable debug logging to see all internal operations:

```bash
RUST_LOG=debug ./target/release/dd-procmgrd --config-dir /etc/pm/processes.d
```

### Metrics to Watch

- **RUNS column**: High values indicate frequent restarts (potential problem)
- **STATE**: Processes stuck in `crashed` likely hit start limit
- **EXIT codes**: Non-zero codes indicate failures

### Integration

Use the gRPC API to integrate with your monitoring systems:
- Prometheus exporters
- Grafana dashboards
- Alert managers
- Log aggregation (ELK, Loki)

## 🐛 Troubleshooting

### Process won't start
- Check if start limit was exceeded: Look for `crashed` state
- View detailed info: `./target/release/cli describe <id>`
- Check logs: `RUST_LOG=debug ./target/release/dd-procmgrd --config-dir /etc/pm/processes.d`
- Ensure command and name are valid (both required)

### Process won't stop
- Check if the PID is valid: `./target/release/cli list`
- Processes must be in `running` or `starting` state to be stopped
- Check logs for "SIGTERM sent successfully"
- Use `--force` flag if needed

### Process keeps restarting
- Check the restart policy: it might be set to `always` or `on-failure`
- Stop the process: `./target/release/cli stop <id>`
- If needed, delete and recreate with different restart policy
- Monitor restarts: `./target/release/cli describe <id>` shows run count

### Process hit start limit
```
ERROR pm_engine: Start limit exceeded - marking as crashed
```
- Process restarted too many times (default: 5 in 5 minutes)
- Fix the underlying issue causing crashes
- Manually restart to reset counter: `./target/release/cli start <id>`
- Or increase limits in config:
  ```yaml
  start_limit_burst: 10
  start_limit_interval: 600
  ```

### Exponential backoff too slow
- Adjust `restart_sec` (base delay)
- Adjust `restart_max_delay` (cap)
- Check consecutive failures: `./target/release/cli describe <id>`

### Config file not loading
- Check the directory path and that it exists
- Verify YAML syntax in each file
- Check daemon logs for parsing errors
- Ensure you're passing the config directory: `./target/release/dd-procmgrd --config-dir /etc/pm/processes.d`
- Verify `command` field is present (required) in each process file

### Can't delete process
- Processes must be stopped first (not in `running` or `starting` state)
- Use `--force` to stop and delete in one command
- Check logs for detailed error messages

### ~~Child processes not cleaned up (orphans)~~ FIXED
**Previous Limitation (Now Fixed):** When you stop a process that spawns children, only the parent was killed.

**Solution:** The process manager now properly kills child processes using cgroups and process groups!

```bash
# Example: Script spawns background workers
dd-procmgr create workers ./spawn_workers.sh --auto-start
dd-procmgr stop workers
# [OK] spawn_workers.sh AND all child workers are killed!
```

**Configuration:** Control how child processes are terminated with `--kill-mode`:
```bash
# Default: Kill entire cgroup (all children)
dd-procmgr create workers ./spawn_workers.sh --kill-mode control-group --auto-start

# Options:
#   control-group: Kill all processes in cgroup (default, most reliable)
#   process-group: Kill all processes in process group
#   process: Kill only main process (orphans children)
#   mixed: SIGTERM to main, then SIGKILL to group/cgroup
```

See [USE_CASES.md](USE_CASES.md#-known-limitations) for details.

### Conditional Process Starting

**Systemd-like ConditionPathExists:** Start processes only if specific files/directories exist.

```bash
# Start only if config file exists
dd-procmgr create app ./myapp --condition-path-exists /etc/app/config.yaml

# Multiple conditions (AND logic - all must exist)
dd-procmgr create app ./myapp \
  --condition-path-exists /etc/app/config.yaml \
  --condition-path-exists /var/lib/app/data

# NOT logic - path must NOT exist (negation)
dd-procmgr create app ./myapp --condition-path-exists '!/etc/app/override.conf'

# OR logic - at least one path must exist
dd-procmgr create app ./myapp \
  --condition-path-exists '|/etc/app/config.yaml' \
  --condition-path-exists '|/etc/app/managed/config.yaml'

# Mixed logic example (systemd-style)
dd-procmgr create otel-collector ./otel-agent \
  --condition-path-exists /opt/otel/bin/otel-agent \
  --condition-path-exists '!/etc/otel/disabled' \
  --condition-path-exists '|/etc/otel/config.yaml' \
  --condition-path-exists '|/etc/otel/managed/config.yaml'
```

**Condition Logic:**
- **No prefix** or **empty**: Path must exist (AND logic - all must be true)
- **`!` prefix**: Path must NOT exist (negation with AND logic)
- **`|` prefix**: Path must exist (OR logic - at least one must be true)

### Runtime Directories

**Systemd-like RuntimeDirectory:** Automatically create directories under `/run/` on start, remove on stop.

```bash
# Create /run/datadog with proper permissions (0755)
dd-procmgr create datadog-agent /opt/datadog/bin/agent \
  --runtime-directory datadog \
  --user dd-agent \
  --group dd-agent

# Multiple runtime directories
dd-procmgr create myapp ./myapp \
  --runtime-directory myapp \
  --runtime-directory myapp/cache \
  --runtime-directory myapp/tmp

# Nested paths work too
dd-procmgr create service ./service --runtime-directory myservice/data/temp

# Note: Like systemd, RuntimeDirectory only accepts relative paths
# Absolute paths (e.g., /tmp/mydir) will be rejected at creation time
```

**Features:**
- **Automatic Creation**: Directories created under `/run/` when process starts
- **Proper Permissions**: Created with `0755` permissions (rwxr-xr-x)
- **Ownership**: Set to match `--user` and `--group` if specified
- **Automatic Cleanup**: Directories removed when process stops (including crashes)
- **Restart-Safe**: Directories recreated on each restart

**Via Config File:**

```yaml
processes:
  datadog-agent:
    command: /opt/datadog/bin/agent
    user: dd-agent
    group: dd-agent
    runtime_directory:
      - datadog        # Creates /run/datadog
      - datadog/logs   # Creates /run/datadog/logs
    auto_start: true
```

**Use Cases:**
- **PID Files**: Store runtime PID files in `/run/myapp/`
- **Unix Sockets**: Create sockets in `/run/myapp/socket`
- **Temporary State**: Runtime caches, locks, or temporary data
- **Systemd Compatibility**: Drop-in replacement for `RuntimeDirectory=` directive

Conditions are checked every time you start a process. If conditions fail, the process won't start and an error is logged.

**Use Cases:**
- **Multi-location configs**: Start if config exists in primary OR fallback location
- **Feature gates**: Skip starting if a "disabled" file exists
- **Optional components**: Only start if required binary/plugin is installed
- **Deployment safety**: Verify required files before starting

See [SYSTEMD_COVERAGE.md](SYSTEMD_COVERAGE.md) for complete systemd compatibility analysis.

## 📚 Additional Documentation

### Core Documentation
- **[ARCHITECTURE.md](ARCHITECTURE.md)** - System design, components, and design decisions
- **[CONTRIBUTING.md](CONTRIBUTING.md)** - How to contribute (development setup, testing, PR process)
- **[CHANGELOG.md](CHANGELOG.md)** - Version history and release notes
- **[TESTING_CHECKLIST.md](TESTING_CHECKLIST.md)** - Comprehensive testing guide

### Feature Guides
- **[UPDATE_GUIDE.md](UPDATE_GUIDE.md)** - Dynamic configuration updates (hot updates, restart-required fields, best practices)
- **[SOCKET_ACTIVATION.md](SOCKET_ACTIVATION.md)** - Socket activation deep dive (Accept modes, re-activation)
- **[RESOURCE_LIMITS.md](RESOURCE_LIMITS.md)** - CPU, memory, PID limits (cgroups v2, K8s-style)
- **[HEALTH_CHECKS.md](HEALTH_CHECKS.md)** - Health monitoring (HTTP, TCP, Exec probes)
- **[PRIVILEGE_MANAGEMENT.md](PRIVILEGE_MANAGEMENT.md)** - User/group execution guide

### Development & Debugging
- **[DEBUG_GUIDE.md](DEBUG_GUIDE.md)** - Debugging tips, log levels, common issues
- **[LOGGING_GUIDE.md](LOGGING_GUIDE.md)** - Logging configuration
- **[QUICKSTART_MACOS.md](QUICKSTART_MACOS.md)** - macOS users Docker setup

### Docker Integration
- **[docker/README.md](docker/README.md)** - Docker deployment with DataDog Agent
- **[docker/QUICKSTART.md](docker/QUICKSTART.md)** - Quick start guide for Docker

### Examples & API
- **[examples/](examples/)** - 10+ YAML configuration examples
- **[systemd/](systemd/)** - systemd service unit files and deployment
- **[proto/process_manager.proto](proto/process_manager.proto)** - gRPC API definition
- **[go-client/](go-client/)** - Go FFI client example

## 🚦 Production Deployment

### Recommended Settings

```yaml
processes:
  my_service:
    command: "./my-service"
    auto_start: true
    restart: "always"

    # Production-ready restart settings
    restart_sec: 3              # 3 second base delay
    restart_max_delay: 300      # Cap at 5 minutes

    # Generous but protective limits
    start_limit_burst: 10       # Allow 10 restarts
    start_limit_interval: 600   # Within 10 minutes

    working_dir: "/opt/app"
    env:
      LOG_LEVEL: "info"
```

### Monitoring

1. Enable structured logging
2. Forward logs to aggregation system
3. Set up alerts on:
   - `ERROR` level messages
   - Start limit exceeded events
   - High consecutive failure counts
   - Processes in `crashed` state

### Security

- Run daemon as non-root user when possible
- Use working directories to isolate processes
- Set appropriate environment variables
- Validate all configuration inputs

## 📜 License

MIT / Apache-2.0 (dual-licensed)

## 🤝 Contributing

Contributions are welcome! Please see **[CONTRIBUTING.md](CONTRIBUTING.md)** for:
- Development setup
- Code style guidelines
- Testing requirements
- Pull request process
- Areas where help is needed

**Quick Start:**
```bash
git clone https://github.com/DataDog/process-manager.git
cd process-manager
cargo test --workspace
```

See **[ARCHITECTURE.md](ARCHITECTURE.md)** to understand the system design.

---

**Built in Rust** | Production-ready process management with systemd compatibility
