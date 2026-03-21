# Windows Agent Service Tests

Tests for Windows Agent service lifecycle behavior. Unlike `install-test/`,
the agent here is installed by the Pulumi provisioner (not by the test itself),
so the suite starts with a running agent already present. Each test then
repeatedly starts and stops it, measuring and validating the behavior of each
cycle.

## What is tested

- All agent services start when the agent is started, and stop when it is stopped
- Service stop time (no service hits its hard stop timeout, which would produce an event log warning)
- Event log errors and warnings during start/stop cycles
- Hard-exit event log entries (service crashed rather than stopped cleanly)
- Correct behavior when individual sub-services (System Probe, Process Agent, Trace Agent, Installer) are disabled in config
- Starting a disabled service is handled gracefully

Each test suite is run twice: once normally, and once with **Driver Verifier**
enabled on the agent's kernel drivers. Driver Verifier applies additional
runtime checks that can surface memory and synchronization bugs; timeouts are
scaled up significantly for those runs.

## Diagnostics collected

After each test: WER crash dumps and the system crash dump (`MEMORY.DMP`) are
checked and downloaded. On failure: agent logs, event log errors/warnings, and
host diagnostics (I/O counters, disk queue length, process handle counts) are
collected.

## Test variants

Both variants are defined in `startstop_test.go`. The same suite runs
under two start/stop mechanisms to verify both work correctly:

- **`TestServiceBehaviorAgentCommand`** — uses `agent.exe start-service` / `stop-service`
- **`TestServiceBehaviorPowerShell`** — uses PowerShell `Start-Service` / `Stop-Service`

## CI

Jobs are in `.gitlab/windows/test/e2e/windows.yml` under
`new-e2e-windows-service-test`. Each test function runs as its own parallel
job via a `parallel: matrix`.
