# SUSM-146: USM Endpoint Misattribution Investigation

## Status

**Investigation ongoing.** ProcQ analysis (2026-03-23) shows the agent correctly tags outgoing `/redfish/v1/` connections as `service:dmt-apiexporter`. However, the erroneous `universal.http.server.hits{service:blackbox-exporter}` metric uses a path (`/redfish/v1/systems/_/processors/`) that only appears on **outgoing** connections in agent data — no matching **incoming** connection was found. ProcQ data is incomplete (~51 of ~2,880 expected messages due to partition limitations). The source of the misattribution remains unclear — it could be in missing agent data, backend metric generation from outgoing connections, or service name resolution.

## Customer Environment

- **Agent version**: 7.68.3
- **Platform**: GKE (Google Kubernetes Engine)
- **Ingress proxy**: Originally Nginx (`internal-ingress-nginx-controller`), recently migrated to HAProxy (`haproxy-ingress` v3.2.4)
- **HAProxy config**: `timeout-client: 300s`, `timeout-server: 300s` (long-lived persistent connections)
- **Note from customer**: "this problem existed before we switched to haproxy, we were using nginx ingress before"

## Observed Symptoms

HTTP endpoints appearing under the wrong service in USM:

- `GET /v1/btc/pasithea_image_url_json` — should be `sre-graphics`, appeared under `dmt-apiexporter`
- `/elemental/alerts`, `/elemental/channelstatus`, `/elemental/stats` — appeared under `blackbox-exporter`

The misattribution is sporadic and low-volume.

## Previous Fix Attempt: Go-TLS Memory Reuse Race Condition

Branch: `usm-gotls-misattribution-fix` (commit `ef4d3346ce`)

### The Bug

The `conn_tup_by_go_tls_conn` eBPF map used only the `tls.Conn` pointer as the key. When the Go runtime reuses memory addresses for new `tls.Conn` objects (e.g., after GC collects a connection where `Close()` was never called), the map would return stale connection tuple data from the old connection, causing HTTP requests to be attributed to the wrong connection.

### The Fix

Changed the map key from a single `tls.Conn` pointer to a composite key (`go_tls_conn_key_t`) containing both the `tls.Conn` pointer AND the `conn_fd_ptr` (pointer to Go's internal `netFD` struct). The `conn_fd_ptr` is unique per TCP connection, so even if Go reuses a `tls.Conn` address, the new connection gets a cache miss instead of a stale hit.

Key changes in `go-tls-conn.h`:
```c
go_tls_conn_key_t key = {
    .tls_conn_ptr = (__u64)conn,
    .conn_fd_ptr = conn_fd_ptr,
};
conn_tuple_t* tup = bpf_map_lookup_elem(&conn_tup_by_go_tls_conn, &key);
```

### Result: Did Not Resolve Customer Issue

A custom agent image (`datadog/agent-dev:usm-gotls-misattribution-fix-full`) was built with the fix, based on agent 7.75 (building on 7.68.3 was not possible).

Timeline:
- **2026-01-28**: Custom image shared with support (Daniel Lavie)
- **2026-02-06**: Customer deployed the custom image
- **2026-02-13**: Customer reported the fix **did not resolve the issue**: "I deployed the new agent on Friday, Feb 6th. Although infrequent still, this is occurring. This endpoint does not belong to the service in the screenshot. Let me know if you want to look at it together or if there is something else we can try."

The go-tls race fix is a valid bug fix (prevents stale cache hits on `tls.Conn` address reuse), but it is not the cause of this customer's misattribution. This led to the current investigation into the native TLS fallback mechanism as a new hypothesis.

## Current Hypothesis: TLS Context Fallback Misassociation

### Background

When system-probe starts, it attaches eBPF probes to OpenSSL/GnuTLS functions. For TLS connections that existed **before** system-probe started, the `SSL_set_fd` call was never intercepted, so the `ssl_sock_by_ctx` map has no entry mapping the TLS context to its socket.

### The Fallback Mechanism

When `ssl_sock_by_ctx` lookup misses (because `SSL_set_fd` wasn't intercepted), a fallback stores the TLS context keyed by `pid_tgid` — a **single slot per thread**:

**`pkg/network/ebpf/c/protocols/tls/https.h:274-286`** — `tup_from_ssl_ctx()`:
```c
// Best-effort fallback mechanism to guess the socket address without
// intercepting the SSL socket initialization. This improves the quality
// of data for TLS connections started *prior* to system-probe
// initialization. Here we simply store the pid_tgid along with its
// corresponding ssl_ctx pointer. In another probe (tcp_sendmsg), we
// query again this map and if there is a match we assume that the *sock
// object is the TCP socket being used by this SSL connection.
bpf_map_update_with_telemetry(ssl_ctx_by_pid_tgid, &pid_tgid, &ssl_ctx, BPF_ANY);
return NULL;
```

**`pkg/network/ebpf/c/protocols/tls/native-tls.h:552-557`** — Every `tcp_sendmsg` calls `map_ssl_ctx_to_sock`:
```c
SEC("kprobe/tcp_sendmsg")
int BPF_BYPASSABLE_KPROBE(kprobe__tcp_sendmsg, struct sock *sk) {
    log_debug("kprobe/tcp_sendmsg: sk=%p", sk);
    map_ssl_ctx_to_sock(sk);
    return 0;
}
```

**`pkg/network/ebpf/c/protocols/tls/https.h:314-330`** — `map_ssl_ctx_to_sock()` consumes the fallback:
```c
static __always_inline void map_ssl_ctx_to_sock(struct sock *skp) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    void **ssl_ctx_map_val = bpf_map_lookup_elem(&ssl_ctx_by_pid_tgid, &pid_tgid);
    if (ssl_ctx_map_val == NULL) {
        return;
    }
    bpf_map_delete_elem(&ssl_ctx_by_pid_tgid, &pid_tgid);

    ssl_sock_t ssl_sock = {};
    if (!read_conn_tuple(&ssl_sock.tup, skp, pid_tgid, CONN_TYPE_TCP)) {
        return;
    }

    void *ssl_ctx = *ssl_ctx_map_val;
    bpf_map_update_with_telemetry(ssl_sock_by_ctx, &ssl_ctx, &ssl_sock, BPF_ANY);
}
```

### How This Causes Misattribution

In a single-threaded event-loop proxy like HAProxy:

1. All connections share one `pid_tgid`
2. `ssl_ctx_by_pid_tgid` is a single-slot-per-key map — each `SSL_read`/`SSL_write` on a pre-existing connection overwrites the previous entry
3. When the proxy then does a plaintext `send()` to a backend, `kprobe/tcp_sendmsg` fires and `map_ssl_ctx_to_sock` consumes the fallback entry
4. This associates the TLS context (from the **frontend** client connection) with the **backend** plaintext socket
5. The backend connection now gets tagged with `LIBSSL` (`StaticTags = 0x2`), causing USM to treat it as a TLS connection and misattribute the decrypted HTTP data

This only affects connections that existed before system-probe started.

### Why It's Sporadic

The fallback only applies to pre-existing connections (those established before system-probe attached its probes). This means:

- New connections established after system-probe starts are unaffected (they go through `SSL_set_fd` → `ssl_sock_by_ctx` properly)
- The window of vulnerability is limited to connections that survived across the system-probe startup
- With `timeout-client: 300s` and `timeout-server: 300s`, pre-existing connections eventually close and get replaced by properly-tracked ones
- The misattribution should only appear around system-probe restarts (agent deployment/restart)

## Local Reproduction

### Setup

Docker-based reproduction with:
- **HAProxy** (172.30.0.10): TLS termination on port 8443, proxying to plaintext backends
- **backend-api** (172.30.0.20): Nginx serving API endpoints (`/v1/btc/...`, `/v1/nfl/...`, etc.)
- **backend-blackbox** (172.30.0.30): Nginx serving monitoring endpoints (`/elemental/...`, `/probe`, `/metrics`)
- **traffic-generator** (172.30.0.40): Python persistent HTTPS client using HTTP/1.1 keep-alive connections

### Reproduction Steps

1. Start the Docker containers (HAProxy + backends + persistent traffic generator)
2. Start system-probe — observe that all traffic is correctly attributed (no TLS tags on plaintext backend connections)
3. Stop system-probe while persistent traffic continues flowing
4. Wait ~15 seconds (traffic generator maintains keep-alive connections)
5. Restart system-probe — the pre-existing connections now trigger the fallback mechanism
6. Observe: backend plaintext connections are tagged with `StaticTags=0x2` (LIBSSL/OpenSSL)

### Results — HAProxy

With system-probe restart while persistent connections are active:
- **Baseline** (system-probe running before connections): 1.3% misattribution (44/3385)
- **After restart** (pre-existing connections trigger fallback): 10.5% misattribution (22/210)
- Without restart (steady state): **0% misattribution**

### Results — Nginx

Reproduced with Nginx TLS proxy (`docker-compose-nginx.yml`) to match the customer's original ingress:
- **After restart**: 4.8% misattribution (22/463)

This confirms the customer's report that the issue existed with **both** Nginx and HAProxy. The fallback mechanism affects any proxy where `SSL_read`/`SSL_write` and plaintext `send()` share the same `pid_tgid` (thread).

### Detection Method

Query `/debug/http_monitoring` and check for entries where:
- `Server.Port` is a plaintext backend port (e.g., 80)
- `StaticTags != 0` (specifically `StaticTags=2` means LIBSSL/OpenSSL)

Example from our reproduction after a restart:
```
BUG (TLS tag on port 80): 22
  172.30.0.10:36960 -> 172.30.0.30:80 /elemental/alerts tags=2
  172.30.0.10:36960 -> 172.30.0.30:80 /elemental/channelstatus tags=2
  172.30.0.10:60244 -> 172.30.0.20:80 /v1/btc/pasithea_image_url_json tags=2
  172.30.0.10:60244 -> 172.30.0.20:80 /v1/graphics/render tags=2
  ...
```

In steady state (no restart), the same query returns **0 bug entries** — all port-80 traffic correctly has `StaticTags=0`.

### How to Confirm the Fallback Is Active in the Customer Environment

**Method 1: Query `/debug/http_monitoring` (works with current agent)**

Ask the customer to run after an agent restart while traffic is flowing:
```bash
sudo curl -s --unix-socket /opt/datadog-agent/run/sysprobe.sock \
  "http://unix/network_tracer/debug/http_monitoring" > /tmp/http_debug.json
```

Then check for entries where `StaticTags=2` on plaintext backend ports. If present, the fallback mechanism is confirmed as the cause.

**Method 2: `bpftool` inspection (does not consume data)**

The `ssl_ctx_by_pid_tgid` eBPF map can be inspected without consuming data:
```bash
sudo bpftool map list | grep ssl_ctx_by_pid
sudo bpftool map dump id <MAP_ID>
```

However, this map is always effectively empty — entries are stored by `tup_from_ssl_ctx` and immediately consumed by `map_ssl_ctx_to_sock` in the next `tcp_sendmsg` within the same thread. The window is too short to observe entries.

**Method 3: Add telemetry counter (requires code change)**

Currently `bpf_map_update_with_telemetry` only tracks map update **errors**, not successful updates. A dedicated counter could be added to `tup_from_ssl_ctx` to count fallback activations, which would appear in `/debug/usm_telemetry`.

## Reproduction Files

All reproduction files are in `pkg/network/usm/testdata/haproxy_tls_leak/`:

| File | Description |
|------|-------------|
| `docker-compose.yml` | HAProxy + 2 nginx backends + Python traffic generator |
| `docker-compose-nginx.yml` | Nginx TLS proxy variant (same backends + traffic gen) |
| `haproxy.cfg` | HAProxy TLS termination with path-based routing |
| `nginx-tls-proxy.conf` | Nginx TLS termination config |
| `nginx-api.conf` | API backend (`/v1/btc/...`, `/v1/nfl/...`, `/conviva/...`, etc.) |
| `nginx-blackbox.conf` | Blackbox backend (`/elemental/...`, `/probe`, `/metrics`) |
| `persistent-client.py` | Python HTTP/1.1 keep-alive client (4 persistent connections) |
| `analyze.py` | Categorizes `/debug/http_monitoring` entries into TLS frontend, correct plaintext, and misattributed |
| `setup.sh` | Generates TLS certs and starts containers |

The `ssl_ctx_race_test/` directory (cherry-picked from `usm-gotls-misattribution-fix`) contains earlier investigation files for the `ssl_ctx_by_pid_tgid` write-path race.

## Diagnostic Fix: Disable TLS Fallback Mechanism

### Approach

Rather than building a full fix (which risks shipping a change for the wrong root cause, as happened with the go-tls race fix), we took a **diagnostic approach**: disable the `ssl_ctx_by_pid_tgid` fallback mechanism entirely and verify misattribution drops to 0%. This gives a binary yes/no answer about whether the fallback is the cause.

The trade-off: pre-existing TLS connections (those established before system-probe starts) will lose their socket association and won't be monitored. New connections are unaffected.

### Fix on 7.68.x — VERIFIED WORKING

Branch: `disable-tls-fallback` (PR #47330, draft, targeting `7.68.x`)

**Change**: Commented out the single write to `ssl_ctx_by_pid_tgid` in `tup_from_ssl_ctx()` (`https.h`):
```c
if (ssl_sock == NULL) {
    // SUSM-146: Disable the fallback mechanism that guesses socket address
    // by storing ssl_ctx keyed by pid_tgid then consuming it in tcp_sendmsg.
    // In single-threaded proxies (HAProxy, Nginx) this causes TLS context
    // from frontend connections to be misassociated with plaintext backend
    // sockets, leading to endpoint misattribution between services.
    // bpf_map_update_with_telemetry(ssl_ctx_by_pid_tgid, &pid_tgid, &ssl_ctx, BPF_ANY);
    return NULL;
}
```

**Results** (tested with direct binary on Vagrant VM):
- HAProxy after restart: **0% misattribution** (was 10.5%)
- Nginx after restart: **0% misattribution** (was 4.8%)

This confirms the `ssl_ctx_by_pid_tgid` fallback is the root cause on 7.68.x.

### Fix on 7.76.x — VERIFIED WORKING

Branch: `SUSM-146/disable-tls-fallback-7.76` (PR #47332, draft, targeting `7.76.x`)

#### Code Differences Between 7.68.x and 7.76.x

7.76.x has **additional write sites** to `ssl_ctx_by_pid_tgid` that don't exist in 7.68.x. These are in `native-tls.h` handshake probes:

1. `uprobe/SSL_do_handshake` — writes `ssl_ctx` keyed by `pid_tgid`
2. `uprobe/SSL_connect` — writes `ssl_ctx` keyed by `pid_tgid`
3. `uprobe/gnutls_handshake` — writes `ssl_ctx` keyed by `pid_tgid`

These probes were added to the 7.76 code to handle handshake-phase fallback in addition to the read/write-phase fallback in `tup_from_ssl_ctx`.

Additionally, 7.76 uses a composite key `ssl_ctx_pid_tgid_t` (pid_tgid + ctx) for `ssl_sock_by_ctx` and adds a `ssl_ctx_by_tuple` map for reverse lookups during cleanup.

#### Fix Applied

Disabled ALL write sites:
1. `tup_from_ssl_ctx()` in `https.h` — same as 7.68.x fix
2. `SSL_do_handshake` uprobe in `native-tls.h` — emptied function body to just `return 0;`
3. `SSL_connect` uprobe in `native-tls.h` — emptied function body to just `return 0;`
4. `gnutls_handshake` uprobe in `native-tls.h` — emptied function body to just `return 0;`

Return probes (`uretprobe`) were left intact — they only delete from the map.

Note: Simply commenting out the `bpf_map_update_with_telemetry` calls caused `-Werror,-Wunused-variable` errors because `pid_tgid` was no longer referenced (and `log_debug` is compiled out in non-debug/prebuilt builds). The solution was to empty the entire function body.

#### Testing Results — Standalone Binary on VM (clean eBPF build)

After cleaning the eBPF build cache (`rm -rf pkg/ebpf/bytecode/build/arm64/`) and rebuilding from scratch on the `SUSM-146/disable-tls-fallback-7.76` branch:

- Nginx first start (pre-existing connections): **0% misattribution** (0/549 port-80 entries)
- Nginx after restart (stop 15s, restart): **0% misattribution** (0/704 port-80 entries)

#### Testing Results — Docker Agent Image

Image: `datadog/agent-dev:disable-tls-fallback-7-76-full` (CI-built from commit `c21d3170fa`)

- Nginx first start (pre-existing connections): **0% misattribution** (0/105 port-80 entries)
- Nginx after restart (stop 15s, restart): **0% misattribution** (0/39 port-80 entries)

#### Control Test — Stock Agent Image

Image: `datadog/agent:7.76.3-full` (unpatched)

- Nginx first start (pre-existing connections): **81.5% misattribution** (44/54 port-80 entries)

#### Previous Incorrect Test Results (Explained)

Earlier testing of the Docker image on the Vagrant VM showed persistent misattribution. This was caused by **eBPF object caching on the VM** — when switching branches for the standalone binary test, Ninja reused stale eBPF `.o` files from the previous branch instead of recompiling. The stale objects didn't have the fix, contaminating the test. Once the eBPF build cache was cleaned (`rm -rf pkg/ebpf/bytecode/build/arm64/`), the standalone binary also showed 0% misattribution, matching the Docker image results.

### PRs

| PR | Branch | Target | Status |
|----|--------|--------|--------|
| [#47330](https://github.com/DataDog/datadog-agent/pull/47330) | `disable-tls-fallback` | `7.68.x` | Draft |
| [#47332](https://github.com/DataDog/datadog-agent/pull/47332) | `SUSM-146/disable-tls-fallback-7.76` | `7.76.x` | Draft — fix verified |

## Docker Agent Testing Setup

For testing custom agent images with the reproduction containers:

```yaml
# /tmp/dd-agent-test/docker-compose.yml
services:
  datadog:
    image: "datadog/agent-dev:disable-tls-fallback-7-76-full"
    environment:
     - DD_API_KEY=0000000000000000
     - DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED=true
     - DD_SYSTEM_PROBE_PROCESS_SERVICE_INFERENCE_ENABLED=true
    volumes:
     - /var/run/docker.sock:/var/run/docker.sock:ro
     - /proc/:/host/proc/:ro
     - /sys/fs/cgroup/:/host/sys/fs/cgroup:ro
     - /sys/kernel/debug:/sys/kernel/debug
     - /lib/modules:/lib/modules
     - /usr/src:/usr/src
     - /var/tmp/datadog-agent/system-probe/build:/var/tmp/datadog-agent/system-probe/build
     - /var/tmp/datadog-agent/system-probe/kernel-headers:/var/tmp/datadog-agent/system-probe/kernel-headers
     - /etc/apt:/host/etc/apt
     - /etc/yum.repos.d:/host/etc/yum.repos.d
     - /etc/zypp:/host/etc/zypp
     - /etc/pki:/host/etc/pki
     - /etc/yum/vars:/host/etc/yum/vars
     - /etc/dnf/vars:/host/etc/dnf/vars
     - /etc/rhsm:/host/etc/rhsm
    cap_add:
     - SYS_ADMIN
     - SYS_RESOURCE
     - SYS_PTRACE
     - NET_ADMIN
     - NET_BROADCAST
     - NET_RAW
     - IPC_LOCK
     - CHOWN
    security_opt:
     - apparmor:unconfined
    network_mode: host
```

Run on the Vagrant VM alongside the reproduction containers. Use `docker compose down && docker compose up -d` to restart the agent (simulates system-probe restart).

## Build Notes

### Proper Build Mechanism

Building system-probe on the Vagrant VM requires using `.run/bash_runner.sh`:
```bash
SCRIPT_TO_RUN=.run/build.sh BUILD_COMMAND=dda\ inv\ system-probe.build /bin/bash .run/bash_runner.sh
```

This handles SSH connection, environment variable forwarding, and uses `bash --login` for proper PATH setup (Go, clang, etc.). Direct SSH commands (`ssh vagrant@... 'dda inv system-probe.build'`) fail because the `cgo -godefs` step requires `clang` in PATH, which is only set up in login shells.

### eBPF Object Caching Warning

The build system (Ninja) tracks file timestamps, not content. When switching branches, eBPF objects may not be recompiled if timestamps haven't changed. For accurate testing across branches, ensure a clean eBPF build (e.g., remove cached `.o` files under the build directory).

## Reproduction Files

All reproduction files are in `pkg/network/usm/testdata/haproxy_tls_leak/`:

| File | Description |
|------|-------------|
| `docker-compose.yml` | HAProxy + 2 nginx backends + Python traffic generator |
| `docker-compose-nginx.yml` | Nginx TLS proxy variant (same backends + traffic gen) |
| `haproxy.cfg` | HAProxy TLS termination with path-based routing |
| `nginx-tls-proxy.conf` | Nginx TLS termination config |
| `nginx-api.conf` | API backend (`/v1/btc/...`, `/v1/nfl/...`, `/conviva/...`, etc.) |
| `nginx-blackbox.conf` | Blackbox backend (`/elemental/...`, `/probe`, `/metrics`) |
| `persistent-client.py` | Python HTTP/1.1 keep-alive client (4 persistent connections) |
| `analyze.py` | Categorizes `/debug/http_monitoring` entries into TLS frontend, correct plaintext, and misattributed |
| `setup.sh` | Generates TLS certs and starts containers |

The `ssl_ctx_race_test/` directory (cherry-picked from `usm-gotls-misattribution-fix`) contains earlier investigation files for the `ssl_ctx_by_pid_tgid` write-path race.

## Customer Deployment of Custom Image

### Timeline

- **2026-03-15**: Custom image `datadog/agent-dev:disable-tls-fallback-7-76-full` shared with support (Daniel Lavie → Traeger Meyer)
- **2026-03-17**: Customer deployed the custom image
- **2026-03-18**: Support reported the fix **did not resolve the issue** — customer still sees the erroneous endpoint `partlow` misattributed to the wrong service

### Implication

The `ssl_ctx_by_pid_tgid` fallback mechanism is a confirmed bug that causes misattribution in local reproduction (single-threaded proxy + system-probe restart), but the customer's misattribution has **at least one additional cause** beyond this fallback. This mirrors the earlier go-tls race fix attempt — each fix addresses a real bug but doesn't fully explain the customer's symptoms.

## ProcQ Analysis: Agent Data Is Correct (2026-03-23)

### Approach

Queried ProcQ-UI (internal Kafka inspection tool) for the customer's actual agent payloads to determine whether the misattribution originates in the agent/eBPF layer or downstream in the backend pipeline.

- **Datacenter**: US5
- **Org ID**: 1300014336
- **Topics queried**: `network_raw` (agent-side data) and `network_connections` (after network-resolver processing)

### Reported Erroneous Endpoint

Customer reported `get_/redfish/v1/systems/_/processors/` appearing under `blackbox-exporter` service on host `gke-mlb-sre-shared-c-mlb-sre-shared-c-d92e0a8b-kszi.c.mlb-sre-shared-b967.internal`.

### Findings from `network_raw` (Agent Payloads)

The `dmt-apiexporter` pods (PIDs 81796, 81573) on this host handle redfish traffic in **two directions**:

**Outgoing connections** (dmt-apiexporter → hardware BMCs):
- Go-TLS connections to external IPs (10.113.x.x:443)
- Long paths: `/redfish/v1/Systems/1/Processors/1`, `/redfish/v1/Systems/System.Embedded.1/Processors/CPU.Socket.1`, etc.
- conn_tags: `env:npd service:dmt-apiexporter version:2.1.16`, `tls.library:go`
- ~3,800 entries in 51 messages

**Incoming connections** (clients → dmt-apiexporter:8080):
- Plaintext HTTP on port 8080 from clients (10.140.16.70, 10.140.17.203 — likely blackbox-exporter probing dmt-apiexporter)
- Short paths: `/redfish/processor`, `/redfish/power`, `/redfish/thermal`, `/redfish_dell/memory`, etc.
- **No conn_tags** (no `service:` tag from the agent on incoming connections)
- ~746 entries in 51 messages

| | PID 81796 | PID 81573 |
|--|-----------|-----------|
| **Container ID** | `6ffd28014c3f...` | `9d00e4995096...` |
| **Pod** | `dmt-apiexporter-npd-54c48f6b7f-xf5d9` | `dmt-apiexporter-npd-54c48f6b7f-6wwxs` |
| **Process cmdline** | `dmt serve --passfile /device-metric-tool/secrets/auths.yaml --noAuth` | same |
| **Image** | `artifacts.mlbinfra.net/docker/o11y/dmt-apiexporter:2.1.16` | same |

**Key observation**: The erroneous metric is `universal.http.server.hits{service:blackbox-exporter, resource_name:get_/redfish/v1/systems/_/processors/}` — a **server-side** metric with a **long** `/redfish/v1/` path. But in the agent data, `/redfish/v1/` paths only appear on **outgoing** connections (dmt-apiexporter calling BMCs). The incoming connections (which generate `server.hits`) only have **short** paths (`/redfish/processor`). We found **zero** incoming connections with `/redfish/v1/` paths across 51 messages.

### Data Completeness Limitation

ProcQ returned only **51 messages** out of ~2,880 expected (agent sends ~1 message/30s). The `partition_scheme: "guess"` only finds a subset of Kafka partitions for this host. The exact moment of the 1 erroneous hit (Mar 22 17:40 UTC) may be in the ~95% of messages we couldn't retrieve.

### Findings from `network_connections` (After Network-Resolver)

The `network_connections` topic (post network-resolver, 350 messages) shows:
- **Outgoing** `/redfish/v1/` connections: present, correctly tagged `service:dmt-apiexporter` with full container tags
- **Incoming** `/redfish/` connections: **not present** (they exist in `network_raw` but are absent from `network_connections`)

The incoming redfish connections disappearing between `network_raw` and `network_connections` is notable — the network-resolver may be dropping or transforming them.

### What We Know

1. The agent correctly tags outgoing `/redfish/v1/` connections as `service:dmt-apiexporter`
2. The incoming `/redfish/` connections to dmt-apiexporter:8080 have **no service tag** from the agent
3. We cannot find an incoming `/redfish/v1/` connection in the data we have — either it's in the missing messages, or the backend is generating the `server.hits` metric from outgoing data
4. There is no `blackbox-exporter` process making `/redfish/v1/` requests on this host

### What We Don't Know

- What the agent payload looked like at the exact moment (Mar 22 17:40 UTC) of the erroneous hit — ProcQ can't retrieve data for that time range
- Whether the backend generates `universal.http.server.hits` from outgoing connections when the remote side is unmonitored
- How the service name `blackbox-exporter` gets assigned — nothing in the agent data references it

## Next Steps

1. **Determine how `universal.http.server.hits` is generated for outgoing connections**: The erroneous metric has a `/redfish/v1/` path that only exists on outgoing connections in agent data. Does the backend/NSX generate `server.hits` from outgoing connections when the remote side is unmonitored? If so, how does it determine the service name?

2. **Investigate the missing incoming connections in `network_connections`**: The incoming `/redfish/` connections exist in `network_raw` but disappear in `network_connections`. Understanding why could explain the data flow.

3. **Get more ProcQ data**: ProcQ only returned 51 out of ~2,880 expected messages due to partition scheme limitations. Try different partition schemes or query by IP instead of hostname to get more complete data, especially around the Mar 22 17:40 UTC timeframe.

4. **Check customer's Service Naming Rules**: The customer may have org-level rules that could affect service resolution.

5. **Proper fix for the fallback bug**: The `ssl_ctx_by_pid_tgid` fallback disable fix is still a valid improvement that should be upstreamed, regardless of this customer's issue.

## Open Questions

- Does the backend generate `universal.http.server.hits` from outgoing connections when the remote side has no agent? If so, how does it determine the service for the "server" side?
- Why do incoming `/redfish/` connections from `network_raw` not appear in `network_connections`?
- How does `blackbox-exporter` get assigned as the service name when nothing in the agent data references it?
- Is there an incoming `/redfish/v1/` connection in the ~95% of messages we couldn't retrieve from ProcQ?