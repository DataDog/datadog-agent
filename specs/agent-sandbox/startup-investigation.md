# Agent Sandbox Startup Investigation Notes

## Current finding

With `--fx-trace`, the sandbox captures real Fx startup spans from the published Agent package. The dominant Fx span observed so far is:

```text
~6.7s onStartHook comp/process/runner/impl.(*runnerImpl).Run-fm()
```

The root Fx startup span is about 8s, while end-to-end sandbox readiness to `agent status` is about 100s in hot runs. This means both installer/cloud-init and Agent startup topology matter.

## Known timing landmarks

A representative hot run with prepared base showed:

```text
prepare_done          ~4s
validate_done         ~6s
vm_started            ~8s
ssh_ready             ~18s
agent_binary_present  ~56s
cloud_init_done       ~75s
service_active        ~75s
agent_status_ready    ~104s
```

Normal Agent logs show the CMD API server itself starts quickly once its OnStart hook runs:

```text
Started HTTP server 'CMD API Server' on 127.0.0.1:5001
```

The Fx span for `comp/api/api/apiimpl.newAPIServer.func1()` was around 1ms.

## Agent-side investigation candidates

1. Process runner startup
   - `comp/process/runner/impl.(*runnerImpl).Run-fm()` dominates Fx OnStart time.
   - Related logs show network ID detection retries and IMDS timeouts in non-cloud VMs.
   - Candidate code areas:
     - `comp/process/runner/impl`
     - `pkg/process/checks/process.go`
     - `pkg/process/checks/net.go`
     - `pkg/process/checks/net_linux.go`

2. CMD API ordering
   - API startup is cheap but appears late in the startup sequence.
   - Candidate code areas:
     - `comp/api/api/apiimpl`
     - `cmd/agent/subcommands/run/command.go`

3. Remote config startup race
   - Logs show early remote-config polling can race CMD API availability.
   - Candidate code areas:
     - `comp/remote-config`
     - `pkg/config/remote/client`

4. Non-cloud metadata probing
   - Logs show repeated attempts to reach `169.254.169.254` in local VMs.
   - Candidate improvement: cache non-cloud result or make network ID detection async/lazy.

## Next useful data

Run:

```bash
dda inv sandbox.up --name fx --fx-trace
dda inv sandbox.fx-spans --name fx --summary
```

Then correlate the top spans with `/var/log/datadog/agent.log` and `journalctl -u datadog-agent`.
