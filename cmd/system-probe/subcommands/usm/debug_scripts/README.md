# USM Debug Scripts

This folder contains Python scripts for debugging USM (Universal Service Monitoring) in staging, production, and customer environments.

## Why Python?

We use Python scripts because:
- The system-probe container already includes Python
- Scripts can be quickly copied and executed without recompilation
- Much faster iteration compared to writing debug tools in Go, compiling, and deploying a new version of system-probe

## Available Scripts

### usm_leak_detector

Detects leaked entries in USM eBPF maps by comparing map contents against active TCP connections.

See [usm_leak_detector/README.md](usm_leak_detector/README.md) for detailed usage instructions.