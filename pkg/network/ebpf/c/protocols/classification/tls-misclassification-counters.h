#ifndef __TLS_MISCLASSIFICATION_COUNTERS_H
#define __TLS_MISCLASSIFICATION_COUNTERS_H

#include "ktypes.h"
#include "map-defs.h"
#include "compiler.h"

// Counters for the TLS-reported-as-plaintext investigation that must be reachable from usm.c as
// well as tracer.c. See jmw/tls-misclassification.
//
// Why a separate map rather than telemetry_t: telemetry_t lives in tracer/maps.h, and including
// that from usm.c would declare every tracer map inside usm.o. This header deliberately depends
// only on ktypes/map-defs/compiler so it can be included from either object.
//
// Why this exists at all: staging showed 248k tls_locked_out_by_applayer events on vaporeon-c, all
// naming PROTOCOL_REDIS (16391), while the two counters at the socket-filter classification site
// (applayer_match_on_tls_payload, redis_match_on_nonstandard_port) stayed at zero. The lock is
// observed but never the act of locking, which means the Redis verdict is being written somewhere
// other than protocol_classifier_entrypoint_dbs. is_redis() has two other callers, both in usm.c:
// the protocol dispatcher and the decrypted-TLS path. Both call set_protocol() on the *shared*
// connection_protocol stack, so either can produce the lock. These counters tell us which.
//
// USM Redis monitoring is confirmed enabled on the affected cluster (usm.redis.connections = 1223
// over 24h), so the dispatcher path is live and is the leading suspect.

typedef enum {
    // is_redis() matched inside classify_protocol_for_dispatcher() (protocol dispatcher, usm.c).
    TLS_DIAG_CTR_USM_DISPATCHER_REDIS = 0,
    // is_redis() matched inside classify_decrypted_payload() (TLS uprobe path, usm.c). A match here
    // is on *decrypted* bytes, so it may be a legitimate Redis-inside-TLS classification.
    TLS_DIAG_CTR_USM_TLS_REDIS = 1,
    // is_redis() matched inside classify_db_protocols() (socket filter, tracer.o). Counted
    // unconditionally so it is directly comparable with the two USM counters above;
    // redis_match_on_nonstandard_port only fires on a non-Redis port, which makes it useful as an
    // invariant but useless as a "did this site fire" signal.
    TLS_DIAG_CTR_SOCKET_FILTER_REDIS = 2,
    TLS_DIAG_CTR_MAX = 4,
} tls_diag_counter_t;

BPF_ARRAY_MAP(tls_diag_usm_counters, __u64, TLS_DIAG_CTR_MAX)

// is_tls_diag_enabled reports whether the diagnostics are active. Patched in by userspace before
// load (see util.TLSDiagnosticsSupported), so at verification time it is a known constant and the
// verifier prunes every guarded branch when false.
//
// NOTE: this constant must be supplied by *every* manager that loads an object containing these
// branches — the tracer managers and the USM manager. If a manager forgets it, the value stays 0
// and the counters silently read zero, which is indistinguishable from "the site never fired".
static __always_inline bool is_tls_diag_enabled() {
    __u64 val = 0;
    LOAD_CONSTANT("tls_diag_enabled", val);
    return val > 0;
}

static __always_inline void tls_diag_count(__u32 idx) {
    __u64 *val = bpf_map_lookup_elem(&tls_diag_usm_counters, &idx);
    if (val) {
        __sync_fetch_and_add(val, 1);
    }
}

#endif // __TLS_MISCLASSIFICATION_COUNTERS_H
