# Configuration Examples

This directory contains example YAML configuration files demonstrating various features of the Process Manager.

## Quick Start

```bash
# Start daemon with a config file
./target/release/daemon --config examples/simple-webserver.yaml

# Or use environment variable
export PM_CONFIG_PATH=examples/simple-webserver.yaml
./target/release/daemon
```

## Example Files

### 📄 `simple-webserver.yaml`
**Features:** Basic process configuration, restart policy, output redirection

Simple web server that restarts automatically. Perfect starting point for beginners.

**Use case:** Long-running web services

### 📄 `restart-policies.yaml`
**Features:** All restart policies (never, always, on-failure, on-success)

Demonstrates different restart behaviors and when to use each.

**Use case:** Understanding restart policies and exponential backoff

### 📄 `process-types.yaml`
**Features:** Process types (simple, oneshot, forking)

Shows how different process types behave:
- `simple`: Long-running services
- `oneshot`: Setup tasks, migrations
- `forking`: Traditional daemons

**Use case:** Different service types

### 📄 `dependencies.yaml`
**Features:** Dependencies (after, before, requires, wants)

Complete dependency example with database → cache → API → frontend chain.

**Use case:** Multi-service applications with startup dependencies

### 📄 `user-group.yaml`
**Features:** User/group switching

Run processes as different users for security isolation.

**Use case:** Production deployments with least privilege

**Note:** Requires daemon to run as root

### 📄 `pre-post-commands.yaml`
**Features:** Pre/post execution hooks

Setup, cleanup, and notification hooks around process lifecycle.

**Use case:** Migrations, health checks, monitoring integration

### 📄 `environment.yaml`
**Features:** Environment variables, working directory

Configure environment for your applications.

**Use case:** 12-factor apps, configuration management

### 📄 `environment-file.yaml`
**Features:** EnvironmentFile= and PIDFile= (systemd-compatible)

Demonstrates systemd-compatible features:
- `environment_file`: Load env vars from file (KEY=VALUE format)
- `pidfile`: Write/track PID files for external monitoring
- Optional env files (prefix with `-` to ignore if missing)
- Automatic PID file cleanup

**Use case:** Production deployments, systemd migration, external monitoring

### 📄 `mock-datadog.yaml`
**Features:** Real-world Datadog Agent simulation

Complete multi-service setup demonstrating:
- Hard dependencies (`requires`)
- Soft dependencies (`wants`)
- Start ordering (`after`)
- PID file management
- Environment files
- Output inheritance

**Use case:** Testing complex service dependencies, systemd migration testing

### 📄 `full-example.yaml`
**Features:** Comprehensive example with all features

Complete real-world example showcasing:
- Restart policies
- Start limits
- Output redirection
- Environment variables
- Pre/post commands
- User/group switching
- Success exit status
- Kill signals
- Timeouts

**Use case:** Production-ready configuration reference

### 📄 `update-example.yaml`
**Features:** Dynamic configuration updates with hot-update and restart-required fields

Demonstrates the `dd-procmgr update` command with:
- Hot-updatable fields (restart policy, resource limits, timeouts)
- Restart-required fields (environment, user, working directory)
- Resource scaling examples
- Health check configuration
- Development vs production patterns
- Includes usage examples for common update scenarios

**Use case:** Dynamic configuration management, resource scaling, zero-downtime updates

## Configuration Reference

### Basic Structure

```yaml
processes:
  process-name:
    # Required
    command: /path/to/executable

    # Optional
    args: ["arg1", "arg2"]
    working_dir: /path/to/workdir
    auto_start: false  # Start immediately when daemon loads config

    # Restart policy
    restart: never|always|on-failure|on-success
    restart_sec: 1                # Base delay between restarts
    restart_max_delay: 60         # Max delay cap (exponential backoff)

    # Start limits (prevent restart loops)
    start_limit_burst: 5          # Max restarts
    start_limit_interval: 300     # Within this time window (seconds)

    # Output redirection
    stdout: /path/to/stdout.log   # Or "inherit" or "null"
    stderr: /path/to/stderr.log   # Or "inherit" or "null"

    # Timeouts
    timeout_start_sec: 90         # Max time to wait for start
    timeout_stop_sec: 90          # Max time for graceful stop

    # Kill signal
    kill_signal: SIGTERM|SIGINT|SIGKILL|SIGQUIT|SIGHUP|SIGUSR1|SIGUSR2

    # Exit status
    success_exit_status: [0, 1]   # Which exit codes are "success"

    # Execution hooks
    exec_start_pre: ["cmd1", "cmd2"]    # Before start
    exec_start_post: ["cmd1", "cmd2"]   # After start
    exec_stop_post: ["cmd1", "cmd2"]    # After stop

    # User/Group (requires root daemon)
    user: username
    group: groupname

    # Dependencies
    after: [process1, process2]         # Start order
    before: [process3]                  # Reverse ordering
    requires: [process1]                # Hard dependency
    wants: [process2]                   # Soft dependency

    # Process type
    process_type: simple|oneshot|forking|notify

    # Environment
    env:
      KEY1: value1
      KEY2: value2
    
    # Environment file (systemd EnvironmentFile=)
    environment_file: /path/to/env/file  # Or prefix with '-' to ignore if missing
    
    # PID file (systemd PIDFile=)
    pidfile: /var/run/process.pid  # Automatically cleaned up on stop
```

## Common Patterns

### Web Application Stack

```yaml
processes:
  database:
    command: postgres
    restart: always

  cache:
    command: redis-server
    after: [database]
    restart: always

  api:
    command: /usr/bin/api-server
    after: [database, cache]
    requires: [database, cache]
    restart: always

  frontend:
    command: nginx
    after: [api]
    requires: [api]
    restart: always
```

### Background Worker

```yaml
processes:
  worker:
    command: python3
    args: ["worker.py"]
    restart: on-failure
    restart_sec: 5
    start_limit_burst: 10
    env:
      QUEUE_URL: redis://localhost
      WORKER_THREADS: "4"
```

### Database Migration (Oneshot)

```yaml
processes:
  migrate:
    command: /usr/bin/migrate
    args: ["up"]
    process_type: oneshot
    success_exit_status: [0]
```

### Cron-like Periodic Job

```yaml
processes:
  cleanup:
    command: /usr/bin/cleanup-script
    restart: on-success
    restart_sec: 3600  # Run every hour
    process_type: oneshot
```

## Tips

1. **Start Simple**: Begin with `simple-webserver.yaml` and add features as needed

2. **Test Locally**: Test configs with a local daemon before deploying:
   ```bash
   ./target/release/daemon --config examples/your-config.yaml
   ```

3. **Check Logs**: Enable debug logging to troubleshoot:
   ```bash
   RUST_LOG=debug ./target/release/daemon --config your-config.yaml
   ```

4. **Dependencies**: Use both `after` and `requires` for strong dependencies:
   ```yaml
   after: [database]      # Ensures start order
   requires: [database]   # Ensures it's running
   ```

5. **Restart Policies**:
   - `never`: For one-time tasks
   - `always`: For critical services that should never be down
   - `on-failure`: For services that might exit cleanly
   - `on-success`: For periodic jobs

6. **Security**: Always use `user`/`group` in production to run with least privilege

7. **Monitoring**: Use `exec_start_post` for health checks and notifications

## Validation

Test your config with:

```bash
# Dry run - check syntax
./target/release/daemon --config examples/your-config.yaml &
PID=$!
sleep 2
./target/release/cli list
kill $PID
```

## See Also

- [Main README](../README.md) - Full documentation
- [PRIVILEGE_MANAGEMENT.md](../PRIVILEGE_MANAGEMENT.md) - User/group execution details
- [DEPENDENCY_LOGGING.md](../DEPENDENCY_LOGGING.md) - Dependency troubleshooting

