# SUSM-164 Investigation: USM Metrics Have Inaccurate (Low) Hit Count Compared to Logs

## Issue Summary

Customer reports under-represented hit counts from USM metrics compared to Istio sidecar logs.

**Examples from the Jira ticket:**
- Service `lendsvcs-npbcollateralevaluation`: 79 USM hits vs 485 Istio logs (Jan 21 2:03pm – Jan 22 5:25am)
- Service `customersrvcsrvcs-readstrcccusts` endpoint `/credit-cards/readstore-customers/v1/card-holder-email-address-details`: 171 USM hits vs 32K Istio logs (3-day window)

**Customer environment:**
- Agent v7.73.0
- Kernel 5.15.0-1102-azure (Ubuntu 22.04, AKS)
- Cilium eBPF dataplane (`nodeLabels_kubernetes.azure.com/ebpf-dataplane:cilium`)
- Istio mTLS enabled (`security.istio.io/tlsMode: istio`)
- Clusters: `aks-cus-gen-prod-002`, `gen-obc-k8-uat2`

**Jira:** https://datadoghq.atlassian.net/browse/SUSM-164
**Related:** SUSM-149 (different customer, possibly similar symptoms)

---

## Agent Flare Analysis

Flare from node `aks-userpool3-24857875-vmss00003g` in cluster `aks-cus-gen-prod-002`.

### Configuration (system_probe_runtime_config_dump.yaml)

```yaml
service_monitoring_config:
  enabled: true
  enable_http_monitoring: true
  enable_http2_monitoring: true
  enable_ring_buffers: true
  enable_event_stream: true
  http:
    use_direct_consumer: false    # not enabled
    max_tracked_connections: 1024
    max_stats_buffered: 100000
    notification_threshold: 512
  tls:
    go:
      enabled: true
    istio:
      enabled: true
      envoy_path: /bin/envoy      # default; actual binary is at /usr/local/bin/envoy
    native:
      enabled: true
    nodejs.enabled: "true"
```

### Telemetry Counters (system_probe_telemetry.log)

Agent uptime at flare time: 3h50m (started 12:46:38 UTC, flare at 16:36:16 UTC).

**HTTP/1.x:**
| Metric | Value |
|--------|-------|
| `usm__http__aggregations` | 221,234 |
| `usm__http__connections` | 209,521 |
| `usm__http__joiner__requests` | 616 |
| `usm__http__joiner__responses` | 6,021 |
| `usm__http__joiner__joined` | 545 |
| `usm__http__joiner__aged` | 71 |
| `usm__http__joiner__responses_dropped` | 15 |

**HTTP/2 (encrypted via Istio hooks):**
| Metric | Value |
|--------|-------|
| `usm__http2__aggregations` | 386 |
| `usm__http2__connections` | 155 |
| `usm__http2__requests{encrypted="true"}` | 711,045 |
| `usm__http2__responses{encrypted="true"}` | 717,289 |
| `usm__http2__eos{encrypted="true"}` | 1,440,779 |

**Joiner analysis:** 5,461 orphaned responses (6,021 - 545 joined - 15 dropped) — these are HTTP/1 responses USM captured but could never match to a request.

**Map cleaner:**
| Map | Entries cleaned |
|-----|----------------|
| `http_in_flight` | 5,170 |
| `http2_in_flight` | 1 |
| `http2_dynamic_table` | 384 |
| `connection_protocol` | 5,997 |
| `tls_enhanced_tags` | 25,986 |

### Traced Programs (expvar/system-probe)

- **26 envoy processes** traced with `ProgramType: istio` (all at `/usr/local/bin/envoy`)
- **34 processes** traced with `ProgramType: usm_tls` (various libcrypto/libssl)
- **14 Go processes** traced with `ProgramType: go-tls`
- **1 Node.js process** traced with `ProgramType: nodejs`
- Envoy processes also appear in the blocked list as `not-go` (for go-tls) and `no-symbols` for pilot-agent
- `cilium-envoy` at `/usr/bin/cilium-envoy` is blocked (not hooked)

### System-Probe Logs

- All USM protocols enabled successfully: HTTP, HTTP2, Kafka, openssl, go-tls, istio, nodejs
- No errors related to USM data loss, perf buffer overflow, or map full conditions
- Warnings are benign: short-lived process attach failures, "path already registered" for duplicate hooking attempts

### dmesg

- **AppArmor denials**: `cri-containerd.apparmor.d` denying `ptrace` read for both `agent` (PID 3782934) and `process-agent` (PID 3783047). Happens continuously (~every 10 seconds). This may affect process metadata resolution but does not affect eBPF data capture.

### Process-Agent Logs

- Connections check runs every 30 seconds, completing in ~160-180ms
- Payloads successfully posted to `https://process.datadoghq.com./api/v1/connections`
- No errors in HTTP stats encoding or delivery

### Discovery Log

Empty (`{}`). Possibly related to AppArmor ptrace denials.

---

## Backend Intake Analysis (procq dumps)

### Dump from `gen-obc-k8-uat2-worker-userpool1-tf11` (dump_1773588474)

1000 messages, last hour. Target endpoint: `POST /credit-cards/readstore-customers/v1/card-holder-email-address-details`.

**Matching connections found:**

```
[HTTP/1] pid=220790 10.42.16.142:15006 -> 10.42.10.135:46210
  method=Post path=/credit-cards/readstore-customers/v1/card-holder-demographic-details hits=1
  status=404 count=1
  direction=incoming
  protocol_stack=[protocolTLS protocolHTTP]
  conn_tags=[tls.cipher_suite_id:0x1301 tls.client_version:tls_1.2 tls.client_version:tls_1.3 tls.library:istio tls.version:tls_1.3]
  container_tags=<empty>  (resolved later by network-resolver)

[HTTP/1] pid=220790 10.42.16.142:15006 -> 10.42.10.135:51318
  method=Post path=/credit-cards/readstore-customers/v1/card-holder-address-details hits=2
  status=404 count=2
  (same tags as above)

[HTTP/1] pid=220790 10.42.16.142:15006 -> 10.42.21.229:58150
  method=Post path=/credit-cards/readstore-customers/v1/card-holder-email-address-details hits=1
  status=404 count=1
  (same tags as above)
```

**Key findings:**
- Traffic IS captured via Istio TLS hooks — tagged `tls.library:istio`, `tls.version:tls_1.3`
- Protocol stack is `[protocolTLS, protocolHTTP]` — HTTP/1.1 inside the mTLS tunnel (NOT HTTP/2)
- Connections are on **pod IPs** (10.42.16.142:15006 → remote), direction incoming
- Port 15006 is Istio's inbound listener (iptables redirects original port 8080 → 15006)
- Container ID is present but container tags are empty (resolved downstream by network-resolver)
- **4 total hits captured** across 3 connections

### Dump from `aks-userpool2-97713605-vmss00000c` (dump_1773587778)

1000 messages. General traffic analysis (not the target endpoint):

| Category | Connections | Total Hits |
|----------|-------------|------------|
| HTTP/1 | 178,781 | 185,918 |
| HTTP/2 | 0 | 0 |

**Notable:** Zero HTTP/2 data in the entire dump despite eBPF telemetry showing 711K HTTP/2 requests. This means the HTTP/2 captured by Istio hooks is being classified and sent as HTTP/1 (since the application protocol inside the mTLS tunnel IS HTTP/1.1).

**Localhost traffic IS present** (963 entries with 127.0.0.1), mostly health checks on ports 10248, 10250, 9879, 15020, 15000.

**Top paths by hits:**
- Health checks: `/stats/prometheus`, `/healthz/ready`, `/hello` (Cilium), `/readyz`
- Application: `/wmd-sde-docsharing/checkDSrelationship` (2,291), `/_bulk` (Elasticsearch), `/clasdblogrestservice/v1/logmsg` (748)

---

## Critical Finding: eBPF Capture vs Metric Discrepancy

For the last hour on `gen-obc-k8-uat2`:

| Source | Hits |
|--------|------|
| Istio sidecar logs | 4 |
| USM backend intake dump | 4 |
| `universal.http` metric | ~1 |

**The eBPF capture is working correctly — 4 for 4.** The data makes it from the Istio hooks through the agent to the backend intake.

**The loss is in the backend pipeline** — between the intake and the `universal.http` metric computation. Likely in the network-resolver or metric aggregation stage.

### Possible cause: Port mismatch in network-resolver

- Istio log shows `client.local.address: 10.42.16.142:8080` (original destination port)
- USM connection shows `10.42.16.142:15006` (after iptables REDIRECT to Istio inbound listener)
- The network-resolver needs to map port 15006 back to the original service port (8080) to correctly attribute the traffic
- If this mapping fails, the stats may be dropped or attributed to the wrong service/resource

---

## Architecture: How USM Captures Istio Traffic

### Traffic flow with Istio mTLS

```
Client Pod                                    Server Pod
┌─────────────┐                               ┌─────────────┐
│ App          │                               │ App (:8080)  │
│   ↓ (outbound)                              │   ↑ (plaintext HTTP/1.1)
│ Envoy sidecar│ ──── mTLS (HTTP/1.1) ────→  │ Envoy sidecar│
│ (:15001 out) │    (via pod IPs)              │ (:15006 in)  │
└─────────────┘                               └─────────────┘
```

iptables on the server pod redirects incoming traffic from original port (8080) to Istio inbound port (15006). The envoy sidecar terminates TLS and forwards to the app on localhost.

### How USM captures this

1. **Istio uprobes** hook into envoy's SSL_read/SSL_write → capture decrypted payload
2. `tup_from_ssl_ctx()` resolves the connection tuple from the SSL context
3. `tls_process()` normalizes the tuple (pid=0, netns=0), classifies the decrypted payload
4. For HTTP/1.1 payloads → dispatches to `PROG_HTTP` → `uprobe__http_process` → `http_process`
5. `http_process` uses `http_in_flight` map (keyed by normalized conn_tuple_t) to track request/response pairs
6. Completed transactions are batched and flushed to userspace
7. Process-agent encodes them as `httpAggregations` on the connection and sends to backend
8. Backend network-resolver attributes service names and computes `universal.http` metrics

### What we confirmed from the dump

- Step 1-7 work correctly: the data arrives in the backend intake with correct path, status, TLS tags
- The connection tuple uses port 15006 (Istio inbound), not 8080 (original)
- The loss appears to be in step 8 (network-resolver / metric computation)

---

## Code Analysis (7.73.x branch)

### Key code paths examined

| Component | File | Notes |
|-----------|------|-------|
| HTTP in_flight map | `pkg/network/ebpf/c/protocols/http/maps.h:10` | `BPF_HASH_MAP(http_in_flight, conn_tuple_t, http_transaction_t, 0)` |
| conn_tuple_t struct | `pkg/network/ebpf/c/conn_tuple.h:23-37` | Includes pid (line 32) and netns (line 31) |
| TLS tuple normalization | `pkg/network/ebpf/c/protocols/tls/https.h:92-94` | Zeros pid and netns before processing |
| tls_process | `pkg/network/ebpf/c/protocols/tls/https.h:89-180` | Sets PROTOCOL_TLS on protocol stack, classifies payload, dispatches |
| Protocol stack TLS skip | `pkg/network/ebpf/c/protocols/classification/dispatcher-helpers.h:150-153` | Socket filter skips packets when LAYER_ENCRYPTION is known |
| normalize_tuple | `pkg/network/ebpf/c/port_range.h:25-40` | Only flips IPs/ports based on ephemeral port heuristics; does NOT touch pid/netns |
| read_conn_tuple_skb | `pkg/network/ebpf/c/ip.h:130-195` | Reads from skb; pid and netns are NOT set (remain 0) |
| ssl_ctx_by_pid_tgid fallback | `pkg/network/ebpf/c/protocols/tls/https.h:271-285` | **Active on 7.73.x** — stores ssl_ctx by pid_tgid when ssl_sock_by_ctx misses |
| map_ssl_ctx_to_sock | `pkg/network/ebpf/c/protocols/tls/https.h:315-323` | Maps socket tuple to ssl_ctx via pid_tgid fallback in tcp_sendmsg |
| HTTP encoder | `pkg/network/encoding/marshal/usm_http.go` | Encodes HTTP stats as protobuf per connection |

### ssl_ctx_by_pid_tgid fallback (active on 7.73.x, disabled in SUSM-146 on main)

On 7.73.x, when `ssl_sock_by_ctx` doesn't have an entry for an SSL context (e.g., for pre-existing TLS connections established before system-probe started), the code falls back to storing the ssl_ctx keyed by pid_tgid. On the next `tcp_sendmsg`, `map_ssl_ctx_to_sock` picks this up and maps the current socket to the ssl_ctx.

This mechanism was disabled in commit `c21d3170fa` (SUSM-146) to fix endpoint misattribution in single-threaded proxies. **It is still active on 7.73.x.** However, we have not confirmed it is the cause of SUSM-164 — the dump data shows the eBPF capture IS working correctly (4/4 hits captured). The issue appears to be downstream.

---

## Istio Test Coverage (7.73.x)

### Unit tests

- `pkg/network/usm/istio_test.go`: Tests binary detection, config paths, uprobe lifecycle (sync/dangling). Does NOT test actual traffic capture.
- `pkg/network/config/usm_config_test.go`: Tests Istio monitoring config defaults and overrides.

### Integration tests

- `pkg/network/usm/tests/tracer_usm_linux_test.go`: Tests full monitor startup with Istio enabled. Does NOT test traffic capture or localhost forwarding.

### Load test

- USM load test for 7.73 with Istio HTTP test **passed with 100% accuracy**. Load test starts system-probe before any connections are established, so all TLS handshakes are observed.

### Missing test coverage

- No tests for plaintext HTTP/1.1 between envoy sidecar and localhost app
- No tests for HTTP/1.1 inside Istio mTLS tunnel (as opposed to HTTP/2)
- No tests for the port 15006 (iptables REDIRECT) scenario
- No end-to-end test validating the full path from Istio hooks → backend metric

---

## Live ProcQ Analysis (2026-03-24, aks-cus-gen-prod-002 prod cluster)

### Methodology

Queried `network_raw-main` on `kafka-networks-main-9429` via ProcQ-UI API. Used `/ready` endpoint to poll for completion before downloading, ensuring full Kafka partition coverage.

### Host: `aks-userpool2-97713605-vmss000008-aks-cus-gen-prod-002`

10-minute window, 1000 messages requested, `/ready` confirmed at 20.8% partition scan (found all host messages).

| Source | Hits for `POST /usb/CollateralEvaluation/v5` |
|--------|----------------------------------------------|
| Istio sidecar logs | 3 |
| `network_raw-main` (procq) | **1** |
| `universal.http.server.hits` metric | **1** |

- **208 messages** from this host in the 10-minute window (full coverage confirmed by `/ready`)
- The 1 captured hit has `TagsIdx: -1` (no connection-level TLS tags), while surrounding istio-captured connections have `TagsIdx: 6` with `[tls.library:istio tls.version:tls_1.3]`
- Other istio-captured endpoints on this host work fine: `/publishlogevent/...`, `/underwriting/...`, `/clasdblogrestservice/...`, `/usb/v1/adminappointments` — all with proper `tls.library:istio` tags

### Host: `aks-userpool1-24720288-vmss00000x-aks-cus-gen-prod-002`

1-hour window, 1000 messages.

| Source | Hits for `POST /usb/CollateralEvaluation/v5` |
|--------|----------------------------------------------|
| Istio sidecar logs | 26 |
| `network_raw-main` (procq) | **1** (found in 40 messages) |
| `universal.http.server.hits` metric | **2** |

- **133 istio-tagged connections** with HTTP data present on this host — uprobes are working
- Other `/usb/` paths ARE captured (`/usbf/v1/retrieve`, `/usb/v1/status`)
- Health checks for the `npbcollateralevaluation` pod captured normally (`/app-health/.../readyz`, `/livez`)
- The `npbcollateralevaluation` pod's container tags present in connection metadata (pod is known to the agent)
- The 1 captured hit for `/usb/CollateralEvaluation/v5` also has `TagsIdx: 0` (no TLS connection tags)

### Key Finding: Loss is at the eBPF/Agent Level

The `network_raw → universal.http` pipeline is **lossless** (1=1 on userpool2, procq matches metric). The loss occurs between the Istio sidecar and the agent's eBPF capture:

- **Capture rate: ~33%** (1 out of 3 on userpool2 in 10-min window)
- Istio uprobes ARE functional on these hosts (other services captured with `tls.library:istio`)
- The few hits that ARE captured for `npbcollateralevaluation` in procq lack `tls.library:istio` tags (`TagsIdx: -1` or `TagsIdx: 0`), suggesting they may be captured via a **different mechanism** than the istio uprobes (possibly plaintext socket filter on the envoy→app localhost leg, or the `ssl_ctx_by_pid_tgid` fallback)
- However, the USM metric query (see below) DOES show 1 hit with `tls.library:istio` for this service — so the uprobe path works at least sometimes
- This is consistent with the earlier UAT cluster findings (4/4 capture but low overall rate)

---

## USM Metric Analysis (2026-03-24, same 10-min window on userpool2)

### Per-service `universal.http.server.hits` with `tls.library:istio` tag

Query: `sum:universal.http.server.hits{kube_cluster_name:aks-cus-gen-prod-002,host:aks-userpool2-97713605-vmss000008-aks-cus-gen-prod-002,tls.library:istio}.as_count()`

| Service | SUM (10min) | AVG | MIN | MAX |
|---------|-------------|-----|-----|-----|
| `cashserv-dblogrestsvc` | 1,642 | 78.19 | 47 | 108 |
| `onekyc8687-workflowauditapi` | 466 | 22.19 | 15 | 32 |
| `fndocs-ceapi` | 454 | 21.62 | 2 | 49 |
| `onekyc8687-authentication` | 178 | 8.48 | 3 | 14 |
| `crmshared-aptpub` | 135 | 6.43 | 1 | 16 |
| `cashmidd-rmrksapp` | 124 | 5.90 | 2 | 14 |
| `onekyc8687-workflowrouter` | 72 | 3.79 | 1 | 9 |
| `epm-app` | 5 | 2.50 | 2 | 3 |
| `wmdsde-docexprnc2` | 5 | 1.00 | 1 | 1 |
| `wmdsde-docexprnc` | 4 | 1.00 | 1 | 1 |
| **`lendsvcs-npbcollateralevaluation`** | **1** | 1.00 | 1 | 1 |
| `crmshared-aptsch` | 1 | 1.00 | 1 | 1 |
| `cashmidd-hgcusenq` | 1 | 1.00 | 1 | 1 |

### Key observations

- The Istio uprobe path IS working on this host — multiple services have hundreds/thousands of hits tagged `tls.library:istio`
- `npbcollateralevaluation` does get 1 hit with `tls.library:istio`, confirming the uprobe fires at least sometimes
- **We have NOT verified** whether the high-volume services also undercount vs their Istio logs — the issue may not be specific to `npbcollateralevaluation` but could affect all services to varying degrees. Low-volume services make the discrepancy more visible.
- Several other services also show very low hit counts (1 hit each: `crmshared-aptsch`, `cashmidd-hgcusenq`) — unclear if these are also undercounting

### Istio access log details for `npbcollateralevaluation` (3 requests in 10-min window)

All 3 requests are `POST /usb/CollateralEvaluation/v5`, protocol HTTP/1.1, response 200.

| # | Time (UTC) | Client IP | Duration (ms) | Upstream host |
|---|------------|-----------|---------------|---------------|
| 1 | 16:53:28 | 100.65.246.110 | 1563 | 100.66.128.101:8080 |
| 2 | 16:54:43 | 100.65.246.57 | 408 | 100.66.128.101:8080 |
| 3 | 16:56:38 | 100.65.246.72 | 1394 | 100.66.128.101:8080 |

- All 3 come from **different client IPs** (different source pods/envoy sidecars)
- Each would require a separate mTLS connection/handshake
- Upstream cluster is `inbound|8080||` — standard Istio inbound routing
- User-Agent: `Apache-HttpClient/5.5.1 (Java/17.0.18)` — Java service calling in
- Note: `cashmidd-rmrksapp` (124 USM hits with `tls.library:istio`) has NO Istio access logs visible — possibly logging is not enabled for all services/namespaces, making Istio log comparison unreliable for some services

### Updated Hypothesis

The eBPF capture undercount is confirmed, but the scope is unclear:

1. **It may be systemic** — affecting all services on the host, not just `npbcollateralevaluation`. High-volume services may also undercount but it's less noticeable proportionally. We need to compare Istio logs vs USM metrics for a high-volume service to confirm.
2. **It may be specific to low-frequency traffic patterns** — services with infrequent requests may hit eBPF map eviction, flush timing, or connection tracking edge cases that high-frequency services avoid.
3. **The `tls.library:istio` tag presence** on the 1 captured USM hit confirms the uprobe path does fire for this service, ruling out a total uprobe hooking failure.

---

## Open Questions / Next Steps

1. **Compare Istio logs vs USM metrics for a high-volume service**
   - Pick a service like `cashserv-dblogrestsvc` (1,642 USM hits in 10min) and compare against its Istio sidecar logs
   - If Istio logs also show significantly more, the issue is systemic (not service-specific)
   - If they match, the issue is specific to low-volume traffic patterns
   - Note: Istio access logging may not be enabled for all services (e.g., `cashmidd-rmrksapp` has USM hits but no visible Istio logs)

2. **Investigate eBPF map eviction and flush timing**
   - Check `http_in_flight` map TTL and cleaner interval — could slow requests (1.5s) be evicted before response arrives?
   - 5,170 entries cleaned from `http_in_flight` in the flare — is this normal or excessive?
   - Check if low-frequency connections get cleaned from connection tracking maps

3. **Why do captured hits in procq lack `tls.library:istio` tags while the USM metric shows the tag?**
   - The procq hits have `TagsIdx: -1` (no TLS tags) but USM metric shows `tls.library:istio`
   - These may be different hits (procq captured a socket-filter hit, metric captured an uprobe hit)
   - Or tags may be resolved differently at different pipeline stages

4. **Verify envoy connection reuse patterns**
   - Even though 3 different client IPs, check if envoy reuses TLS sessions (session tickets/resumption)
   - Check envoy connection pool settings for inbound listener

5. **Test the `ssl_ctx_by_pid_tgid` fallback disable (SUSM-146)**
   - The fallback was disabled on main (commit `c21d3170fa`) but is active on 7.73.x
   - Would disabling it make things worse or trigger a different code path?

6. **Investigate whether upgrading to a newer agent version helps**
   - Main branch has SUSM-146 changes — does this affect capture rate?
   - Are there other istio-related fixes post-7.73.0?

---

## Files Referenced

- Agent flare: `~/Downloads/aks-userpool3-24857875-vmss00003 3/`
- Backend dump (gen-obc-k8-uat2): `~/Downloads/dump_1773588474`
- Backend dump (aks-cus-gen-prod-002): `~/Downloads/dump_1773587778`
- Backend dump (small): `~/Downloads/dump_1773587746`
- Analyzer tool: `/Users/daniel.lavie/go/src/github.com/DataDog/dd-go/networks/decode/dump/analyzer/main.go`
- ProcQ dumps (2026-03-24):
  - userpool2 10min: `/tmp/procq_userpool2_10k.bin` (208 messages, 2.2MB)
  - userpool1 1hr: `/tmp/procq_wait.bin` (40 messages, 436K)
  - Org-wide 1hr: `/tmp/procq_dump_10k_org.bin` (6570 messages, 81MB)