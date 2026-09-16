# e2ectl: local agent test — validation plan

> **Implemented — live-verified (local path 4/4 in 0.07s; EC2 path pending the
> correct Pulumi passphrase in the user's environment).**
> Companion to the [test-integration code plan](qa-e2ectl-local-agent-test-integration-plan.md).

## The test

`test/new-e2e/tests/agent-metric-emission/metric_emission_test.go` — four
assertions that pass identically on both environments:

| Test | What it proves |
|---|---|
| `TestAgentHeartbeat` | The Agent is alive and flushing to the fakeintake (the most common e2e pattern) |
| `TestAgentCommandExecution` | `RemoteHost.Execute` works — SSH on EC2, docker exec locally |
| `TestFakeintakeReachable` | The fakeintake is queryable from the test framework |
| `TestCpuCheckIsRunning` | A default Go core check is emitting metrics (the conf.d seeding works) |

## Validation matrix

### A. Local Docker container (the new path)

```sh
# Start and install the local environment:
e2ectl init --base local --output /tmp/val.yaml
e2ectl start --config /tmp/val.yaml --name val
e2ectl install --env val

# Run the test against it:
E2ECTL_LOCAL_ENV=val go test ./tests/agent-metric-emission/ -run TestMetricEmissionOnLocal -v
```

Gate: all four tests pass. `TestAgentCommandExecution` runs `cat /etc/datadog-agent/datadog.yaml`
via docker exec (no sudo needed — cat works as root).

### B. Remote EC2 VM (the existing path)

```sh
go test ./tests/agent-metric-emission/ -run TestMetricEmissionOnHost -v
```

Gate: all four tests pass (provisioning the VM, installing the Agent via the
install script, and asserting via SSH + the ECS Fargate fakeintake). This is the
existing behavior — it should be unaffected by the transport change.

### C. The transport is transparent

Run both paths and compare: the test body is identical, the provisioner is
the only difference. If any assertion passes on one and fails on the other,
that is a capability gap to document (e.g. sudo, systemd), not a test bug.

## What is deliberately NOT tested yet

- **sudo commands** — the test avoids `sudo` entirely (`cat` works as root).
  A test using `sudo cmd` will fail in the container until a shim is added.
- **systemd / service management** — no `systemctl` in the test; the container
  has no systemd.
- **File transfer** (`GetFolder`, `CopyFile`) — not tested; the docker
  artifact client returns a "not supported" error.
- **Agent HTTP API** (`DialPort`, `NewHTTPClient`) — not tested; the container's
  port 5001 is not exposed.
- **Package installation** — the test uses `cat`, not `dpkg -l` or `apt`.

When these become needed, the test gains a second suite variant that exercises
them (and runs only on the VM), or the local environment gains the capability.

## Hermetic checks (no Docker, no AWS)

- `HostFileSystem` interface compliance: both `sshExecutor` and `dockerExecutor`
  satisfy it (compile-time assertion already in the code).
- `NewHost` with `Transport: "docker"` → returns a `Host` with a
  `dockerExecutor`, no SSH attempted.
- `NewHost` without `Transport` → returns a `Host` with the existing SSH
  executor — the existing path is byte-identical.
- The snapshot entry has `transport: "docker"` and `address` is the container
  name — verified by reading the local env's snapshot.

## Cleanup

After validation: `e2ectl stop --env val` (cleans container, network, entry).
The EC2 path cleans itself via the standard provisioner teardown.
