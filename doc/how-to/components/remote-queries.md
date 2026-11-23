# Remote query upload destination

Remote queries use the `com.datadoghq.remoteaction.integrations.runQuery` action on the
core Agent. Both `remote_queries.enabled` and the Private Action Runner's action
allowlist must permit the action.

## Remote action contract

The action selects its operation with the required boolean `resolveOnly`, not a
`mode` or `operation` string. Resolution accepts only `integration`, `target`, and
`resolveOnly`. Execution additionally requires `query`, `includeSchema`, and
`resultDelivery`; `timeout` is optional and belongs to the runner.

`resultDelivery` contains only `crawlId`, `taskId`, `uploadId`, and `limits`.
(`crawlId` is required here. The embedded RC tunnel path has no crawl and
hands the integration a `resultDelivery` without it. That path is off by default:
it is registered only when `remote_queries.tunnel.execute_enabled` is true, with a
positive `remote_queries.tunnel.org_id`. Remote Config authorizes who
may call Execute; the Agent additionally requires each request to name its own
live connection ID, as observed from the handshake, and the configured org. If
either is unknown, the request is refused. While the handler is registered, the
Agent also sets the check inventory metadata key `remote_queries.tunnel_connection_id`
to the current connection ID on every loaded check instance that serves remote
queries, within about 30 s of a connection or of a newly scheduled instance.
Anyone with the org's API key can write inventory metadata, so the key is a
routing hint, not an authentication. This is experimental, for staging
validation.)
The remote caller does **not** provide `baseUrl`, another upload address, or
credentials. Unknown fields, including a legacy `resultDelivery.baseUrl`, are
rejected before any integration check is called. There is no remote override or
backward-compatible destination fallback.

## Agent-to-integration bridge

The core Agent derives the public HTTPS upload address from its locally
configured `site` and the fixed `/api/unstable/its-agent-intake` path. Public sites
use `https://api.<site>`, including `https://api.datad0g.com` for US staging
(`datad0g.com`). Unsupported or empty sites fail execution closed rather
than falling back to another destination. Resolution does not need an upload
endpoint. The metrics `dd_url` setting is not an upload destination override.

Only after validating the action input does the Agent add the trusted `baseUrl`
to the integration-facing `resultDelivery` JSON. This is a separate local bridge
contract: integrations continue receiving their existing payload and need no
runtime change. Database credentials remain with the loaded check, upload
authentication remains local, and the action response carries only the final
outcome, never result rows.
