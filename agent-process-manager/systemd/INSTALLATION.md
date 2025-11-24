# systemd Service Installation Guide

This guide explains how to install and configure the Process Manager as a systemd service for different deployment scenarios.

## Table of Contents
- [Quick Start](#quick-start)
- [Service Variants](#service-variants)
- [System Service (Root)](#system-service-root)
- [System Service (Capabilities)](#system-service-capabilities)
- [User Service](#user-service)
- [Hardened Service](#hardened-service)
- [Configuration](#configuration)
- [Management Commands](#management-commands)
- [Troubleshooting](#troubleshooting)

---

## Quick Start

### For Production (Standard)
```bash
# 1. Install binary
sudo cp target/release/daemon /usr/bin/dd-procmgrd
sudo cp target/release/cli /usr/bin/dd-procmgr-cli
sudo chmod 755 /usr/bin/dd-procmgrd /usr/bin/dd-procmgr-cli

# 2. Create directories
sudo mkdir -p /etc/dd-procmgrd /var/lib/dd-procmgrd /var/log/dd-procmgrd

# 3. Install service
sudo cp systemd/dd-procmgrd.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable dd-procmgrd
sudo systemctl start dd-procmgrd

# 4. Verify
sudo systemctl status dd-procmgrd
pm-cli list
```

### For Development (User Service)
```bash
# 1. Install binary locally
mkdir -p ~/.local/bin
cp target/release/daemon ~/.local/bin/dd-procmgrd
cp target/release/cli ~/.local/bin/pm-cli

# 2. Create directories
mkdir -p ~/.config/dd-procmgrd ~/.local/share/dd-procmgrd

# 3. Install user service
mkdir -p ~/.config/systemd/user/
cp systemd/dd-procmgrd-user.service ~/.config/systemd/user/dd-procmgrd.service
systemctl --user daemon-reload
systemctl --user enable dd-procmgrd
systemctl --user start dd-procmgrd

# 4. Verify
systemctl --user status dd-procmgrd
pm-cli list
```

---

## Service Variants

We provide four systemd service files for different use cases:

| File | Use Case | User | Privileges |
|------|----------|------|------------|
| `dd-procmgrd.service` | **Production (Standard)** | root | Full root |
| `dd-procmgrd-capability.service` | **Production (Enhanced Security)** | dd-procmgrd | CAP_SETUID/CAP_SETGID |
| `dd-procmgrd-user.service` | **Development/Single-User** | current user | None |
| `dd-procmgrd-secure.service` | **High Security Production** | root | Hardened root |

---

## System Service (Root)

**Use this for:** Standard production deployments where you need to run processes as different users.

### Installation

```bash
# 1. Build release binary
cargo build --release

# 2. Install binaries
sudo install -m 755 target/release/daemon /usr/bin/dd-procmgrd
sudo install -m 755 target/release/cli /usr/bin/dd-procmgr-cli

# 3. Create system user and directories
sudo mkdir -p /etc/dd-procmgrd
sudo mkdir -p /var/lib/dd-procmgrd
sudo mkdir -p /var/log/dd-procmgrd
sudo mkdir -p /run/dd-procmgrd

# 4. Set permissions
sudo chmod 755 /var/lib/dd-procmgrd
sudo chmod 755 /var/log/dd-procmgrd
sudo chmod 700 /etc/dd-procmgrd

# 5. Install configuration
sudo cp processes.yaml /etc/dd-procmgrd/processes.yaml
sudo chmod 644 /etc/dd-procmgrd/processes.yaml

# 6. Install service
sudo cp systemd/dd-procmgrd.service /etc/systemd/system/
sudo systemctl daemon-reload

# 7. Enable and start
sudo systemctl enable dd-procmgrd
sudo systemctl start dd-procmgrd
```

### Verify Installation

```bash
# Check service status
sudo systemctl status dd-procmgrd

# Check logs
sudo journalctl -u dd-procmgrd -f

# Test CLI
pm-cli list
pm-cli create test sleep 10 --auto-start
pm-cli list
```

### Configuration

Edit `/etc/dd-procmgrd/processes.yaml`:

```yaml
processes:
  webapp:
    command: gunicorn app:app
    user: www-data
    group: www-data
    auto_start: true
    restart: always
    working_dir: /var/www/app
    stdout: /var/log/dd-procmgrd/webapp.log
    stderr: /var/log/dd-procmgrd/webapp-error.log
```

Reload configuration:
```bash
sudo systemctl reload dd-procmgrd
# or
pm-cli reload-config
```

---

## System Service (Capabilities)

**Use this for:** Enhanced security production deployments where daemon doesn't need full root access.

### Prerequisites

1. **Linux with capabilities support** (most modern Linux distributions)
2. **Filesystem with extended attributes** (ext4, xfs, btrfs)

### Installation

```bash
# 1. Install binaries
sudo install -m 755 target/release/daemon /usr/bin/dd-procmgrd
sudo install -m 755 target/release/cli /usr/bin/dd-procmgr-cli

# 2. Create service user
sudo useradd -r -s /bin/false -d /var/lib/dd-procmgrd dd-procmgrd

# 3. Create directories
sudo mkdir -p /etc/dd-procmgrd
sudo mkdir -p /var/lib/dd-procmgrd
sudo mkdir -p /var/log/dd-procmgrd
sudo mkdir -p /run/dd-procmgrd

# 4. Set ownership
sudo chown dd-procmgrd:dd-procmgrd /var/lib/dd-procmgrd
sudo chown dd-procmgrd:dd-procmgrd /var/log/dd-procmgrd
sudo chown dd-procmgrd:dd-procmgrd /run/dd-procmgrd
sudo chown root:dd-procmgrd /etc/dd-procmgrd
sudo chmod 750 /etc/dd-procmgrd

# 5. Set capabilities on binary
sudo setcap cap_setuid,cap_setgid+ep /usr/bin/dd-procmgrd

# 6. Verify capabilities
getcap /usr/bin/dd-procmgrd
# Should show: /usr/bin/dd-procmgrd = cap_setgid,cap_setuid+ep

# 7. Protect binary
sudo chown root:root /usr/bin/dd-procmgrd
sudo chmod 755 /usr/bin/dd-procmgrd

# 8. Install configuration
sudo cp processes.yaml /etc/dd-procmgrd/processes.yaml
sudo chown root:dd-procmgrd /etc/dd-procmgrd/processes.yaml
sudo chmod 640 /etc/dd-procmgrd/processes.yaml

# 9. Install service
sudo cp systemd/dd-procmgrd-capability.service /etc/systemd/system/dd-procmgrd.service
sudo systemctl daemon-reload

# 10. Enable and start
sudo systemctl enable dd-procmgrd
sudo systemctl start dd-procmgrd
```

### Verify Capabilities

```bash
# Check service is running as dd-procmgrd user
ps aux | grep dd-procmgrd

# Check capabilities are working
pm-cli create test-cap bash -c 'whoami' --user nobody --stdout /tmp/test-cap.log --auto-start
sleep 2
cat /tmp/test-cap.log
# Should show: nobody
```

### Removing Capabilities

If you need to remove capabilities:
```bash
sudo systemctl stop dd-procmgrd
sudo setcap -r /usr/bin/dd-procmgrd
getcap /usr/bin/dd-procmgrd  # Should show nothing
```

---

## User Service

**Use this for:** Development, testing, or single-user environments.

### Installation

```bash
# 1. Install binaries locally
mkdir -p ~/.local/bin
cp target/release/daemon ~/.local/bin/dd-procmgrd
cp target/release/cli ~/.local/bin/pm-cli
chmod 755 ~/.local/bin/dd-procmgrd ~/.local/bin/pm-cli

# Add to PATH if needed
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc

# 2. Create directories
mkdir -p ~/.config/dd-procmgrd
mkdir -p ~/.local/share/dd-procmgrd
mkdir -p ~/.local/share/dd-procmgrd/logs

# 3. Install configuration
cp processes.yaml ~/.config/dd-procmgrd/processes.yaml

# 4. Install user service
mkdir -p ~/.config/systemd/user/
cp systemd/dd-procmgrd-user.service ~/.config/systemd/user/dd-procmgrd.service

# 5. Reload systemd user instance
systemctl --user daemon-reload

# 6. Enable and start
systemctl --user enable dd-procmgrd
systemctl --user start dd-procmgrd
```

### Enable Lingering (Optional)

By default, user services stop when you log out. To keep them running:

```bash
# Enable lingering for your user
sudo loginctl enable-linger $USER

# Check lingering status
loginctl show-user $USER | grep Linger
```

### Management

```bash
# Status
systemctl --user status dd-procmgrd

# Logs
journalctl --user -u dd-procmgrd -f

# Stop/start
systemctl --user stop dd-procmgrd
systemctl --user start dd-procmgrd

# Disable
systemctl --user disable dd-procmgrd
```

---

## Hardened Service

**Use this for:** High-security production deployments with maximum systemd hardening.

### Installation

Same as [System Service (Root)](#system-service-root), but use `dd-procmgrd-secure.service`:

```bash
sudo cp systemd/dd-procmgrd-secure.service /etc/systemd/system/dd-procmgrd.service
sudo systemctl daemon-reload
sudo systemctl enable dd-procmgrd
sudo systemctl start dd-procmgrd
```

### Security Analysis

Check the security settings applied:

```bash
# Show security analysis
systemd-analyze security dd-procmgrd

# This will show:
# - Overall exposure level
# - Security features enabled/disabled
# - Recommendations for improvement
```

### Adjusting Security Settings

If you need to relax some restrictions:

```bash
# 1. Edit the service file
sudo systemctl edit dd-procmgrd

# 2. Add overrides in the editor:
[Service]
PrivateDevices=no           # If managed processes need device access
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX AF_NETLINK
# etc.

# 3. Reload and restart
sudo systemctl daemon-reload
sudo systemctl restart dd-procmgrd
```

---

## Configuration

### Environment Variables

Set environment variables in the service file:

```ini
[Service]
Environment="PM_CONFIG_PATH=/etc/dd-procmgrd/processes.yaml"
Environment="RUST_LOG=info"
Environment="PM_BIND_ADDRESS=127.0.0.1:50051"
```

Or use an environment file:

```ini
[Service]
EnvironmentFile=/etc/dd-procmgrd/daemon.env
```

Create `/etc/dd-procmgrd/daemon.env`:
```bash
PM_CONFIG_PATH=/etc/dd-procmgrd/processes.yaml
RUST_LOG=info
PM_BIND_ADDRESS=127.0.0.1:50051
```

### Process Configuration

Edit `/etc/dd-procmgrd/processes.yaml` to define processes that should start automatically:

```yaml
processes:
  # Web application
  webapp:
    command: /usr/bin/gunicorn
    args:
      - "app:app"
      - "-w"
      - "4"
      - "-b"
      - "0.0.0.0:8000"
    user: www-data
    group: www-data
    working_dir: /var/www/app
    auto_start: true
    restart: always
    restart_sec: 5
    stdout: /var/log/dd-procmgrd/webapp-access.log
    stderr: /var/log/dd-procmgrd/webapp-error.log
    env:
      DJANGO_SETTINGS_MODULE: "app.settings.production"
      DATABASE_URL: "postgresql://localhost/myapp"

  # Background worker
  worker:
    command: /usr/bin/celery
    args:
      - "-A"
      - "app"
      - "worker"
    user: www-data
    group: www-data
    working_dir: /var/www/app
    auto_start: true
    restart: always
    restart_sec: 10
    stdout: /var/log/dd-procmgrd/worker.log
    stderr: /var/log/dd-procmgrd/worker-error.log
```

Reload configuration:
```bash
sudo systemctl reload dd-procmgrd
```

---

## Management Commands

### Systemctl Commands

```bash
# Start service
sudo systemctl start dd-procmgrd

# Stop service
sudo systemctl stop dd-procmgrd

# Restart service
sudo systemctl restart dd-procmgrd

# Reload configuration (without restart)
sudo systemctl reload dd-procmgrd

# Enable at boot
sudo systemctl enable dd-procmgrd

# Disable at boot
sudo systemctl disable dd-procmgrd

# Check status
sudo systemctl status dd-procmgrd

# Is service active?
sudo systemctl is-active dd-procmgrd

# Is service enabled?
sudo systemctl is-enabled dd-procmgrd
```

### Logs

```bash
# View logs (all)
sudo journalctl -u dd-procmgrd

# Follow logs (tail -f)
sudo journalctl -u dd-procmgrd -f

# Logs since boot
sudo journalctl -u dd-procmgrd -b

# Logs for last hour
sudo journalctl -u dd-procmgrd --since "1 hour ago"

# Logs with priority (errors only)
sudo journalctl -u dd-procmgrd -p err

# Export logs
sudo journalctl -u dd-procmgrd > /tmp/dd-procmgrd.log
```

### Process Manager CLI

```bash
# List all processes
pm-cli list

# Create a process
pm-cli create myapp /usr/bin/python app.py --user www-data --auto-start

# Start/stop processes
pm-cli start myapp
pm-cli stop myapp

# Describe a process
pm-cli describe myapp

# Delete a process
pm-cli delete myapp

# Reload config from file
pm-cli reload-config
```

---

## Troubleshooting

### Service Won't Start

```bash
# Check detailed status
sudo systemctl status dd-procmgrd -l

# Check logs
sudo journalctl -u dd-procmgrd -n 50

# Validate service file
systemd-analyze verify /etc/systemd/system/dd-procmgrd.service

# Check binary exists and is executable
ls -l /usr/bin/dd-procmgrd
file /usr/bin/dd-procmgrd

# Try running manually
sudo /usr/bin/dd-procmgrd
```

### Permission Denied Errors

```bash
# Check file permissions
ls -l /usr/bin/dd-procmgrd
ls -ld /var/lib/dd-procmgrd /var/log/dd-procmgrd /etc/dd-procmgrd

# Check capabilities (if using capability-based service)
getcap /usr/bin/dd-procmgrd

# Check SELinux (if enabled)
sudo setenforce 0  # Temporarily disable
# If this fixes it, you need to configure SELinux policy

# Check AppArmor (if enabled)
sudo aa-status
sudo aa-complain /usr/bin/dd-procmgrd  # Put in complain mode for testing
```

### Capabilities Not Working

```bash
# Verify filesystem supports xattrs
mount | grep $(df /usr/bin/dd-procmgrd | tail -1 | awk '{print $1}')
# Look for "xattr" or "user_xattr"

# Try setting capabilities again
sudo setcap -r /usr/bin/dd-procmgrd
sudo setcap cap_setuid,cap_setgid+ep /usr/bin/dd-procmgrd
getcap /usr/bin/dd-procmgrd

# Check ambient capabilities in service
systemctl show dd-procmgrd -p AmbientCapabilities
```

### Port Already in Use

```bash
# Check what's using port 50051
sudo ss -tlnp | grep 50051
sudo lsof -i :50051

# Kill old process if needed
sudo pkill -9 -f dd-procmgrd

# Change port in service file
sudo systemctl edit dd-procmgrd
# Add: Environment="PM_BIND_ADDRESS=127.0.0.1:50052"
```

### High Memory/CPU Usage

```bash
# Check current resource usage
systemctl status dd-procmgrd

# Set resource limits in service file
sudo systemctl edit dd-procmgrd
# Add:
# [Service]
# MemoryMax=1G
# CPUQuota=100%

sudo systemctl daemon-reload
sudo systemctl restart dd-procmgrd
```

### Cannot Connect to Daemon

```bash
# Check if daemon is listening
sudo ss -tlnp | grep 50051

# Check firewall (should allow localhost)
sudo iptables -L -n | grep 50051

# Test connection
telnet 127.0.0.1 50051

# Check if daemon is bound to correct address
sudo journalctl -u dd-procmgrd | grep "Starting Process Manager"
# Should show: addr=127.0.0.1:50051
```

---

## Security Checklist

Before deploying to production:

- [ ] Binary installed in system location (`/usr/bin/`)
- [ ] Binary owned by root (`chown root:root`)
- [ ] Binary not writable by non-root (`chmod 755`)
- [ ] Configuration files readable only by daemon user (`chmod 640`)
- [ ] Service running as dedicated user (if using capabilities)
- [ ] Capabilities set correctly (if used)
- [ ] Filesystem supports xattrs (if using capabilities)
- [ ] Daemon binds to 127.0.0.1 only (not 0.0.0.0)
- [ ] SELinux/AppArmor policies configured (if used)
- [ ] Logs being written to appropriate location
- [ ] Resource limits set appropriately
- [ ] Restart policies configured
- [ ] Processes dropping privileges appropriately

---

## Advanced Configuration

### Log Rotation

Create `/etc/logrotate.d/dd-procmgrd`:

```
/var/log/dd-procmgrd/*.log {
    daily
    rotate 14
    compress
    delaycompress
    missingok
    notifempty
    create 0640 dd-procmgrd dd-procmgrd
    sharedscripts
    postrotate
        systemctl reload dd-procmgrd > /dev/null 2>&1 || true
    endscript
}
```

### Monitoring with Prometheus

Add to your service file:

```ini
[Service]
Environment="METRICS_ENABLED=true"
Environment="METRICS_PORT=9090"
```

### Integration with journald

All logs automatically go to journald. Query them:

```bash
# View structured logs
journalctl -u dd-procmgrd -o json-pretty

# Filter by priority
journalctl -u dd-procmgrd -p warning

# Follow logs from specific process
journalctl -u dd-procmgrd | grep "process_id=abc123"
```

---

## See Also

- [PRIVILEGE_MANAGEMENT.md](../PRIVILEGE_MANAGEMENT.md) - User switching and security
- [README.md](../README.md) - General usage documentation
- [systemd.service(5)](https://www.freedesktop.org/software/systemd/man/systemd.service.html) - systemd service manual
- [systemd.exec(5)](https://www.freedesktop.org/software/systemd/man/systemd.exec.html) - Execution environment configuration

