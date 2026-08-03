#ifndef __TLS_MISCLASSIFICATION_DIAG_H
#define __TLS_MISCLASSIFICATION_DIAG_H

#include "ktypes.h"
#include "map-defs.h"
#include "conn_tuple.h"
#include "protocols/classification/defs.h"
#include "protocols/classification/tls-misclassification-diag-types.h"
#include "compiler.h"

// Diagnostics for the suspected TLS-reported-as-plaintext root cause.
// See the investigation notes on branch jmw/tls-misclassification.
//
// Suspected chain, on a connection whose TLS handshake was never observed by the socket filter
// (established before system-probe started, or before its tuple was tracked):
//
//   1. is_tls() rejects genuine TLS bytes, because read_tls_record_header() requires the entire
//      record to fit inside one packet. Records run to 16 KB; MSS is ~1460.
//   2. Classification falls through to the app-layer classifiers. is_redis() tests a SINGLE byte
//      against 14 RESP type markers, so it matches arbitrary ciphertext ~5.5% of the time.
//   3. mark_as_fully_classified() plus the `app_layer_proto == UNKNOWN || POSTGRES` gate on the
//      is_tls() call mean the wrong answer is pinned for the life of the map entry.
//
// Result: encrypted traffic is reported as tls_encrypted:false.
//
// The counters in telemetry_t quantify each link. This map carries the per-connection detail
// (which port, which protocol, which TLS content type) that makes a report actionable, for
// userspace to drain and log.
//
// Deliberately a plain hash map, NOT an LRU map: BPF_MAP_TYPE_LRU_HASH was added in kernel 4.10,
// but classification runs from 4.11 down through runtime compilation on far older kernels
// (classificationMinimumKernel is 4.11, and KMT covers debian_9 / ubuntu_16.04 on 4.9 / 4.4).
// Runtime compilation uses the host's kernel headers, so referencing the LRU enum there fails to
// compile outright. A plain hash map does not evict, so userspace deletes every entry it visits
// on each drain to keep space available; if the map does fill between drains, further events are
// dropped, which is acceptable because the telemetry_t counters remain authoritative.


// is_tls_diag_enabled reports whether these diagnostics are active. The value is patched in by
// userspace (see util.TLSDiagnosticsSupported) before the program is loaded, so at verification time
// it is a known constant and the verifier prunes every guarded branch when it is false. That is what
// keeps the classifier's complexity at baseline on older kernels, whose verifiers prune far less
// aggressively and rejected socket__classifier_entry with these branches present.
//
// Note this does not reduce program size — the dead instructions still count toward BPF_MAXINSNS.
static __always_inline bool is_tls_diag_enabled() {
    __u64 val = 0;
    LOAD_CONSTANT("tls_diag_enabled", val);
    return val > 0;
}

// Standard Redis ports. Redis serves 6379 by convention (6380 for TLS), 26379 for Sentinel and
// 16379 for the cluster bus. A Redis classification on anything else is not credible.
#define REDIS_PORT_DEFAULT  6379
#define REDIS_PORT_TLS      6380
#define REDIS_PORT_CLUSTER 16379
#define REDIS_PORT_SENTINEL 26379


// Bounded deliberately: this is a diagnostic side channel, not a data path. 1024 entries is enough
// to characterise a problem without meaningful memory cost.
BPF_HASH_MAP(tls_diag_events, conn_tuple_t, tls_diag_event_t, 1024)

// is_standard_redis_port reports whether either side of the tuple is a port Redis actually serves.
static __always_inline bool is_standard_redis_port(__u16 sport, __u16 dport) {
    return sport == REDIS_PORT_DEFAULT || dport == REDIS_PORT_DEFAULT ||
           sport == REDIS_PORT_TLS || dport == REDIS_PORT_TLS ||
           sport == REDIS_PORT_CLUSTER || dport == REDIS_PORT_CLUSTER ||
           sport == REDIS_PORT_SENTINEL || dport == REDIS_PORT_SENTINEL;
}

// record_tls_misclassification_event records (or bumps the hit count on) a diagnostic event for
// this connection. Keyed by the pre-normalization skb tuple with pid/netns zeroed, so repeated
// observations of the same connection coalesce into one entry instead of flooding the map.
//
// Best-effort by design: a failed insert (map full, since a plain hash map does not evict) is
// silently ignored, because the telemetry_t counters remain the authoritative signal.
static __always_inline void record_tls_misclassification_event(conn_tuple_t *tup, tls_diag_reason_t reason, __u16 app_layer_proto, __u8 tls_content_type) {
    conn_tuple_t key = *tup;
    key.pid = 0;
    key.netns = 0;

    tls_diag_event_t *existing = bpf_map_lookup_elem(&tls_diag_events, &key);
    if (existing) {
        // Only coalesce when it is the same finding; a different reason on the same connection is
        // worth surfacing, so overwrite and let userspace log it again.
        if (existing->reason == reason) {
            __sync_fetch_and_add(&existing->hits, 1);
            return;
        }
    }

    tls_diag_event_t event = {};
    event.timestamp = bpf_ktime_get_ns();
    event.hits = 1;
    event.sport = tup->sport;
    event.dport = tup->dport;
    event.app_layer_proto = app_layer_proto;
    event.tls_content_type = tls_content_type;
    event.reason = (__u8)reason;

    bpf_map_update_elem(&tls_diag_events, &key, &event, BPF_ANY);
}

#endif // __TLS_MISCLASSIFICATION_DIAG_H
