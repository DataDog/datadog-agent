# cws-stream-events

`cws-stream-events` is a tiny standalone gRPC client that connects to a running
system-probe's CWS event-stream socket and pretty-prints every
`SecurityEventMessage` it receives to stdout.

It is meant for **local exploration and debugging only** — a stripped-down
stand-in for the loop that security-agent runs in production at
[`pkg/security/agent/agent.go:170-227`](../../pkg/security/agent/agent.go).

| Production: security-agent | Debug: cws-stream-events |
|---|---|
| auth_token / IPC plumbing | unauthenticated unix-socket dial |
| forwards to backend, telemetry | prints JSON to stdout |
| structured logging via `seclog` | `fmt.Printf` |
| metrics, retry counters, heartbeats | 2s sleep between reconnects |

It depends on the protobuf surface in
[`pkg/security/proto/api`](../../pkg/security/proto/api), so it lives inside
the main module and rebuilds against whatever the current proto looks like.
This is intentional: drift in the proto API will surface as a compile error
the next time someone rebuilds this tool.

## Build

```
go build -o bin/cws-stream-events ./tools/cws-stream-events
```

or, with the project's invoke wrapper:

```
dda inv -- go build -o bin/cws-stream-events ./tools/cws-stream-events
```

Linux only (`//go:build linux`); the underlying CWS event stream doesn't have
a portable equivalent on other platforms.

## Use

Start system-probe with CWS enabled and a runtime-security socket configured,
then point the helper at it:

```
sudo ./bin/cws-stream-events --socket /tmp/cws-explore/runtime-security.sock
```

Flags:

- `--socket` — path to the system-probe `runtime_security_config.socket`
  (default: `/tmp/cws-explore/runtime-security.sock`).
- `--pretty` — pretty-print the JSON payload (default `true`); pass
  `--pretty=false` for one-event-per-line output suitable for piping into
  `jq` / `grep`.

Output shape — one block per event, header line plus indented JSON:

```
=== rule=catchall_dns service= tags=[rule_id:catchall_dns ...] time=2026-05-07T00:55:16Z
  {
    "agent": { "rule_id": "catchall_dns", "policy_name": "default.policy", ... },
    "dns":   { "id": 12394, "is_query": true, "question": { "name": "example.com", ... } },
    "evt":   { "category": "Network Activity", "name": "dns",
               "rule_context": { "expression": "dns.question.name != \"\"" } },
    "network": { "destination": { "ip": "10.126.64.2", "port": 53 }, ... },
    "process": { "argv0": "getent", "executable": { ... }, "pid": ... }
  }
```

The JSON shape is what system-probe writes via
[`pkg/security/probe/serializers.go`](../../pkg/security/probe/serializers.go).
Note `evt.rule_context.expression` — that's the SECL predicate that fired,
your trail back from a runtime event to its rule.

## Caveats

- **Permissions.** The runtime-security socket is created `0660 root:dd-agent`
  by [`pkg/security/utils/grpc/grpc.go`](../../pkg/security/utils/grpc/grpc.go);
  this tool runs under `sudo` for that reason. The unix-socket dial is
  unauthenticated, so anyone with read access to the socket file can read
  events.
- **Rate limiting.** The server-side rate limiter is configured at
  `runtime_security_config.event_server.rate` /
  `runtime_security_config.event_server.burst` (struct fields
  `EventServerRate` / `EventServerBurst` in
  [`pkg/security/config/config.go:188-191`](../../pkg/security/config/config.go),
  bound to `viper` at lines 539-540, sized into the events channel at
  [`pkg/security/module/server.go:923`](../../pkg/security/module/server.go)).
  A noisy catch-all policy will silently drop events under the default
  `rate=10/burst=40`; bump those values up if you're seeing fewer events than
  you expect.
- **No backpressure.** This tool prints synchronously; if your terminal can't
  keep up, the gRPC stream will eventually be cancelled by system-probe.
- **Reconnect loop is dumb.** A flat 2s sleep, no exponential backoff. If you
  need production-grade resilience, you want
  [`pkg/security/agent/agent.go`](../../pkg/security/agent/agent.go) instead.
