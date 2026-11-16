# Remote query upload destination

Remote queries use the `com.datadoghq.remoteaction.queries.execute` action on the
core Agent. Both `remote_queries.enabled` and the Private Action Runner's action
allowlist must permit the action.

## Remote action contract

The action selects its operation with the required boolean `resolveOnly`, not a
`mode` or `operation` string. Resolution accepts only `integration`, `target`, and
`resolveOnly`. Execution additionally requires `query`, `includeSchema`, and
`resultDelivery`; `timeout` is optional and belongs to the runner.

`resultDelivery` contains only `crawlId`, `taskId`, `uploadId`, and `limits`.
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
