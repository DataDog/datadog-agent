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

## Open Questions / Next Steps

1. **Why does the backend metric show ~1 hit when the intake dump has 4?**
   - Is the network-resolver failing to attribute connections on port 15006?
   - Does the port mismatch (15006 vs original 8080) cause the resolver to drop or misattribute stats?
   - Are stats being deduplicated or merged incorrectly?

2. **Is the historical discrepancy (171 vs 32K over 3 days) the same issue or different?**
   - The current snapshot shows 4/4 capture rate at the eBPF level
   - Need to verify whether the eBPF capture rate was also ~100% during the historical period, or if there was genuine eBPF-level loss
   - The 32K Istio log count might include traffic from multiple pods/replicas while the USM metric might be scoped to a single node

3. **Investigate the network-resolver attribution for Istio port 15006 connections**
   - How does the resolver handle iptables REDIRECT (original port 8080 → 15006)?
   - Does it use conntrack to resolve the original destination?
   - What happens when the resolver can't map port 15006 back to the service?

4. **Verify the scope of the customer's comparison**
   - Are the Istio logs scoped to the same node/pod as the USM metric?
   - Could the 32K logs include traffic from multiple replicas across nodes?
   - What is the exact USM metric query the customer/SE is using?

---

## Files Referenced

- Agent flare: `~/Downloads/aks-userpool3-24857875-vmss00003 3/`
- Backend dump (gen-obc-k8-uat2): `~/Downloads/dump_1773588474`
- Backend dump (aks-cus-gen-prod-002): `~/Downloads/dump_1773587778`
- Backend dump (small): `~/Downloads/dump_1773587746`
- Analyzer tool: `/Users/daniel.lavie/go/src/github.com/DataDog/dd-go/networks/decode/dump/analyzer/main.go` (modified for this investigation)