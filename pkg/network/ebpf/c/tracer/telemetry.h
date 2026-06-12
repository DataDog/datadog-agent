#ifndef __TRACER_TELEMETRY_H
#define __TRACER_TELEMETRY_H

#include "ktypes.h"

#if defined(COMPILE_RUNTIME) || defined(COMPILE_PREBUILT)
#include <linux/socket.h>
#include <uapi/linux/in.h>
#endif

#include "bpf_endian.h"

#include "ip.h"
#include "tracer/maps.h"
#include "compiler.h"

enum telemetry_counter {
    udp_send_processed,
    udp_send_missed,
    udp_dropped_conns,
    tcp_done_missing_pid,
    tcp_connect_failed_tuple,
    tcp_done_failed_tuple,
    tcp_finish_connect_failed_tuple,
    tcp_close_target_failures,
    tcp_done_connection_flush,
    tcp_close_connection_flush,
    tcp_syn_retransmit,
    protocol_classifier_calls,
    protocol_classifier_skipped_fully_classified,
    protocol_classifier_skipped_fully_classified_v2,
};

static __always_inline void __increment_telemetry_count(enum telemetry_counter counter_name, int times) {
    __u64 key = 0;
    telemetry_t *val = NULL;
    val = bpf_map_lookup_elem(&telemetry, &key);
    if (val == NULL) {
        return;
    }

    switch (counter_name) {
    case udp_send_processed:
        __sync_fetch_and_add(&val->udp_sends_processed, times);
        break;
    case udp_send_missed:
        __sync_fetch_and_add(&val->udp_sends_missed, times);
        break;
    case udp_dropped_conns:
        __sync_fetch_and_add(&val->udp_dropped_conns, times);
        break;
    case tcp_done_missing_pid:
        __sync_fetch_and_add(&val->tcp_done_missing_pid, times);
        break;
    case tcp_connect_failed_tuple:
        __sync_fetch_and_add(&val->tcp_connect_failed_tuple, times);
        break;
    case tcp_done_failed_tuple:
        __sync_fetch_and_add(&val->tcp_done_failed_tuple, times);
        break;
    case tcp_finish_connect_failed_tuple:
        __sync_fetch_and_add(&val->tcp_finish_connect_failed_tuple, times);
        break;
    case tcp_close_target_failures:
        __sync_fetch_and_add(&val->tcp_close_target_failures, times);
        break;
    case tcp_done_connection_flush:
        __sync_fetch_and_add(&val->tcp_done_connection_flush, times);
        break;
    case tcp_close_connection_flush:
        __sync_fetch_and_add(&val->tcp_close_connection_flush, times);
        break;
    case tcp_syn_retransmit:
        __sync_fetch_and_add(&val->tcp_syn_retransmit, times);
        break;
    case protocol_classifier_calls:
        __sync_fetch_and_add(&val->protocol_classifier_calls, times);
        break;
    case protocol_classifier_skipped_fully_classified:
        __sync_fetch_and_add(&val->protocol_classifier_skipped_fully_classified, times);
        break;
    case protocol_classifier_skipped_fully_classified_v2:
        __sync_fetch_and_add(&val->protocol_classifier_skipped_fully_classified_v2, times);
        break;
    }
}

static __always_inline void increment_telemetry_count(enum telemetry_counter counter_name) {
    __increment_telemetry_count(counter_name, 1);
}

static __always_inline void increment_telemetry_count_times(enum telemetry_counter counter_name, int times) {
    __increment_telemetry_count(counter_name, times);
}

// record_classification_attempt_resolved records, into the shadow-evaluation
// histogram, that a connection's application-layer protocol (identified by its
// stored low byte `app_proto_num`) was first observed resolved after `attempts`
// classification attempts. Bounds-checks both indices so the eBPF verifier
// accepts the 2D array access. See the NTWK-684 plan doc.
static __always_inline void record_classification_attempt_resolved(__u8 app_proto_num, __u16 attempts) {
    if (app_proto_num == 0 || app_proto_num >= CLASSIFICATION_APP_PROTO_BUCKETS) {
        return;
    }
    __u16 bucket = attempts;
    if (bucket >= CLASSIFICATION_MAX_ATTEMPT_BUCKETS) {
        bucket = CLASSIFICATION_MAX_ATTEMPT_BUCKETS - 1;
    }
    __u64 key = 0;
    telemetry_t *val = bpf_map_lookup_elem(&telemetry, &key);
    if (val == NULL) {
        return;
    }
    // Bound the indices for the eBPF verifier. The >= checks above let the compiler
    // prove app_proto_num < 16 and ELIDE a plain mask (which is why an earlier
    // `& 15` had no effect — same "invalid access to map value, off=32880"). Run the
    // values through barrier_var() first so the compiler treats them as opaque and
    // cannot elide the mask; the subsequent `& (N-1)` then gives the verifier a
    // provable [0, N-1] bound for the 2D map-value access.
    __u8 proto_idx = app_proto_num;
    __u16 bucket_idx = bucket;
    barrier_var(proto_idx);
    barrier_var(bucket_idx);
    proto_idx &= (CLASSIFICATION_APP_PROTO_BUCKETS - 1);
    bucket_idx &= (CLASSIFICATION_MAX_ATTEMPT_BUCKETS - 1);
    __sync_fetch_and_add(&val->classification_attempt_histogram[proto_idx][bucket_idx], 1);
}

// record_classification_skip_attempt records, into the shadow-evaluation skip histogram,
// that a flow's per-flow classification-attempt counter `a` reached an exact depth in
// CLASSIFICATION_SKIP_EDGES {2,3,4,5,6,7,8,9,10,100}. The counter increments by exactly 1
// per classification pass (monotonic, saturating), so an exact match fires at most once per
// flow per edge — bucket N therefore counts each flow once and equals the number of flows
// that reached >= N attempts (flows a max-attempts cap of N would cut off). Depths not in
// the edge set are ignored. See the NTWK-684 plan doc.
static __always_inline void record_classification_skip_attempt(__u16 a) {
    int bucket;
    switch (a) {
    case 2:   bucket = 0; break;
    case 3:   bucket = 1; break;
    case 4:   bucket = 2; break;
    case 5:   bucket = 3; break;
    case 6:   bucket = 4; break;
    case 7:   bucket = 5; break;
    case 8:   bucket = 6; break;
    case 9:   bucket = 7; break;
    case 10:  bucket = 8; break;
    case 100: bucket = 9; break;
    default:  return;
    }
    __u64 key = 0;
    telemetry_t *val = bpf_map_lookup_elem(&telemetry, &key);
    if (val == NULL) {
        return;
    }
    __sync_fetch_and_add(&val->classification_skip_attempt_histogram[bucket], 1);
}

__maybe_unused static __always_inline void sockaddr_to_addr(struct sockaddr *sa, u64 *addr_h, u64 *addr_l, u16 *port, u32 *metadata) {
    if (!sa) {
        return;
    }

    u16 family = 0;
    bpf_probe_read_kernel(&family, sizeof(family), &sa->sa_family);

    struct sockaddr_in *sin;
    struct sockaddr_in6 *sin6;
    switch (family) {
    case AF_INET:
        *metadata |= CONN_V4;
        sin = (struct sockaddr_in *)sa;
        if (addr_l) {
            bpf_probe_read_kernel(addr_l, sizeof(__be32), &(sin->sin_addr.s_addr));
        }
        if (port) {
            bpf_probe_read_kernel(port, sizeof(__be16), &sin->sin_port);
            *port = bpf_ntohs(*port);
        }
        break;
    case AF_INET6:
        *metadata |= CONN_V6;
        sin6 = (struct sockaddr_in6 *)sa;
        if (addr_l && addr_h) {
            bpf_probe_read_kernel(addr_h, sizeof(u64), sin6->sin6_addr.in6_u.u6_addr8);
            bpf_probe_read_kernel(addr_l, sizeof(u64), &(sin6->sin6_addr.in6_u.u6_addr8[8]));
        }
        if (port) {
            bpf_probe_read_kernel(port, sizeof(u16), &sin6->sin6_port);
            *port = bpf_ntohs(*port);
        }
        break;
    default:
        log_debug("ERR(sockaddr_to_addr): invalid family: %u", family);
    }
}

#endif // __TRACER_TELEMETRY_H
