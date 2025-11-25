# systemd Service Files

This directory contains systemd service files for deploying the Process Manager daemon in various configurations.

## Files

| File | Description | Use Case |
|------|-------------|----------|
| **dd-procmgrd.service** | Standard system service (root) | Production with privilege management |
| **dd-procmgrd-capability.service** | Capability-based service (non-root) | Enhanced security production |
| **dd-procmgrd-user.service** | User service | Development/testing |
| **dd-procmgrd-secure.service** | Hardened system service (root) | High-security production |
| **INSTALLATION.md** | Complete installation guide | Reference for all deployment types |

## Quick Start

### Standard Production Deployment

```bash
# Install
sudo cp dd-procmgrd.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable dd-procmgrd
sudo systemctl start dd-procmgrd

# Verify
sudo systemctl status dd-procmgrd
```

### User Service (Development)

```bash
# Install
mkdir -p ~/.config/systemd/user/
cp dd-procmgrd-user.service ~/.config/systemd/user/dd-procmgrd.service
systemctl --user daemon-reload
systemctl --user enable dd-procmgrd
systemctl --user start dd-procmgrd

# Verify
systemctl --user status dd-procmgrd
```

## Choosing the Right Service File

```
Need privilege management? ──────┬───── Yes ─────┬───── Want max security? ──── Yes ──── dd-procmgrd-secure.service
                                 │                │
                                 │                └───── No ───────────────────── dd-procmgrd.service
                                 │
                                 └───── No ─────────────┬───── Development? ───── Yes ──── dd-procmgrd-user.service
                                                        │
                                                        └───── Production? ────── Yes ──── dd-procmgrd-capability.service
```

### Decision Matrix

| Question | If YES | If NO |
|----------|--------|-------|
| Need to run processes as different users? | Continue → | Use `dd-procmgrd-user.service` |
| Running on Linux with capabilities support? | Consider `dd-procmgrd-capability.service` | Use `dd-procmgrd.service` |
| Need maximum security hardening? | Use `dd-procmgrd-secure.service` | Use `dd-procmgrd.service` |
| Development environment? | Use `dd-procmgrd-user.service` | Use `dd-procmgrd.service` |

## Service Comparison

### dd-procmgrd.service (Standard Root)
```
✓ Simple setup
✓ Works everywhere
✓ Full privilege management
⚠ Daemon runs as root
```

### dd-procmgrd-capability.service (Enhanced Security)
```
✓ Non-root daemon
✓ Capability-based privileges
✓ Better security posture
⚠ Linux-only
⚠ Requires xattr filesystem
⚠ More complex setup
```

### dd-procmgrd-user.service (Development)
```
✓ Simplest setup
✓ No root needed
✓ Perfect for testing
⚠ No privilege switching
⚠ Single-user only
```

### dd-procmgrd-secure.service (Maximum Hardening)
```
✓ Maximum systemd security
✓ Comprehensive restrictions
✓ Audit-ready
⚠ May need adjustments
⚠ More complex troubleshooting
```

## Installation Steps

### 1. Build the Binary
```bash
cd /workspace
cargo build --release
```

### 2. Choose Your Service File
Pick from the table above based on your needs.

### 3. Follow Installation Guide
See [INSTALLATION.md](INSTALLATION.md) for detailed instructions.

### 4. Verify Installation
```bash
# For system services
sudo systemctl status dd-procmgrd

# For user services
systemctl --user status dd-procmgrd

# Test CLI
pm-cli list
```

## Common Tasks

### View Logs
```bash
# System service
sudo journalctl -u dd-procmgrd -f

# User service
journalctl --user -u dd-procmgrd -f
```

### Restart Service
```bash
# System service
sudo systemctl restart dd-procmgrd

# User service
systemctl --user restart dd-procmgrd
```

### Check Configuration
```bash
# Verify service file syntax
systemd-analyze verify /etc/systemd/system/dd-procmgrd.service

# Show active configuration
systemctl cat dd-procmgrd

# Show with overrides
systemctl cat dd-procmgrd --full
```

### Override Settings
```bash
# Edit service (creates drop-in override)
sudo systemctl edit dd-procmgrd

# Add overrides:
[Service]
MemoryMax=2G
CPUQuota=150%

# Reload
sudo systemctl daemon-reload
sudo systemctl restart dd-procmgrd
```

## Directory Structure

After installation, you should have:

```
System Service (Root):
├── /usr/bin/dd-procmgrd              # Main daemon binary
├── /usr/bin/dd-procmgr-cli                 # CLI tool
├── /etc/systemd/system/
│   └── dd-procmgrd.service           # Service file
├── /etc/dd-procmgrd/
│   ├── processes.yaml              # Process config
│   └── daemon.env                  # Environment (optional)
├── /var/lib/dd-procmgrd/             # Working directory
├── /var/log/dd-procmgrd/             # Logs
└── /run/dd-procmgrd/                 # Runtime files

User Service:
├── ~/.local/bin/
│   ├── dd-procmgrd
│   └── pm-cli
├── ~/.config/systemd/user/
│   └── dd-procmgrd.service
├── ~/.config/dd-procmgrd/
│   └── processes.yaml
└── ~/.local/share/dd-procmgrd/       # Working directory & logs
```

## Security Considerations

### System Service (Root)
- Daemon has full root privileges
- Binds to localhost only (127.0.0.1:50051)
- Drops privileges per managed process
- Standard systemd security model

### Capability-Based Service
- Daemon runs as non-root user
- Only CAP_SETUID and CAP_SETGID capabilities
- Binary must be protected from modification
- Requires filesystem with xattr support

### Security Checklist
- [ ] Binary in /usr/bin/ owned by root
- [ ] Service binds to 127.0.0.1 only
- [ ] Config files have restricted permissions
- [ ] Logs directory properly owned
- [ ] Resource limits configured
- [ ] Security hardening options enabled

## Troubleshooting

### Service Won't Start
```bash
# Check status and logs
sudo systemctl status dd-procmgrd -l
sudo journalctl -u dd-procmgrd -n 50

# Validate service file
systemd-analyze verify /etc/systemd/system/dd-procmgrd.service

# Test binary directly
sudo /usr/bin/dd-procmgrd
```

### Permission Errors
```bash
# Check file permissions
ls -l /usr/bin/dd-procmgrd
ls -ld /var/lib/dd-procmgrd

# For capability service
getcap /usr/bin/dd-procmgrd
```

### Port Already in Use
```bash
# Find what's using port 50051
sudo ss -tlnp | grep 50051

# Kill old daemon
sudo pkill -9 dd-procmgrd
sudo systemctl restart dd-procmgrd
```

## Additional Resources

- **[INSTALLATION.md](INSTALLATION.md)** - Complete installation guide with troubleshooting
- **[../PRIVILEGE_MANAGEMENT.md](../PRIVILEGE_MANAGEMENT.md)** - Security and privilege management
- **[../README.md](../README.md)** - General documentation
- **[systemd.service(5)](https://www.freedesktop.org/software/systemd/man/systemd.service.html)** - systemd service manual

## Support

For issues or questions:
1. Check [INSTALLATION.md](INSTALLATION.md) troubleshooting section
2. Review logs: `journalctl -u dd-procmgrd`
3. Validate service file: `systemd-analyze verify`
4. Test binary directly: `sudo /usr/bin/dd-procmgrd`

## License

See [../LICENSE](../LICENSE) for license information.

