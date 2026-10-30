# Intake routing architecture

Intake routing lets the Datadog backend select the host that receives the telemetry of an Agent.
The backend can send a customer to a customer-specific host, or to a specialized host that
handles that customer for a period of time.

The Agent asks the backend for this host. It checks the answer and makes a test request to the
new host. Then it changes to the new host.

The feature is disabled by default (`intake_routing.enabled: false`). At this time, only the core
logs agent uses it.

- [Terms](#terms)
- [Scope](#scope)
- [Component API](#component-api)
- [Routes and groups](#routes-and-groups)
- [Startup](#startup)
- [Refresh](#refresh)
- [How consumers get changes](#how-consumers-get-changes)
- [Decisions](#decisions)
- [Safety checks](#safety-checks)
- [Wire protocol (provisional)](#wire-protocol-provisional)
- [Configuration](#configuration)
- [Telemetry](#telemetry)
- [Logs agent integration](#logs-agent-integration)
- [Code and tests](#code-and-tests)
- [Other telemetry types](#other-telemetry-types)
- [Trade-offs and risks](#trade-offs-and-risks)

## Terms

This document uses these terms with one meaning only.

| Term | Meaning |
|---|---|
| Consumer | An Agent component that sends telemetry and registers its endpoints with this component. Example: the logs agent. |
| Default endpoint | The endpoint that comes from the Agent configuration. |
| Override | An endpoint that the backend tells the Agent to use instead of the default endpoint. |
| Current endpoint | The endpoint that the consumer must use now: the override if one is active, else the default endpoint. |
| Route | One default endpoint that a consumer registered, with its API key. |
| Site | The Datadog site of a default endpoint, for example `datadoghq.com` or `datadoghq.eu`. |
| Group | All the routes that have the same site and the same API key. |
| Query | The request that asks the backend for the overrides of one group. |
| Pre-flight check | A test request to an override before the Agent uses it. |
| Grace period | The maximum time that a consumer waits at startup for the first decision. |

## Scope

| Telemetry type | Status |
|---|---|
| Core logs agent (`agent-http-intake.logs.<site>`) | Implemented |
| Metrics, traces, EVP | Not implemented. Refer to [Other telemetry types](#other-telemetry-types). |
| CWS and compliance, agent telemetry, OTel, serverless | Out of scope. These are different telemetry types. |

## Component API

`def/component.go` defines the API. The API is generic: it does not contain HTTP or logs
details. Thus, a different implementation can supply the same API to other processes, for example
over IPC.

```go
type TelemetryType string                      // Logs = "logs"
type Endpoint struct{ Host string; Port int }  // Port 0 = 443. Always HTTPS.

type Request struct {
	Type    TelemetryType
	Default Endpoint                           // The default endpoint. Its host gives the site.
	APIKey  func() (apiKey string, ready bool) // Read every 5 seconds and before each query.
}

type Component interface {
	Register(reqs []Request) Registration // Does not block.
}

type Registration interface {
	WaitInitial(ctx context.Context) []Endpoint // Returns the current endpoints after the first decisions.
	Current() []Endpoint                        // One current endpoint for each request.
	Changes() <-chan struct{}                   // Gets a signal when Current() can be different.
	Close()                                     // Stops the routing of these requests.
}
```

The `APIKey` function lets the component follow API key rotation and delegated authentication.
The component never sends a key that is not `ready`.

When the feature is disabled, `Register` returns the default endpoints. `WaitInitial` returns
immediately. The component does not start a background task.

## Routes and groups

Each request of a registration becomes one route. The component gets the site of the route from
the host of its default endpoint. If the host is not under an official Datadog domain, the
component never routes it.

The component puts routes into groups by site and API key. It sends one query for each group.
The query does not select telemetry types. The answer contains an entry for each telemetry type
that the backend has an answer for. Each route uses the entry of its own telemetry type. The
component ignores the other entries, and it does not validate them.

An API key belongs to one Datadog organization. Thus, each organization gets its own answer, from
the site that receives its telemetry. Examples:

| Endpoints | Queries |
|---|---|
| Main endpoint and one additional endpoint, same site, same key | One query |
| Two endpoints on the same site, keys of two organizations | Two queries, one for each key |
| Two endpoints on different sites | One query to each site, each with the key of its endpoint |

When a route joins a group that already has an answer, it uses that answer immediately. The
component does not send a new query.

The component reads the API key of every route every 5 seconds, and before each query. When an
API key changes, the route moves to the group of the new key. Thus, a rotation is found within 5
seconds, not at the next refresh. The component queries the new group immediately, unless the group already
has an answer. It deletes the old group when the group has no more routes.

If the API key of a route is not ready, the route keeps its current endpoint until the key is
ready.

## Startup

At startup, the consumer must not wait for the backend for a long time. Thus, the component uses a
grace period (1 second by default).

1. The consumer calls `Register`. `Register` returns immediately. The first queries start in the
   background.
2. The consumer calls `WaitInitial`. `WaitInitial` returns when one of these conditions occurs:
   - The component has a decision for each route.
   - The grace period (counted from `Register`) is over.
   - The context of the consumer is done.
3. The consumer starts to send telemetry to the endpoints that `WaitInitial` returned.
4. If an answer arrives after the grace period, the component applies it. Then it sends a signal
   to the consumer, and the consumer changes to the new endpoints.

A failed query is also a decision: the route keeps its default endpoint. A route whose API key
is not ready does not make `WaitInitial` wait.

If the queries of a group fail before the group gets its first answer, the component tries again
quickly. The first retry occurs after 1 second. Each retry doubles the delay, to a maximum of 5
minutes.

## Refresh

After the first answer, the component sends the query of each group again at a regular interval.
This interval is the refresh interval (6 hours by default). The refresh occurs in the background.
It does not stop or slow the consumer.

| Result of the last query | Next query |
|---|---|
| An answer from the backend (this includes HTTP 404, an override that is not valid, and a failed pre-flight check) | After the refresh interval |
| A failure, before the first answer of the group | After the startup retry delay |
| A failure, after the first answer of the group | After the refresh interval |

A random variation (jitter) changes each delay: ±10% for the refresh interval, ±20% for the
startup retries. This prevents many Agents from sending their queries at the same time.

If an override starts to fail between two refreshes, the component does not change it. The
consumer retries its requests as usual until the next refresh.

## How consumers get changes

The component does not change the endpoints of the consumer. The consumer reads them.

1. The consumer starts on the endpoints that `WaitInitial` returned.
2. The consumer listens on `Changes()`.
3. When a decision changes a current endpoint of the registration, the component sends a signal on
   `Changes()`.
4. The consumer calls `Current()` and compares the result with the endpoints that it uses.
5. If the endpoints are different, the consumer changes to the new endpoints.

The `Changes()` channel holds a maximum of one signal. Thus, many changes in a short time can
give only one signal. A signal can also occur when no endpoint changed. For these two reasons,
the consumer must always compare the endpoints. It must not count the signals.

After `Close` returns, the registration gets no more signals.

## Decisions

For each route, the component applies the first row of this table that matches.

| Condition | Result |
|---|---|
| The Agent does not reach the default host directly (private address or proxy). Refer to [Default host path check](#default-host-path-check). | Use the default endpoint. If no route of the group can be routed, do not send the query. |
| The Agent cannot resolve the default host | Keep the current endpoint. Send the query. |
| The query failed (network error, timeout, unexpected HTTP status, answer that cannot be read) | Keep the current endpoint |
| HTTP 404, or the answer has no entry for this telemetry type | Keep the current endpoint |
| The answer says "no override", or the override is equal to the default endpoint | Use the default endpoint immediately. Do not do a pre-flight check. |
| The override is not valid | Keep the current endpoint. Log a warning. |
| The override is equal to the current endpoint | No change |
| The override is different and the pre-flight check passes | Use the override |
| The override is different and the pre-flight check fails | Keep the current endpoint |

The component sends a maximum of one pre-flight check for each host, port, and API key during a
refresh.

## Safety checks

### Override validation

The component accepts an override only when all these conditions are true:

- The host is a DNS name. It contains only letters, digits, hyphens, and dots. It is not an IP
  address. It does not contain a path, user information, or a port.
- The host is under an official Datadog domain. It can be under a different site than the default
  endpoint.
- The host is not the site itself (for example, `datadoghq.com`).
- The port is between 0 and 65535.

The Agent always sends telemetry to an override over HTTPS.

### Pre-flight check

The component sends `GET https://<host>[:port]/probe` with the API key of the route in the
`DD-API-KEY` header. Any 2xx status passes. A redirect fails the check.

### Default host path check

Customers that use AWS PrivateLink (or a similar private connection) often keep the official host
names. Their private DNS resolves `agent-http-intake.logs.<site>` to a VPC endpoint. An override
from the backend is not reachable through this VPC endpoint. If the Agent used the override, the
telemetry would go over the internet, at a much higher cost for the customer.

Thus, before each query, the component examines how the Agent reaches the default host of each
route. It does this examination with the same proxy settings that the consumer uses:

1. **Proxy**: if the proxy settings (`proxy.https` and `no_proxy`) send the host through a proxy,
   the Agent cannot know how the proxy reaches the host. The component does not route it.
2. **DNS resolution**: the component resolves the host as an absolute name (with a final dot).
   If the resolution fails, the result is `lookup_failed`.
3. **Private address**: if one or more addresses are not public, the component does not route the
   host. Public means a global unicast address outside RFC 1918, RFC 4193, and RFC 6598.

| Result | Effect |
|---|---|
| Proxy or private address | Use the default endpoint, also when an override is active. If no route of the group can be routed, do not send the query. |
| `lookup_failed` | Keep the current endpoint. Send the query. If DNS is not available at startup, the query also fails, and the startup retries examine the host again soon. |

The component does this examination on each refresh. Thus, it finds a new or removed PrivateLink
setup at the next refresh.

These cases are not found: a VPC that gives public addresses to its endpoints, and IPv6-only
interface endpoints. A comparison with the published Datadog IP ranges (`ip-ranges.<site>`) can
find them in a later version.

### Logs

The component never logs API keys. It logs these events:

| Event | Level |
|---|---|
| Change to an override, or back to the default endpoint | Info |
| Failed pre-flight check, override that is not valid | Warn |
| The Agent stops or starts to reach a default host directly | Info (Warn for a DNS failure), one time for each change |
| First failed query of a series | Warn |
| Next failed queries of the same series | Debug |
| First successful query after a series of failures | Info |

## Wire protocol (provisional)

The backend API is not final. The component uses the `routingClient` interface.
`client_http.go` implements a provisional format. When the API is final, replace this file.

Query, with the API key of the group. The query has no parameters:

```
GET https://api.<site>/api/unstable/agent/intake-routing
DD-API-KEY: <key>
Accept: application/json
User-Agent: datadog-agent/<version>
```

Answer (2xx):

```json
{"routes": {"logs": {"override": true, "host": "<host>", "port": 443}}}
```

- The answer contains all the telemetry types that the backend has an answer for. It is not
  limited to the types that the Agent uses.
- `"override": false` means "no override": use the default endpoint.
- If a telemetry type is not in the answer, there is no answer for it.
- The component ignores the entries of telemetry types that no route uses.
- An answer without a `routes` object, or larger than 64 KiB, is not valid.

| HTTP status | Query status |
|---|---|
| 2xx with a valid body | `success` |
| 404 | `not_offered` |
| 401, 403 | `http_error`, with a warning about the API key |
| Other status, or a redirect that is not permitted | `http_error` |
| 2xx with a body that is not valid | `invalid_response` |
| Time limit | `timeout` |
| Other transport errors | `network_error` |

The component follows a maximum of 3 redirects. It follows only HTTPS redirects to an official
Datadog domain, because Go sends custom headers such as `DD-API-KEY` to the new host.

The HTTP client uses the proxy and TLS settings of the Agent.

## Configuration

These settings are in `pkg/config/schema/yaml/core_schema.yaml`. They are not documented for
customers, and they apply only to the full Agent.

| Setting | Default | Limits |
|---|---|---|
| `intake_routing.enabled` | `false` | |
| `intake_routing.startup_grace_period` | `1s` | 0s to 30s. A value outside this range is changed, with a warning. |
| `intake_routing.refresh_interval` | `6h0m0s` | 1m minimum. A smaller value is changed, with a warning. |

## Telemetry

The metrics use the subsystem `intake_routing`. No tag contains a host or a key.

| Metric | Type | Tags |
|---|---|---|
| `queries` | counter | `status`: `success`, `not_offered`, `network_error`, `timeout`, `http_error`, `invalid_response` |
| `preflights` | counter | `status`: `success`, `failure` |
| `switches` | counter | `telemetry_type`; `direction`: `to_enhanced`, `to_default`, `enhanced_to_enhanced` |
| `rejected_answers` | counter | `telemetry_type`; `reason`: `invalid_host`, `absent` |
| `skipped_endpoints` | counter | `telemetry_type`; `reason`: `private_address`, `proxied`, `lookup_failed` |
| `active_overrides` | gauge | `telemetry_type` |

Agent telemetry (COAT) sends all these metrics from all Agents, with the `intake-routing` profile
of `comp/core/agenttelemetry/impl/defaultProfiles.yaml`. COAT names them
`intake_routing.<name>`. The profile does not send zero values. Thus, an Agent with the feature
disabled sends nothing.

The logs agent restarts that a change causes also show in `logs.restart_attempt`.

Two tests make sure that the names agree on the two sides: `TestTelemetryNames` in this component,
and `TestShippedIntakeRoutingProfile` in `comp/core/agenttelemetry/impl`.

## Logs agent integration

### Eligible endpoints

`comp/logs/agent/config/intake_routing.go` selects the logs endpoints that the logs agent
registers. An endpoint is eligible when all these conditions are true:

- The logs agent uses HTTP. Refer to [Additional endpoints and TCP](#additional-endpoints-and-tcp).
- The endpoint is the main endpoint or one of `logs_config.additional_endpoints`. MRF and
  Observability Pipelines Worker endpoints are not eligible.
- The host is exactly `agent-http-intake.logs.<official site>`, with or without a final dot. The
  host can come from `site` or from an explicit setting such as `logs_dd_url`. Dedicated
  PrivateLink host names, proxies, and custom hosts are not eligible.
- SSL is on, there is no path prefix, the port is 0 or 443, and the endpoint uses the v2 API.

Each eligible endpoint becomes one route with its own API key. Thus, when additional endpoints send
to other organizations or other sites, the component routes each of them separately. Refer to
[Routes and groups](#routes-and-groups).

PrivateLink with private DNS keeps the official host name, so it is eligible. The
[default host path check](#default-host-path-check) keeps it on its default endpoint.

### Additional endpoints and TCP

When `logs_config.additional_endpoints` is set, the logs agent uses TCP, unless
`logs_config.force_use_http` is `true`. This is true also when the additional endpoints are HTTP
intake hosts. In this case, the logs agent also does not try to upgrade to HTTP later.

Intake routing applies only to HTTP. Thus, an Agent with additional endpoints and without
`force_use_http: true` routes no endpoint, not even the main endpoint. To use intake routing
with additional endpoints, set `logs_config.force_use_http: true`.

This component does not change this behavior. Older configurations put TCP intake hosts in
`additional_endpoints`, and a change to HTTP would break them.

### How the logs agent uses the component

- **Start**: the logs agent builds its endpoints and does its HTTP connectivity check. Then it
  registers the eligible endpoints and calls `WaitInitial`. It starts the pipeline on the
  endpoints that `WaitInitial` returned. Then it listens on `Changes()`.
- **Change**: when it gets a signal, the logs agent compares the current endpoints with the
  endpoints that it uses. If they are different, it restarts the pipeline on the new endpoints. It
  uses the partial restart of the TCP to HTTP upgrade. This restart keeps the tailers, the
  auditor, the sources, and the schedulers. If the restart fails, the pipeline goes back to the
  previous endpoints, and the next signal tries again.
- **TCP to HTTP upgrade**: the logs agent registers the new HTTP endpoints and waits for the first
  decision. Then it restarts on the current endpoints.
- **Stop**: the logs agent closes the registration before it stops the pipeline.

The logs agent never changes the endpoints that it built from the configuration. It uses a copy
with the hosts of the overrides. The copy shares the API keys of the configured endpoints, so API
key rotation continues to work.

`agent status` shows the default host of a routed endpoint:
`... (intake routing; default host: agent-http-intake.logs.datadoghq.com.)`.

Serverless builds do not have this component. The logs agent uses empty functions there.

## Code and tests

| Location | Contents |
|---|---|
| `def/` | The component API |
| `impl/intakerouting.go` | Settings, the disabled component, registrations |
| `impl/scheduler.go` | Groups, the background task, decisions, and the schedule of queries |
| `impl/client.go`, `impl/client_http.go` | The query and pre-flight check client, with the provisional wire format |
| `impl/path.go` | The default host path check |
| `impl/validate.go` | Override validation |
| `impl/telemetry.go` | Metrics |
| `fx/` | The Fx module. The core Agent includes it in `cmd/agent/subcommands/run/command.go`. |
| `mock/` | A mock for consumer tests (`SetRoute`, `Requests`) |
| `comp/logs/agent/config/intake_routing.go` | Eligible logs endpoints, and the copy with overrides |
| `comp/logs/agent/impl/agent_intakerouting.go` | Registration and restarts in the logs agent |

The tests use a mock clock, a fake client, and a fake path check. They cover startup, the grace
period, retries, refreshes, decisions, pre-flight checks, groups, API keys, the path check, and
the logs agent restarts.

To do an end-to-end test:

1. Run a fake backend that answers the routing path, `/probe`, and `POST /api/v2/logs`.
2. Send `api.datad0g.com` and an override host such as
   `acme.agent-http-intake.logs.datad0g.com` to the fake backend (with `/etc/hosts` or a proxy).
3. Run the Agent with `site: datad0g.com`, `logs_config.force_use_http: true`,
   `skip_ssl_validation: true`, and `intake_routing: {enabled: true, refresh_interval: 1m}`.

## Other telemetry types

**Metrics** (default forwarder): the forwarder sets its domains when it starts, and its retry queue
is keyed by domain. Routing at startup is possible: create the domain forwarders after
`WaitInitial`. A change at runtime needs a drain and a replacement of each domain forwarder and
its retry queue. Until then, a consumer can ignore `Changes()`, and the new host applies at the
next Agent restart.

**Traces**: the trace-agent is a different process, and each sender has a fixed URL. There are two
possible solutions:

- Run this component in the trace-agent, and build new senders for changes at runtime.
- Keep the queries in the core Agent, and give its decisions to the trace-agent over IPC, with a
  different implementation of the same API. This solution also serves ADP.

**EVP**: it uses the logs endpoint configuration. It needs a pipeline restart in
`comp/forwarder/eventplatform`.

## Trade-offs and risks

- **Startup delay**: when the feature is enabled and the backend is slow, the logs agent can
  wait for the grace period (1 second) after its HTTP connectivity check (5 seconds maximum).
  Other components that start after the logs agent also wait.
- **Each change restarts the logs pipeline**: the pipeline stops for a short time, and the Agent
  can send some logs two times.
- **Diagnostics**: `agent diagnose` and the connectivity checks use the default hosts.
- **Additional endpoints over TCP**: these Agents get no routing. Refer to
  [Additional endpoints and TCP](#additional-endpoints-and-tcp).
- **Not done**: a status section for intake routing, and inventory fields.
- **Release note**: none while the settings are not documented. Add a `features` note when the
  backend API is final.
