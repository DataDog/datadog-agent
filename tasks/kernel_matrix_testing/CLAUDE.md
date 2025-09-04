# KMT Troubleshooting Guide

This supplements the main [README.md](./README.md) with solutions to common issues. **Read the README first** for basic setup.

## Quick Remote VM Setup

Remote VMs are most reliable:

```bash
# Clean existing stacks
dda inv -e kmt.destroy-stack --stack=<stack-name>

# Create stack configuration  
dda inv -e kmt.gen-config --vms=<vm-spec> --stack=<stack-name> --yes

# Launch stack (IMPORTANT: Let run completely - takes 5-10 minutes)
dda inv -e kmt.launch-stack --stack=<stack-name>

# Verify and test
dda inv -e kmt.status --stack=<stack-name>
```

## Tested VM Configurations

| Purpose | VM Spec | Kernel | Arch |
|---------|---------|--------|------|
| Ubuntu 20.04 | `arm64-ubuntu_20.04-distro` | 5.4.0-167-generic | ARM64 |
| Ubuntu 22.04 | `arm64-ubuntu_22.04-distro` | 5.15.0-91-generic | ARM64 |
| x86_64 | `x86_64-ubuntu_20.04-distro` | 5.4.0-167-generic | x86_64 |

## Common Issues

### Stack Launch Interruption
- **Always let `kmt.launch-stack` run to completion** (5-10 minutes)
- If interrupted, destroy and recreate stack completely

### Stack Corruption
Symptoms: `Failed to read stack output file`

```bash
dda inv -e kmt.destroy-stack --stack=<stack-name>
rm -rf /Users/$USER/kernel-version-testing/stacks/<stack-name>-ddvm
rm -rf kmt-deps/<stack-name>-ddvm
```

### SSH/VM Connection Issues
Check VM status on AWS instance:
```bash
ssh -i /Users/$USER/kernel-version-testing/ddvm_rsa ubuntu@<EC2-IP> 'sudo virsh list --all'
```

If VM is shut off, start manually:
```bash
ssh -i /Users/$USER/kernel-version-testing/ddvm_rsa ubuntu@<EC2-IP> 'sudo virsh start <vm-name>'
```

### Docker Permission Issues
Apply immediately after container starts:
```bash
docker exec -u root kmt-compiler-arm64 bash -c "mkdir -p /go/pkg/mod/cache && chown -R 503:20 /go/pkg/mod && chmod -R 755 /go/pkg/mod"
docker exec -u root kmt-compiler-arm64 bash -c "mkdir -p /go/bin && chown -R 503:20 /go && chmod -R 755 /go"
```

### Docker Disk Full
```bash
docker stop kmt-compiler-arm64 && docker rm kmt-compiler-arm64
# Container recreated automatically on next test
```

## Clean Environment Setup

```bash
# Destroy all stacks
ls /Users/$USER/kernel-version-testing/stacks/ | while read stack; do
    stack_name=$(echo $stack | sed 's/-ddvm$//')
    dda inv -e kmt.destroy-stack --stack=$stack_name
done

# Clean dependencies
rm -rf kmt-deps/*
docker stop kmt-compiler-arm64 && docker rm kmt-compiler-arm64 2>/dev/null || true
```

## Key Debugging Commands

```bash
# Stack status
dda inv -e kmt.status --stack=<stack-name>

# VM connectivity test
ssh -i /Users/$USER/kernel-version-testing/ddvm_rsa ubuntu@<EC2-IP> \
    'ssh -i /home/kernel-version-testing/ddvm_rsa root@100.1.0.2 "uname -a"'

# Check VMs on AWS instance
ssh -i /Users/$USER/kernel-version-testing/ddvm_rsa ubuntu@<EC2-IP> 'sudo virsh list --all'

# Start shut-off VM
ssh -i /Users/$USER/kernel-version-testing/ddvm_rsa ubuntu@<EC2-IP> 'sudo virsh start <vm-name>'
```

## Best Practices

- Focus on remote VMs only (local VMs have more issues)
- ARM64 configurations work well for most testing
- Always complete stack launch - don't interrupt
- Apply Docker permission fixes proactively
- Use clean slate approach when troubleshooting
- Test specific tests with `--run` filter for faster execution