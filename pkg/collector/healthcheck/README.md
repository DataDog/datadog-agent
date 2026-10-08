# Health-check remediation

Enable `health_check_remediation.enabled` in `datadog.yaml` to remediate integration
health checks through the local Private Action Runner (PAR). The flag defaults to
`false`. PAR must be enabled, enrolled, ready, and running its split-deployment
executor. The agent authenticates to the configured local executor socket with
the shared IPC certificate. If PAR is unreachable, unready, or lacks the local
remediation RPC, dispatch falls back to the Pass-1 dry-run event.

The PAR `private_action_runner.actions_allowlist` must explicitly permit
`com.datadoghq.remoteaction.rshell.runRemediationCommand`. Missing action
permission fails the attempt and emits `escalate`; it does not fall back to a
dry run.

Declare a health check at the top level of a file-provider integration config:

```yaml
init_config: {}
instances:
  - url: http://localhost:8080/health
health_check:
  enabled: true
  service_check: http.can_connect
  remediation:
    steps:
      - command: truncate -s 0 /var/log/example/stale.log
    cooldown: 5m
    max_attempts: 3
    allowed_paths:
      - /var/log/example:rw
    allowed_services: {}
```

Steps run sequentially in rshell remediation mode and stop on the first failure.
Use rshell-supported command names, not arbitrary host executables or scripts.
Execution is non-privileged: local tasks never grant escalation or carry a
Director proof. Ordinary executor-process filesystem permissions still apply.

The YAML provides the local substitute for normally signed
`SystemInputs.remote_action` policy. `allowed_paths` is passed explicitly to
rshell as directories, including optional `:ro` or `:rw` access suffixes. Command
permissions are synthesized from literal command names in each step, including nested commands
and pipelines. Dynamic command names and wildcard policy entries are rejected.
No paths or services are granted implicitly. To authorize a service operation,
explicitly declare its actions, for example:

```yaml
allowed_services:
  example.service:
    - restart
```

Local policy is still intersected with PAR operator restrictions. This deliberate
local trust path requires action-platform review; the OPMS path continues to
require signed task envelopes.

Only a previously observed OK followed directly by CRITICAL triggers dispatch.
An initial CRITICAL, WARNING-to-CRITICAL, and repeated CRITICAL do not trigger.
An empty `service_check` accepts any name, with transitions tracked separately
for each name and a shared attempt limit for the check ID.

`cooldown` defines a window starting at the first eligible transition. Up to
`max_attempts` distinct OK-to-CRITICAL transitions can dispatch within that
window. After expiry, the next eligible transition starts a fresh window.
Missing or invalid/non-positive cooldowns use five minutes; non-positive attempt
limits use three attempts.

Events report `detected`, `remediated`, or `escalate`, with per-step status and
exit codes. Raw command output and execution errors are omitted from these
events. `remediated` means all steps succeeded; the health check's next result
confirms recovery. A lost RPC after dispatch is reported as an uncertain outcome,
never as a dry run. An attempt is bounded to 30 seconds, with a one-second
readiness probe.

The sender hands observations to a bounded queue without waiting. A full queue
drops observations, so detection is best effort under backpressure.
The aggregator owns the worker and cancels outstanding dispatch at shutdown.
Unscheduling removes the declaration and transition state; queued observations
from older registrations cannot affect a newly registered check.
