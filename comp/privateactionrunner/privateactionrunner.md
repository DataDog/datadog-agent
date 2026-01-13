## Private Action Runner component

Technical reference for `comp/privateactionrunner`, which bootstraps and runs the Private Action Runner (PAR) inside the Agent.

### Directory layout
- `def/`: empty interface definition for the component type.
- `fx/`: fx module that wires the component; provided even when nothing depends on it to force instantiation.
- `impl/`: component implementation and lifecycle hooks.
- `pkg/privateactionrunner`: runnable logic (configuration adapter, enrollment, task verifier, workflow runner, bundles, OPMS client).

### High-level responsibilities
- Gate PAR behind `privateactionrunner.enabled`.
- Build runtime configuration from Agent config (including allowlists and defaults).
- Fetch or self-generate runner identity (URN + private key) and persist it.
- Subscribe to Remote Config for public keys used to verify signed tasks.
- Start a workflow loop that dequeues tasks from OPMS, verifies signatures, resolves credentials, executes allowed actions, heartbeats, and publishes results.

### Lifecycle and dependencies
`impl.NewComponent` requires:
- `config.Component`: reads runner settings and mutates runtime values (URN/private key) after enrollment.
- `log.Component`: structured logging.
- `compdef.Lifecycle`: registers start/stop hooks.
- `rcclient.Component`: Remote Config client for key distribution.

Execution path:
1. If disabled, returns a no-op component.
2. Load persisted identity from the configured identity file; if found, inject into runtime config.
3. Build `parconfig.Config` via `parconfig.FromDDConfig` (applies defaults and allowlists).
4. If identity is incomplete:
   - When `privateactionrunner.self_enroll` is `true`, perform self-enrollment against `https://api.<site>` using API+APP keys, persist identity, and inject URN/private key into runtime config.
   - Otherwise, fail startup with an identity error.
5. Log startup metadata (runner version/site/URN).
6. Construct helpers:
   - `taskverifier.KeysManager` (subscribes to Remote Config product `action_platform_runner_keys`).
   - `taskverifier.TaskVerifier` (verifies signature, org ID, runner ID).
   - `opms.Client` (task dequeue, heartbeat, result publish).
   - `runners.WorkflowRunner` (task loop + execution).
7. Register lifecycle hooks:
   - `OnStart`: run `WorkflowRunner.Start` on a background context.
   - `OnStop`: close the runner gracefully.

### Runtime flow
- **Key subscription**: KeysManager subscribes to Remote Config and blocks `WorkflowRunner.Start` until the first key set is applied.
- **Task loop**: `runners.Loop` polls OPMS via a circuit breaker. When no task is available, it sleeps for `LoopInterval` (default 1s).
- **Validation and verification**:
  - Validate the dequeued task envelope.
  - Verify signature and expiry using the Remote Config keys; enforce org/runner match with local config.
  - Propagate the dequeued job ID into the verified task.
- **Credential resolution**: map `connectionInfo` into executable credentials via `resolver.PrivateCredentialResolver`.
- **Execution**:
  - Locate the bundle/action from the registry (`pkg/privateactionrunner/bundles`), keyed by FQN `bundle.action`.
  - Enforce `actions_allowlist` and HTTP host allowlist (`privateactionrunner.allowlist`) before running.
  - Spawn execution with pool size `RunnerPoolSize` (default 1).
  - Emit heartbeats on `HeartbeatInterval` (default 20s) while the action runs.
  - Publish success or failure to OPMS with client/action/job identifiers.

### Configuration reference
Key settings consumed by `comp/privateactionrunner` and `parconfig.FromDDConfig`:
- `privateactionrunner.enabled` (bool): feature gate.
- `privateactionrunner.self_enroll` (bool): allow automatic enrollment when identity is missing.
- `privateactionrunner.private_key` / `privateactionrunner.urn`: identity (JWK/Base64 + URN). Set automatically on successful self-enrollment or from persisted identity.
- `privateactionrunner.identity_file_path`: optional override for persisted identity file; defaults next to the main Agent config (or alongside `auth_token_file_path`).
- `privateactionrunner.actions_allowlist` (string slice): list of FQNs (`bundle.action`); `*` allowed per bundle.
- `privateactionrunner.allowlist` (comma-separated): hostname glob allowlist for HTTP bundle actions.
- `privateactionrunner.allow_imds_endpoint` (bool): allow HTTP bundle access to IMDS.
- Runtime tuning defaults (overridable in config):
  - Backoff and retry: `MinBackoff=1s`, `MaxBackoff=3m`, `MaxAttempts=20`, `WaitBeforeRetry=5m`.
  - Loop and execution: `LoopInterval=1s`, `RunnerPoolSize=1`, `OpmsRequestTimeout=30s`.
  - Heartbeats and health: `HeartbeatInterval=20s`, `HealthCheckInterval=30s`, `HealthCheckEndpoint=/healthz`.
  - HTTP server: `Port=9016`, `HttpServerReadTimeout=10s`, `HttpServerWriteTimeout=60s`.
  - Auth headers: `RunnerAccessTokenHeader`, `RunnerAccessTokenIdHeader`.
  - Metrics: defaults to `statsd.NoOpClient`.

### Bundles and action FQNs
Actions are grouped under bundles in `pkg/privateactionrunner/bundles` (e.g., `gitlab`, `kubernetes`, `http`, `jenkins`). The allowlist uses `bundle.action` names; HTTP bundles additionally enforce hostname allowlisting. Unknown bundles/actions or disallowed entries yield sanitized PAR errors.

### ASCII architecture
```
          Agent config
              |
       parconfig.FromDDConfig
              |
   +---------------------------+
   | privateactionrunner impl  |
   | (fx Module -> NewComponent)|
   +-------------+-------------+
                 |
        +--------v--------+
        | Identity & Config|<-- persisted identity file / self-enroll
        +--------+--------+
                 |
   +-------------v-------------+
   |  WorkflowRunner           |
   |  - Registry (bundles)     |
   |  - Resolver               |
   |  - TaskVerifier           |
   |  - OPMS client            |
   |  - KeysManager (RC)       |
   +-------------+-------------+
                 |
         +-------v-------+
         |   Loop        |
         | - Dequeue     |
         | - Verify      |
         | - Resolve     |
         | - Execute     |
         | - Heartbeat   |
         | - Publish     |
         +---------------+
```

### Operational notes
- Start occurs on Agent lifecycle start even if nothing depends on the component (fx `Invoke` forces instantiation).
- Heartbeats and metrics run until the loop is stopped via lifecycle `OnStop`.
- Failures in enrollment or missing Remote Config keys prevent successful task processing and are surfaced via startup errors or task-level failures.