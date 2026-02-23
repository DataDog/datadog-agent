# Remote Agent Example

A standalone example client that implements the Remote Agent Registry (RAR) contract: it registers with the core agent over IPC, runs a gRPC server (Status, Flare, Telemetry), and refreshes its registration periodically.

## Purpose

Used to test and demonstrate RAR integration: the core agent discovers the remote agent’s gRPC endpoint, calls Status/Flare/Telemetry as needed, and keeps the session alive via refresh. This binary is not for production; it is a minimal reference implementation.

## Building

```bash
# With invoke (from repo root)
dda inv agent.build-remote-agent

# Or with go
go build -o bin/remote-agent ./internal/remote-agent
```

The binary is produced at `bin/remote-agent`.

## Prerequisites

1. **Running core agent** with RAR enabled (`remote_agent_registry.enabled: true`, which is typical).
2. **Auth token file** – same as the core agent’s (e.g. `bin/agent/dist/auth_token` after the agent has run).
3. **IPC certificate file** – same PEM file the core agent uses for IPC (e.g. `bin/agent/dist/ipc_cert.pem`). Required for mTLS to the agent’s IPC server and for the remote agent’s gRPC server TLS.

Ensure the core agent has run at least once so `auth_token` and `ipc_cert.pem` exist under its config/dist directory.

## Running (testing)

All of the following flags are required.

| Flag | Description | Example |
|------|-------------|---------|
| `-agent-flavor` | Flavor reported to RAR | `my-remote-agent` |
| `-display-name` | Display name in RAR | `Test Remote Agent` |
| `-listen-addr` | Address for the remote agent’s gRPC server | `127.0.0.1:0` (any port) or `127.0.0.1:50051` |
| `-agent-ipc-address` | Core agent IPC address | `localhost:5001` |
| `-agent-auth-token-file` | Path to auth token file | `bin/agent/dist/auth_token` |
| `-agent-cert-file` | Path to IPC cert PEM (cert + key) | `bin/agent/dist/ipc_cert.pem` |

Example from repo root (core agent must be running and listening on `localhost:5001`):

```bash
./bin/remote-agent \
  -agent-flavor test-remote-agent \
  -display-name "Test Remote Agent" \
  -listen-addr 127.0.0.1:50051 \
  -agent-ipc-address localhost:5001 \
  -agent-auth-token-file bin/agent/dist/auth_token \
  -agent-cert-file bin/agent/dist/ipc_cert.pem
```

You should see logs like:

- `Spawned remote agent gRPC server on 127.0.0.1:50051.`
- `Registering with Core Agent at localhost:5001...`
- `Registered with Core Agent. Recommended refresh interval of N seconds.`
- `Refreshed registration with Core Agent.` (repeating)

## How to test that it works

1. **Build** the core agent and run it so `bin/agent/dist/auth_token` and `bin/agent/dist/ipc_cert.pem` exist.
2. **Start the core agent** (e.g. `./bin/agent/agent run -c bin/agent/dist/datadog.yaml`).
3. **Build** the remote-agent: `dda inv agent.build-remote-agent` or `go build -o bin/remote-agent ./internal/remote-agent`.
4. **Run** the remote-agent with the six flags above, using `bin/agent/dist` for `-agent-auth-token-file` and `-agent-cert-file`.
5. **Check logs**: registration and periodic “Refreshed registration” confirm the client cert and RAR flow work. You can also run `agent status` or use the GUI/API to see the registered remote agent.

If the core agent is not running or the cert/token paths are wrong, you will see connection or auth errors instead of registration success.

## See also

- **Remote Agent Registry:** `comp/core/remoteagentregistry/`
- **Config stream test client:** `cmd/config-stream-client/` (also connects to agent IPC with client cert)
