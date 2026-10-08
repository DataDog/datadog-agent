# Health-check remediation

Enable `health_check_remediation.enabled` in `datadog.yaml` to remediate integration
health checks through the local Private Action Runner (PAR). The flag defaults to
`false`. The always-on PAR component registers a `remediation` command with the
Remote Agent Registry when remediation and `remote_agent.registry.enabled` are
enabled. Registration uses PAR's existing IPC authentication and a dedicated
listener. The on-demand split executor does not register. If the registry is
disabled or PAR is absent or lacks the command, dispatch falls back to a dry-run
event. An unavailable provider or dropped stream has an uncertain outcome,
even if no response frame has been received, because steps may have executed.

The `remediation` command provider trusts agent-authored remediation and a
caller-supplied allowlist carried over the authenticated Remote Agent Registry.
This path runs non-privileged rshell with a default-deny allowlist and is gated
by both `health_check_remediation.enabled` and `remote_agent.registry.enabled`.
Only explicitly allowed commands, paths, and service actions can run.

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
Director proof. Ordinary PAR-process filesystem permissions still apply.

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
confirms recovery. Registry `NotFound` and `Unimplemented` errors produce
dry-run events. `Unavailable` always produces `detected` followed by an
uncertain-outcome `escalate` event because steps may have run, even if no
response frame has been received.
Other transport errors and malformed or incomplete responses also produce an
uncertain-outcome `escalate` event. An attempt is bounded to 30 seconds.

The sender hands observations to a bounded queue without waiting. A full queue
drops observations, so detection is best effort under backpressure.
The aggregator owns the worker and cancels outstanding dispatch at shutdown.
Unscheduling removes the declaration and transition state; queued observations
from older registrations cannot affect a newly registered check.
