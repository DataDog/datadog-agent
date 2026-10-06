# Health-check remediation (Pass 1)

Enable `health_check_remediation.enabled` in `datadog.yaml` to emit dry-run events
for integration health checks. The flag defaults to `false`. This pass never
executes commands and does not use health-platform storage.

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
      - command: /usr/local/bin/restart-example
    cooldown: 5m
    max_attempts: 3
    allowed_paths:
      - /usr/local/bin/restart-example
```

Only a previously observed OK followed directly by CRITICAL triggers an event.
An initial CRITICAL, WARNING-to-CRITICAL, and repeated CRITICAL do not trigger.
An empty `service_check` accepts any name, with transitions tracked separately
for each name and a shared attempt limit for the check ID.

`cooldown` defines a window starting at the first eligible transition. Up to
`max_attempts` distinct OK-to-CRITICAL transitions can dispatch within that
window. After expiry, the next eligible transition starts a fresh window.
Missing or invalid/non-positive cooldowns use five minutes; non-positive attempt
limits use three attempts. `allowed_paths` is retained as declaration data for
future execution support; this pass only describes the steps in an event.

The sender hands observations to a bounded queue without waiting. A full queue
drops observations, so dry-run detection is best effort under backpressure.
The aggregator owns the worker and cancels blocked event delivery at shutdown.
Unscheduling removes the declaration and transition state; queued observations
from older registrations cannot affect a newly registered check.
