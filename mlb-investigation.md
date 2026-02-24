# SUSM-146: USM Endpoint Misattribution Investigation

## Status

Investigation in progress. Local reproduction achieved.

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

### Results

With system-probe restart while persistent connections are active:
- **333,165 requests** with `tags=OpenSSL` on plaintext backend connections
- **45.5% leak rate** among observed entries
- Without restart (system-probe running from the start): **0% leak rate**

### Detection Method

Query `/debug/http_monitoring` and check for entries where:
- `server_addr` is a plaintext backend (port 80)
- `StaticTags != 0` (has TLS tags like `LIBSSL = 0x2`)

This means USM is incorrectly attributing TLS-decrypted data to the plaintext backend connection tuple, causing endpoint misattribution between services.

## Open Questions

- The customer reports this issue existed with both Nginx and HAProxy. Nginx uses multi-process architecture (not single-threaded like HAProxy). The `pid_tgid` collision mechanism may work differently with Nginx workers — needs further investigation.
- What is the customer's agent restart/deployment cadence? This would help correlate with observed misattribution windows.
- Does the customer observe misattribution concentrated around specific time windows (which would correlate with agent restarts)?