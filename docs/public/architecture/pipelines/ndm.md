# Network device monitoring servers

-----

Network Device Monitoring (NDM) is a family of features that all live inside the core agent process: the SNMP check polls devices for metrics and metadata, embedded UDP servers receive NetFlow/IPFIX/sFlow flows and SNMP traps pushed by devices, a path-test collector runs traceroutes toward monitored endpoints, and auxiliary services (device scans, reverse-DNS enrichment, HA failover, network configuration management) support them. Unlike the metrics pipeline, none of this data goes through the aggregator: every NDM product serializes JSON payloads and hands them to the [event platform forwarder](event-platform.md) on a dedicated intake track. This page covers each server and its data flow; the shared batching/retry machinery is documented on the [event platform](event-platform.md) page.

## The five intake tracks

Each NDM event type maps to a passthrough pipeline declared in [`comp/forwarder/eventplatform/impl`](<<<SRC>>>/comp/forwarder/eventplatform/impl). The track type selects the backend processing pipeline; the hostname prefix selects the intake domain (`<prefix><site>`).

| Track | Event type | Producer | Intake host prefix | Endpoint config prefix |
|---|---|---|---|---|
| `ndm` | `EventTypeNetworkDevicesMetadata` | SNMP check metadata, device scans, NetFlow exporter metadata | `ndm-intake.` | `network_devices.metadata.` |
| `ndmflow` | `EventTypeNetworkDevicesNetFlow` | NetFlow server | `ndmflow-intake.` | `network_devices.netflow.forwarder.` |
| `ndmtraps` | `EventTypeSnmpTraps` | SNMP traps server | `snmp-traps-intake.` | `network_devices.snmp_traps.forwarder.` |
| `netpath` | `EventTypeNetworkPath` | Network path collector | `netpath-intake.` | `network_path.forwarder.` |
| `ndmconfig` | `EventTypeNetworkConfigManagement` | Network configuration management | `ndm-intake.` | `network_devices.config_management.forwarder.` |

The pipeline descriptors live in [`pipelines_ndm_core.go`](<<<SRC>>>/comp/forwarder/eventplatform/impl/pipelines_ndm_core.go), [`pipelines_ndm_integrations.go`](<<<SRC>>>/comp/forwarder/eventplatform/impl/pipelines_ndm_integrations.go), and [`pipelines_networkpath.go`](<<<SRC>>>/comp/forwarder/eventplatform/impl/pipelines_networkpath.go). All devices share a namespace (`network_devices.namespace`, default `default`) that disambiguates identical IPs across environments; device IDs are `<namespace>:<ip>`.

```text
                            core agent process
          +---------------------------------------------------------+
 devices  |  SNMP corecheck ----+                                   |
 <------- |  snmpscan(manager) -+--> sender.EventPlatformEvent --+  |
 UDP 161  |  NCM (SSH 22) ------+                                |  |
          |                                                      v  |
 devices  |  netflow server --> FlowAggregator ---> event platform  |    HTTPS
 -------> |  (UDP 2055/4739/6343)                     forwarder ----+--> ndm / ndmflow /
          |                                              ^          |    ndmtraps / netpath /
 devices  |  snmptraps server --> formatter/forwarder ---+          |    ndmconfig intakes
 -------> |  (UDP 9162)                                  |          |
          |  npcollector --> traceroute (remote client) -+          |
          +----------------------|----------------------------------+
                                 | HTTP over system-probe socket
                                 v
                     system-probe traceroute module
```

## Key packages

| Path | Purpose |
|---|---|
| [`pkg/collector/corechecks/snmp`](<<<SRC>>>/pkg/collector/corechecks/snmp) | SNMP corecheck: device polling, profiles, metadata |
| [`comp/core/autodiscovery/listeners/snmp.go`](<<<SRC>>>/comp/core/autodiscovery/listeners/snmp.go) | SNMP autodiscovery listener (subnet scanning) |
| [`pkg/networkdevice/metadata`](<<<SRC>>>/pkg/networkdevice/metadata) | `NetworkDevicesMetadata` payload types and batching |
| [`comp/netflow`](<<<SRC>>>/comp/netflow) | NetFlow/IPFIX/sFlow server and flow aggregator |
| [`comp/snmptraps`](<<<SRC>>>/comp/snmptraps) | SNMP traps server (listener, OID resolver, formatter, forwarder) |
| [`comp/networkpath`](<<<SRC>>>/comp/networkpath) | Path-test collector (`npcollector`) and traceroute component |
| [`cmd/system-probe/modules/traceroute.go`](<<<SRC>>>/cmd/system-probe/modules/traceroute.go) | system-probe module that executes traceroutes |
| [`comp/rdnsquerier`](<<<SRC>>>/comp/rdnsquerier) | Rate-limited, cached reverse-DNS enrichment |
| [`comp/haagent`](<<<SRC>>>/comp/haagent) | Active/standby agent failover for device-polling checks |
| [`comp/snmpscan`](<<<SRC>>>/comp/snmpscan) / [`comp/snmpscanmanager`](<<<SRC>>>/comp/snmpscanmanager) | Full-device OID scans (on-demand and automatic) |
| [`comp/networkconfigmanagement`](<<<SRC>>>/comp/networkconfigmanagement) | Network configuration management (NCM) over SSH |
| [`comp/ndmtmp`](<<<SRC>>>/comp/ndmtmp) | Shim exposing the event platform forwarder as a component |

## SNMP check and autodiscovery

The SNMP corecheck ([`snmp.go`](<<<SRC>>>/pkg/collector/corechecks/snmp/snmp.go), registered in [`pkg/commonchecks/corechecks.go`](<<<SRC>>>/pkg/commonchecks/corechecks.go)) is the pull side of NDM. It is a regular [Go core check](../checks/corechecks.md) with two modes: a single-device instance (`ip_address`) builds one `devicecheck.DeviceCheck`, while a discovery instance (`network`: CIDR) starts a background [`discovery`](<<<SRC>>>/pkg/collector/corechecks/snmp/internal/discovery/discovery.go) loop that scans the range and fans discovered devices out to `workers` goroutines on each run.

[`DeviceCheck.Run`](<<<SRC>>>/pkg/collector/corechecks/snmp/internal/devicecheck/devicecheck.go) fetches scalar and column OIDs in batches, converts values to metrics using profile definitions, emits the `snmp.can_check` service check and `snmp.device.reachable`/`unreachable` metrics, and optionally pings the device (`networkdevice.ping.*` via [`pkg/networkdevice/pinger`](<<<SRC>>>/pkg/networkdevice/pinger) — on Linux an in-process UDP-socket ping by default, delegating to the system-probe ping module when `ping.linux.use_raw_socket` is set; on Windows an in-process raw socket). Profiles are resolved per device from its sysObjectID, in priority order: inline profile in the instance, user YAML under `<confd_path>/snmp.d/profiles/`, bundled defaults, or — with `use_remote_config_profiles` — the [Remote Config](../configuration/remote-config.md) product `NDM_DEVICE_PROFILES_CUSTOM` served through the singleton `UpdatableProvider` in [`internal/profile`](<<<SRC>>>/pkg/collector/corechecks/snmp/internal/profile/rc_provider.go). The profile cache refreshes every 600 seconds or when the sysObjectID changes.

Every run also builds device, interface, IP address, LLDP/CDP topology-link, and optionally VPN-tunnel metadata; [`report_device_metadata.go`](<<<SRC>>>/pkg/collector/corechecks/snmp/internal/report/report_device_metadata.go) batches them 100 resources per payload ([`BatchPayloads`](<<<SRC>>>/pkg/networkdevice/metadata/payload_utils.go)) and sends each as an `EventTypeNetworkDevicesMetadata` event through the check sender. With `use_device_id_as_hostname` the device ID replaces the agent hostname on the device's metrics, registered as external host tags via [`pkg/collector/externalhost`](<<<SRC>>>/pkg/collector/externalhost).

Autodiscovery of devices happens one level up from the check: `SNMPListener` ([`listeners/snmp.go`](<<<SRC>>>/comp/core/autodiscovery/listeners/snmp.go)) is an [Autodiscovery](../checks/autodiscovery.md) service listener activated by the `network_devices.autodiscovery` config section (legacy alias `snmp_listener`; parsing in [`pkg/snmp/snmp.go`](<<<SRC>>>/pkg/snmp/snmp.go)). For each configured subnet, worker goroutines (default 2) walk every IP, trying each entry of the `authentications` list until a reachability probe (an SNMP GetNext on OID `1.0`) succeeds. Each discovered device becomes an AD service matching the templated `snmp` auto-conf, so the collector schedules one check instance per device. A device that stops answering is only dropped after `discovery_allowed_failures` (default 3) consecutive failures. Discovery state is persisted per subnet with [`pkg/persistentcache`](<<<SRC>>>/pkg/persistentcache) so restarts do not rescan from scratch, and, when `network_devices.autodiscovery.use_deduplication` is enabled (default `false`), [`devicededuper`](<<<SRC>>>/pkg/snmp/devicededuper/device_deduper.go) recognizes the same physical device answering on multiple IPs (equal name, description, sysObjectID, and boot time within 5 seconds) and keeps only the lowest IP.

## NetFlow server

[`comp/netflow`](<<<SRC>>>/comp/netflow) turns the core agent into a flow collector. It is enabled by `network_devices.netflow.enabled` and wired only into the core agent run command ([`command.go`](<<<SRC>>>/cmd/agent/subcommands/run/command.go), `netflow.Bundle()`); when disabled, a stub component costs nothing.

[`Server.Start`](<<<SRC>>>/comp/netflow/server/impl/server.go) launches one [goflow2](https://github.com/netsampler/goflow2) listener per entry in `network_devices.netflow.listeners` via [`goflowlib.StartFlowRoutine`](<<<SRC>>>/comp/netflow/goflowlib/flowstate.go). Supported `flow_type` values and default UDP ports: `netflow5` and `netflow9` on 2055, `ipfix` on 4739, `sflow5` on 6343, binding `0.0.0.0` by default with per-listener `workers`, `namespace`, and an optional `mapping` list that makes a custom decoder ([`netflowstate`](<<<SRC>>>/comp/netflow/goflowlib/netflowstate)) capture extra NetFlow/IPFIX fields. Decoded flows are converted ([`convert.go`](<<<SRC>>>/comp/netflow/goflowlib/convert.go)) to the internal `common.Flow` and pushed into the `FlowAggregator` input channel (`aggregator_buffer_size`, default 10000).

The [`FlowAggregator`](<<<SRC>>>/comp/netflow/flowaggregator/aggregator.go) pipeline applies three mechanisms before anything reaches the intake:

1. **Aggregation** ([`flowaccumulator.go`](<<<SRC>>>/comp/netflow/flowaggregator/flowaccumulator.go)): flows are merged by an FNV hash of namespace, exporter, source/destination addresses, ports, protocol, TOS, and input interface, accumulating bytes/packets for `aggregator_flush_interval` (default 300 s). Flow contexts linger one extra interval so counters survive across windows.
1. **Port rollup** ([`portrollup`](<<<SRC>>>/comp/netflow/portrollup/portrollup.go)): once an endpoint pair exceeds `aggregator_port_rollup_threshold` (default 10) distinct ports on one side, the ephemeral side's port is replaced with `-1`, collapsing scanner-style traffic into a single flow. The tracking store is two generations deep and swaps every 5 minutes.
1. **Top-N capping** ([`topn`](<<<SRC>>>/comp/netflow/topn)): if `aggregator_max_flows_per_flush_interval` is set, a min-heap over bytes keeps only the largest flows per period and a jittered throttler spreads flushes.

The flush loop ticks every 10 seconds (hardcoded) but a given flow context only flushes when its own `aggregator_flush_interval` elapses. Flushed flows are built into JSON payloads ([`payload.go`](<<<SRC>>>/comp/netflow/payload/payload.go)) and sent one event per flow with `SendEventPlatformEventBlocking` on the `ndmflow` track. The aggregator also emits **exporter metadata** (`NetflowExporter`) on the `ndm` metadata track so flow exporters appear as devices in the product, tracks per-exporter sequence numbers (large negative deltas count as sequence resets), and publishes rich telemetry under `datadog.netflow.*`, including goflow2's Prometheus metrics republished through [`metric.go`](<<<SRC>>>/comp/netflow/goflowlib/metric.go). If `network_path.netflow_monitoring.enabled` is on, every flushed flow is offered to the network path collector for tracerouting (see below).

## SNMP traps server

[`comp/snmptraps`](<<<SRC>>>/comp/snmptraps) receives asynchronous SNMP v1/v2c/v3 traps. It is enabled by `network_devices.snmp_traps.enabled` and hosted only in the core agent ([`command_snmptraps.go`](<<<SRC>>>/cmd/agent/subcommands/run/command_snmptraps.go)). Internally the [server component](<<<SRC>>>/comp/snmptraps/server/impl/server.go) builds a **nested `fx.App`** hosting its sub-components (config, OID resolver, formatter, listener, forwarder) — an acknowledged anti-pattern (see the TODO in the code); a startup failure is captured into a status provider instead of failing the agent, so a broken traps configuration is only visible in `agent status`.

The [listener](<<<SRC>>>/comp/snmptraps/listener/impl/listener.go) runs a [gosnmp](https://github.com/gosnmp/gosnmp) `TrapListener` on UDP `bind_host:port` (default `0.0.0.0:9162`). Credentials come from [`TrapsConfig`](<<<SRC>>>/comp/snmptraps/config/def/config.go): with no v3 `users` the listener runs in plain v2c mode; with `users` it runs gosnmp in `Version3` mode with a USM security-parameters table, which deliberately remains compatible with v1/v2c traps at the same time. v1/v2c packets are validated against `community_strings` with a constant-time comparison; rejects increment `datadog.snmp_traps.invalid_packet`. The v3 authoritative engine ID is deterministic — `0x80 0xff 0xff 0xff 0xff` followed by an FNV-128 hash of the agent hostname — so renaming the host silently breaks v3 senders configured against the old engine ID.

Accepted packets flow through a channel (size 100) to the [forwarder](<<<SRC>>>/comp/snmptraps/forwarder/impl/forwarder.go), which calls the [formatter](<<<SRC>>>/comp/snmptraps/formatter/impl/formatter.go) and sends the result as an `EventTypeSnmpTraps` event on the `ndmtraps` track. The formatter resolves trap OIDs to names and enum-mapped values using the [OID resolver](<<<SRC>>>/comp/snmptraps/oidresolver/impl/oid_resolver.go), which loads trap database files (JSON/YAML, optionally gzipped) from `<confd_path>/snmp.d/traps_db/`; user-provided files win over the Datadog-shipped `dd_traps_db*` database on conflicts.

## SNMP device scans

[`comp/snmpscan`](<<<SRC>>>/comp/snmpscan) executes full OID walks of a device: [`ScanDeviceAndSendData`](<<<SRC>>>/comp/snmpscan/impl/devicescan.go) emits `ScanStatusMetadata` (in progress / completed / error) and batches of raw `DeviceOID` values as metadata events on the `ndm` track. Scans are triggered three ways: from the backend via Remote Config agent tasks of type `TaskDeviceScan`, from the CLI (`agent snmp walk <ip>` and `agent snmp scan <ip>`, [`command.go`](<<<SRC>>>/cmd/agent/subcommands/snmp/command.go)), and automatically by the scan manager.

[`comp/snmpscanmanager`](<<<SRC>>>/comp/snmpscanmanager/impl/snmpscanmanager.go) orchestrates **default scans** (`network_devices.default_scan.enabled`, default `true`): every SNMP check `Configure` of a single-device instance requests a scan for its device. The manager dedupes by IP, keeps a queue of 10000 with 2 workers, throttles to 8 SNMP calls per second and 100000 calls per scan, skips `network_devices.default_scan.excluded_ips`, persists completed scans in a persistent cache so they survive restarts, retries connection failures with a growing backoff (1 h up to 1 week), and rescans each device roughly every 6 months with jitter.

A subtle detail: scan credentials are **not** re-parsed from config files. The manager (and the `agent snmp` CLI when credentials are not passed on the command line) asks the running agent over its [IPC API](../processes/ipc.md) — [`snmpparse.GetParamsFromAgent`](<<<SRC>>>/pkg/snmp/snmpparse/config_snmp.go) calls the authenticated `GET /agent/config-check?raw=true` endpoint and matches the device IP against live SNMP check instances. If the IPC API is unreachable (auth token mismatch, agent down), scans fail even though the check itself runs fine.

## Network path

[`comp/networkpath`](<<<SRC>>>/comp/networkpath) produces "path tests": traceroutes toward monitored endpoints, sent as `EventTypeNetworkPath` events on the `netpath` track. Path tests originate from three sources:

1. **Static**: the `network_path` corecheck ([`networkpath.go`](<<<SRC>>>/pkg/collector/corechecks/networkpath/networkpath.go)) runs one traceroute per `conf.d` instance per run (origin `network_path_integration`).
1. **NPM connections**: with `network_path.connections_monitoring.enabled`, the connections check in the process-agent ([`net.go`](<<<SRC>>>/pkg/process/checks/net.go)) and the system-probe direct connections sender ([`sender_linux.go`](<<<SRC>>>/pkg/network/sender/sender_linux.go)) feed observed outgoing connections to the collector (origin `network_traffic`). See [network monitoring](../ebpf/network-monitoring.md).
1. **NetFlow**: with `network_path.netflow_monitoring.enabled`, the NetFlow aggregator schedules flushed flows (origin `netflow`), skipping flows sourced from the agent host's own IPs ([`localips.go`](<<<SRC>>>/comp/networkpath/npcollector/impl/localips.go)).

The scheduler, [`npcollector`](<<<SRC>>>/comp/networkpath/npcollector/impl/npcollector.go), filters incoming connections aggressively: intra-host and incoming connections, system-probe's own connections, IPv6 destinations without an observed DNS name (from NPM's DNS data), intra-VPC traffic when `disable_intra_vpc_collection` is set (VPC subnets fetched from cloud metadata via [`pkg/util/cloudproviders/network`](<<<SRC>>>/pkg/util/cloudproviders/network)), plus CIDR-based `source_excludes`/`dest_excludes` and domain/IP filters. Survivors land in the [`pathteststore`](<<<SRC>>>/comp/networkpath/npcollector/impl/pathteststore/pathteststore.go), which dedupes per pathtest context (a hash of hostname, port, protocol, origin, namespace, and source container ID), caps contexts at `pathtest_contexts_limit` (1000), re-runs each context every `pathtest_interval` (30 m) for `pathtest_ttl` (70 m), and rate-limits to `pathtest_max_per_minute` (150). A 10-second flush loop feeds `workers` (default 4) that execute the traceroute and send the resulting `NetworkPath` payload; destination and hop IPs are reverse-DNS enriched when `network_path.collector.reverse_dns_enrichment.enabled` (default `true`). All tunables live in [`config.go`](<<<SRC>>>/comp/networkpath/npcollector/impl/config.go) under `network_path.collector.*`.

### Traceroute execution: always in system-probe

The traceroute [component](<<<SRC>>>/comp/networkpath/traceroute/def/component.go) has two implementations:

1. [`impl-local`](<<<SRC>>>/comp/networkpath/traceroute/impl-local/traceroute.go) wraps the runner from the external [datadog-traceroute](https://github.com/DataDog/datadog-traceroute) library and is loaded **only in system-probe** ([`command.go`](<<<SRC>>>/cmd/system-probe/subcommands/run/command.go)).
1. [`impl-remote`](<<<SRC>>>/comp/networkpath/traceroute/impl-remote/traceroute.go) is an HTTP client that calls `GET /traceroute/{host}?...` over the system-probe socket. It is loaded in the core agent, process-agent, cluster-agent, and private action runner.

The [system-probe traceroute module](<<<SRC>>>/cmd/system-probe/modules/traceroute.go) (config key `traceroute.enabled` in `system-probe.yaml`, default `false` — enabling a network path feature on the agent side does **not** enable it) parses query parameters, runs the local implementation, and returns JSON or a structured error with user-facing messages. On Windows it manages a kernel driver lifecycle (opt out with `network_path.collector.disable_windows_driver`). The consequence: **traceroutes never execute in the core agent process** — even the `network_path` corecheck delegates over the socket, and "check that the traceroute module is enabled in the system-probe.yaml" errors are the canonical symptom of missing [system-probe](../ebpf/system-probe.md) wiring. Whichever process hosts a scheduling source runs its own `npcollector` instance (core agent, process-agent, and system-probe all can), but they all converge on the same module.

## Reverse DNS querier

[`comp/rdnsquerier`](<<<SRC>>>/comp/rdnsquerier/impl/rdnsquerier.go) enriches IPs with hostnames for NetFlow and network path payloads. The real implementation is only built when NetFlow rDNS (`network_devices.netflow.reverse_dns_enrichment_enabled`) or network path rDNS is enabled; otherwise the fx module yields a [noop](<<<SRC>>>/comp/rdnsquerier/impl-none/none.go). Because the **same instance is shared** by both consumers, each consumer additionally swaps in its own local noop when its own sub-flag is off — a pattern any new consumer must copy.

Two properties are easy to trip over. First, it resolves **private IPs only** (`netip.Addr.IsPrivate`); public IPs return an empty hostname immediately, by design (PII and resolver load). Second, everything is bounded: a channel (`reverse_dns_enrichment.chan_size`, default 5000) feeds `workers` (default 10) calling `net.LookupAddr` behind an adaptive rate limiter (default 1000 qps, throttling to 1 qps after 10 consecutive errors, with gradual recovery) and a cache (default on: 24 h TTL, 1 M entries, persisted to disk every 2 h via `pkg/persistentcache`). The [component API](<<<SRC>>>/comp/rdnsquerier/def/component.go) offers `GetHostnameAsync` (channel-based, used by NetFlow) and blocking `GetHostname`/`GetHostnames` with timeouts (used by `npcollector`).

## HA agent

[`comp/haagent`](<<<SRC>>>/comp/haagent/impl/haagent.go) implements active/standby failover for device-polling checks, so two agents can monitor the same devices without double-polling. Configuration is `ha_agent.enabled` plus a mandatory top-level `config_id`; agents sharing a `config_id` form a failover group. The backend elects a leader and pushes it through the Remote Config product `HA_AGENT`; `onHaAgentUpdate` compares the pushed `active_agent` hostname with the local hostname and sets the state to `active` or `standby` (initial state `unknown`; an empty update resets to `unknown`).

Enforcement lives in the check runner, not the checks: the [check worker](<<<SRC>>>/pkg/collector/worker/worker.go) skips a scheduled run when HA is enabled, the check reports `IsHASupported() == true`, and the local agent is not active. HA-supported checks today are the `snmp`, `oracle`, `network_path`, `cisco_sdwan`, and `versa` corechecks plus Python checks that declare support. Everything else runs normally on standby agents. If `config_id` is missing, the component logs one error and silently disables itself (`Enabled()` returns `false`). The module is loaded in the core agent and the [cluster agent](../containers/cluster-agent.md) (gating cluster-check runners).

## ndmtmp

[`comp/ndmtmp`](<<<SRC>>>/comp/ndmtmp/forwarder/impl/forwarder.go) is a one-function shim: its `forwarder` component **is** the `eventplatform.Forwarder`, obtained by calling `demultiplexer.GetEventPlatformForwarder()`. It exists so NDM components (the NetFlow server, for one) can depend on the event platform forwarder without a direct demultiplexer dependency; the "tmp" in the name marks it for dissolution once the event platform forwarder is a first-class dependency everywhere.

## Network configuration management

[`comp/networkconfigmanagement`](<<<SRC>>>/comp/networkconfigmanagement/impl) (NCM) retrieves, versions, and can restore network device configurations. The `network_config_management` corecheck ([`networkconfigmanagement.go`](<<<SRC>>>/pkg/collector/corechecks/networkconfigmanagement/networkconfigmanagement.go), default interval 15 m) registers its device with the component and calls `ReportConfig` each run: the component connects to the device **over SSH** ([`pkg/networkconfigmanagement/remote`](<<<SRC>>>/pkg/networkconfigmanagement/remote/ssh.go)), executes vendor-profile commands ([`profile`](<<<SRC>>>/pkg/networkconfigmanagement/profile)) to fetch the running and startup configurations, normalizes and redacts them, and emits `EventTypeNetworkConfigManagement` events on the `ndmconfig` track.

With `network_devices.config_management.rollback.enabled`, retrieved configs are also stored locally in a bbolt database at `<run_path>/ncm_config.db` ([`store`](<<<SRC>>>/pkg/networkconfigmanagement/store)), and the component registers two authenticated agent-API endpoints ([`types.go`](<<<SRC>>>/comp/networkconfigmanagement/impl/types.go)): `GET /agent/ncm/config?uuid=...` to inspect stored configs and `POST /agent/ncm/rollback` to push a saved config back to the device over SSH. NCM is compiled only under the `ncm` build tag (part of default agent builds); without it, a [stub](<<<SRC>>>/comp/networkconfigmanagement/stub/stub.go) is provided and the corecheck is inert. If the local DB cannot be opened, rollback silently degrades to no-rollback mode.

## Adjacent: synthetics test scheduler

[`comp/syntheticstestscheduler`](<<<SRC>>>/comp/syntheticstestscheduler/impl/scheduler.go) (enabled by `synthetics.collector.enabled`, hosted in the core agent) is owned by the synthetics product, not NDM, but reuses the same plumbing: it polls synthetics network-test configurations from the Datadog API, executes tests through the same remote traceroute component (hence system-probe), evaluates assertions, and sends results to the event platform.

## Configuration summary

All keys live in `datadog.yaml` and accept `DD_` environment-variable overrides. The most consequential ones:

| Key | Default | Governs |
|---|---|---|
| `network_devices.namespace` | `default` | Device namespace shared by all NDM products |
| `network_devices.autodiscovery.*` | — | Subnet scanning (`configs[]`, `workers`, `discovery_interval`, `discovery_allowed_failures`); legacy alias `snmp_listener` |
| `network_devices.default_scan.enabled` | `true` | Automatic full scans of monitored devices |
| `network_devices.snmp_traps.enabled` / `.port` / `.bind_host` | `false` / 9162 / `0.0.0.0` | Traps server |
| `network_devices.snmp_traps.community_strings` / `.users` | — | v1/v2c and v3 trap credentials |
| `network_devices.netflow.enabled` / `.listeners[]` | `false` / — | NetFlow server; per-listener `flow_type`, `port`, `bind_host`, `workers`, `mapping` |
| `network_devices.netflow.aggregator_flush_interval` | 300 s | Flow aggregation window |
| `network_devices.netflow.aggregator_port_rollup_threshold` | 10 | Ephemeral-port rollup trigger |
| `network_devices.netflow.reverse_dns_enrichment_enabled` | `false` | rDNS on flows |
| `network_path.connections_monitoring.enabled` | `false` | Path tests from NPM connections |
| `network_path.netflow_monitoring.enabled` | `false` | Path tests from NetFlow flows |
| `network_path.collector.*` | see [`config.go`](<<<SRC>>>/comp/networkpath/npcollector/impl/config.go) | Workers, pathtest interval/TTL/limits, filters, traceroute technique |
| `reverse_dns_enrichment.*` | workers 10, chan 5000, cache 24 h | rdnsquerier tuning |
| `ha_agent.enabled` + `config_id` | `false` / `''` | HA failover group |
| `network_devices.config_management.rollback.enabled` | `false` | NCM local rollback store and API endpoints |
| system-probe: `traceroute.enabled`, `ping.enabled` | `false` / `false` | system-probe modules backing network path and device ping |

## Deployment modes

- **Host install**: everything runs in the main `datadog-agent` service. Network path traceroutes additionally require system-probe with the `traceroute` module explicitly enabled in `system-probe.yaml` (default `false`); Linux device pings need the `ping` module only when `use_raw_socket` is set. Windows traceroute uses a system-probe-managed kernel driver; Windows device ping uses an in-process raw socket.
- **Docker / Kubernetes DaemonSet**: NetFlow and traps listeners bind UDP inside the node-agent container, so the Helm chart or manifest must expose `hostPort`s (or host networking) for 2055/4739/6343/9162 for devices to reach them. The `network_devices` config sections are node-agent-only (tagged `full-agent-only` in the schema). SNMP checks can be dispatched as [cluster checks](../containers/cluster-checks.md) to cluster-check runners, but the IPC credential lookup used by default scans and the `agent snmp` CLI only queries the local agent, so scan semantics differ there.
- **Cluster agent**: runs no NetFlow/traps/SNMP servers; it loads only the HA agent module (to gate cluster-check runners) and the remote traceroute client.
- **process-agent**: hosts its own `npcollector` and remote traceroute client for connection-driven path tests.
- **system-probe**: hosts the only local traceroute implementation, plus its own `npcollector` (for the Linux direct connections sender) and rdnsquerier.
- **Fargate / serverless**: none of these features run (no full node agent, no system-probe).

## Ports and IPC

| Channel | Direction | Details |
|---|---|---|
| UDP 2055 / 4739 / 6343 (configurable) | inbound to core agent | NetFlow5+9 / IPFIX / sFlow5 listeners |
| UDP 9162 (configurable) | inbound to core agent | SNMP trap listener |
| UDP 161 (per device) | core agent to devices | SNMP polling (check, discovery, scans) |
| SSH 22 (per device) | core agent to devices | NCM config retrieval and rollback |
| system-probe socket | agent / process-agent / cluster-agent to system-probe | `GET /traceroute/{host}`, ping module; HTTP over Unix socket (Linux) or named pipe (Windows) |
| Agent IPC API (localhost `cmd_port`, TLS + bearer token) | CLI and self | `/agent/config-check` (scan credential lookup), `/agent/ncm/config`, `/agent/ncm/rollback` |
| `localhost:9090` (opt-in) | local scrape | Raw goflow2 Prometheus metrics (`prometheus_listener_enabled`) |
| HTTPS intakes | outbound | The five event platform tracks (see table above), via the [event platform forwarder](event-platform.md) |
| Remote Config | backend to agent | `NDM_DEVICE_PROFILES_CUSTOM` (SNMP profiles), `HA_AGENT` (leadership), `TaskDeviceScan` agent tasks |

## Gotchas

- The traps server is a nested fx application inside the component; adding dependencies there risks double instantiation, and startup failures are swallowed into a status provider — the agent starts fine and the breakage is only visible in `agent status`.
- The traps v3 engine ID is derived from the agent hostname; renaming the host changes the engine ID and breaks v3 trap senders configured against it.
- NetFlow9/IPFIX templates are held in memory only — after an agent restart, flows from an exporter are undecodable until it re-advertises its templates.
- NetFlow flushes with `SendEventPlatformEventBlocking`, so when the `ndmflow` pipeline's input channel (default 10000) fills during large flushes the flush loop blocks instead of dropping; while it is blocked the aggregator stops draining its own input channel (`aggregator_buffer_size`, default 10000) and inbound flows back up in goflow2. Raise `network_devices.netflow.forwarder.input_chan_size` if `datadog.netflow.aggregator.input_buffer.*` telemetry shows sustained pressure. The flush cadence (10 s ticks) is distinct from the aggregation window (`aggregator_flush_interval`).
- NetFlow sequence-delta metrics are only accurate with one observation domain per exporter IP.
- rdnsquerier resolves private IPs only; consumers must never expect public reverse DNS. Its single shared instance means each consumer must wrap it with a local noop when that consumer's own flag is off.
- The HA agent silently disables itself when `config_id` is missing, and standby agents still run all non-HA checks — only `IsHASupported()` checks are gated.
- Default scans piggyback on check `Configure` and fetch credentials through the agent's own HTTP API; an unreachable IPC API (bad auth token) breaks scans while the check keeps working.
- The SNMP corecheck derives its check ID from the device ID (`snmp:<device_id>:<hash>`) by mutating the raw instance config — relevant when correlating check IDs in status output.
- Only IPv4 connections are eligible for dynamic path tests unless a DNS name was observed for the destination, and `npcollector` drops pathtests silently (metric only: `datadog.network_path.collector.*.pathtest_dropped`) when its channels fill.
- SNMP autodiscovery persists discovered devices per subnet; removing a subnet from the config does not purge its persistent cache file. The device deduper tolerates 5 seconds of boot-time skew, so devices with unstable sysUpTime can flap dedup decisions.
