#ifndef __TRACER_EVENTS_H
#define __TRACER_EVENTS_H

#include "tracer.h"

#include "tracer-maps.h"
#include "tracer-telemetry.h"
#include "tcp_states.h"
#include "cookie.h"
#include "protocols/protocol-classification-helpers.h"

#include "bpf_helpers.h"
#include "bpf_builtins.h"

static __always_inline int get_proto(conn_tuple_t *t) {
    return (t->metadata & CONN_TYPE_TCP) ? CONN_TYPE_TCP : CONN_TYPE_UDP;
}

static __always_inline void clean_protocol_classification(conn_tuple_t *tup) {
    conn_tuple_t conn_tuple = *tup;
    conn_tuple.pid = 0;
    conn_tuple.netns = 0;
    bpf_map_delete_elem(&connection_protocol, &conn_tuple);

    conn_tuple_t *skb_tup_ptr = bpf_map_lookup_elem(&conn_tuple_to_socket_skb_conn_tuple, &conn_tuple);
    if (skb_tup_ptr != NULL) {
        conn_tuple_t skb_tup = *skb_tup_ptr;
        conn_tuple_t inverse_skb_conn_tup = {0};
        invert_conn_tuple(skb_tup_ptr, &inverse_skb_conn_tup);
        inverse_skb_conn_tup.pid = 0;
        inverse_skb_conn_tup.netns = 0;
        bpf_map_delete_elem(&connection_protocol, &inverse_skb_conn_tup);
        bpf_map_delete_elem(&skb_conn_tuple_to_socket_conn_tuple, &skb_tup);
    }

    bpf_map_delete_elem(&conn_tuple_to_socket_skb_conn_tuple, &conn_tuple);
}

static __always_inline void cleanup_conn(conn_tuple_t *tup, struct sock *sk) {
    clean_protocol_classification(tup);

    u32 cpu = bpf_get_smp_processor_id();

    // Will hold the full connection data to send through the perf buffer
    conn_t conn = { .tup = *tup };
    conn_stats_ts_t *cst = NULL;
    bool is_tcp = get_proto(&conn.tup) == CONN_TYPE_TCP;
    bool is_udp = get_proto(&conn.tup) == CONN_TYPE_UDP;

    // TCP stats don't have the PID
    if (is_tcp) {
        conn.tup.pid = 0;
        tcp_stats_t *tst = bpf_map_lookup_elem(&tcp_stats, &(conn.tup));
        if (tst) {
            conn.tcp_stats = *tst;
            bpf_map_delete_elem(&tcp_stats, &(conn.tup));
        }
        conn.tup.pid = tup->pid;

        conn.tcp_stats.state_transitions |= (1 << TCP_CLOSE);
    }

    cst = bpf_map_lookup_elem(&conn_stats, &(conn.tup));
    if (!cst && is_udp) {
        increment_telemetry_count(udp_dropped_conns);
        return; // nothing to report
    }

    if (cst) {
        conn.conn_stats = *cst;
        bpf_map_delete_elem(&conn_stats, &(conn.tup));
    } else {
        // we don't have any stats for the connection,
        // so cookie is not set, set it here
        conn.conn_stats.cookie = get_sk_cookie(sk);
    }

    conn.conn_stats.timestamp = bpf_ktime_get_ns();

    // Batch TCP closed connections before generating a perf event
    batch_t *batch_ptr = bpf_map_lookup_elem(&conn_close_batch, &cpu);
    if (batch_ptr == NULL) {
        return;
    }

    // TODO: Can we turn this into a macro based on TCP_CLOSED_BATCH_SIZE?
    switch (batch_ptr->len) {
    case 0:
        batch_ptr->c0 = conn;
        batch_ptr->len++;
        return;
    case 1:
        batch_ptr->c1 = conn;
        batch_ptr->len++;
        return;
    case 2:
        batch_ptr->c2 = conn;
        batch_ptr->len++;
        return;
    case 3:
        batch_ptr->c3 = conn;
        batch_ptr->len++;
        // In this case the batch is ready to be flushed, which we defer to kretprobe/tcp_close
        // in order to cope with the eBPF stack limitation of 512 bytes.
        return;
    }

    // If we hit this section it means we had one or more interleaved tcp_close calls.
    // This could result in a missed tcp_close event, so we track it using our telemetry map.
    if (is_tcp) {
        increment_telemetry_count(missed_tcp_close);
    }
    if (is_udp) {
        increment_telemetry_count(missed_udp_close);
    }
}

static __always_inline void flush_conn_close_if_full(struct pt_regs *ctx) {
    u32 cpu = bpf_get_smp_processor_id();
    batch_t *batch_ptr = bpf_map_lookup_elem(&conn_close_batch, &cpu);
    if (!batch_ptr) {
        return;
    }

    if (batch_ptr->len == CONN_CLOSED_BATCH_SIZE) {
        // Here we copy the batch data to a variable allocated in the eBPF stack
        // This is necessary for older Kernel versions only (we validated this behavior on 4.4.0),
        // since you can't directly write a map entry to the perf buffer.
        batch_t batch_copy = {};
        bpf_memcpy(&batch_copy, batch_ptr, sizeof(batch_copy));
        batch_ptr->len = 0;
        batch_ptr->id++;
        bpf_perf_event_output(ctx, &conn_close_event, cpu, &batch_copy, sizeof(batch_copy));
    }
}

// Forward declaration.
static int read_conn_tuple(conn_tuple_t *t, struct sock *skp, u64 pid_tgid, metadata_mask_t type);

static __always_inline void read_into_buffer1(char *buffer, char *data, size_t data_size) {
    log_debug("guy read into buffer %p", buffer);
    // we read CLASSIFICATION_MAX_BUFFER-1 bytes to ensure that the string is always null terminated
    int ret = bpf_probe_read_user_with_telemetry(buffer, CLASSIFICATION_MAX_BUFFER - 1, data);
    if (ret < 0) {
        log_debug("guy err %d %p", ret, buffer);
        ret = bpf_probe_read_kernel_with_telemetry(buffer, CLASSIFICATION_MAX_BUFFER - 1, data);
        if (ret >= 0) {
            return;
        }

        log_debug("guy err2 %d %p", ret, buffer);
// note: arm64 bpf_probe_read_user() could page fault if the CLASSIFICATION_MAX_BUFFER overlap a page
#pragma unroll(CLASSIFICATION_MAX_BUFFER - 1)
        for (int i = 0; i < CLASSIFICATION_MAX_BUFFER - 1; i++) {
            bpf_probe_read_user(&buffer[i], 1, &data[i]);
            if (buffer[i] == 0) {
                return;
            }
        }
    }
}

static __always_inline void read_into_buffer2(char *buffer, char *data, size_t data_size) {
    // we read CLASSIFICATION_MAX_BUFFER-1 bytes to ensure that the string is always null terminated
    if (bpf_probe_read_kernel_with_telemetry(buffer, CLASSIFICATION_MAX_BUFFER - 1, data) < 0) {
// note: arm64 bpf_probe_read_kernel() could page fault if the CLASSIFICATION_MAX_BUFFER overlap a page
#pragma unroll(CLASSIFICATION_MAX_BUFFER - 1)
        for (int i = 0; i < CLASSIFICATION_MAX_BUFFER - 1; i++) {
            bpf_probe_read_kernel(&buffer[i], 1, &data[i]);
            if (buffer[i] == 0) {
                return;
            }
        }
    }
}

// Common implementation for tcp_sendmsg different hooks among prebuilt/runtime binaries.
static __always_inline void tcp_sendmsg_helper(struct sock *sk, void *buffer_ptr, size_t buffer_size) {
    u64 pid_tgid = bpf_get_current_pid_tgid();

    log_debug("kprobe/tcp_sendmsg: pid_tgid: %d\n", pid_tgid);

    tcp_sendmsg_args_t args = {0};
    args.sk = sk;
    if (!read_conn_tuple(&args.conn_tuple, args.sk, pid_tgid, CONN_TYPE_TCP)) {
        return;
    }

    log_debug("%llu guy send addr %llu %llu\n", pid_tgid, args.conn_tuple.saddr_l, args.conn_tuple.daddr_l);
    log_debug("%llu guy send port %d %d\n", pid_tgid, args.conn_tuple.sport, args.conn_tuple.dport);
    log_debug("%llu guy send pid %d %lu\n", pid_tgid, args.conn_tuple.pid, args.conn_tuple.netns);
    log_debug("%llu guy send metadata %d\n", pid_tgid, args.conn_tuple.metadata);
    protocol_t protocol = get_cached_protocol_or_default(&args.conn_tuple);
    if (protocol != PROTOCOL_UNKNOWN && protocol != PROTOCOL_UNCLASSIFIED) {
        goto final;
    }

    if (buffer_ptr == NULL) {
        goto final;
    }

    size_t buffer_final_size = buffer_size > CLASSIFICATION_MAX_BUFFER ? (CLASSIFICATION_MAX_BUFFER - 1):buffer_size;
    if (buffer_final_size == 0) {
        goto final;
    }

    char local_buffer_copy[CLASSIFICATION_MAX_BUFFER];
    bpf_memset(local_buffer_copy, 0, CLASSIFICATION_MAX_BUFFER);
    read_into_buffer1(local_buffer_copy, buffer_ptr, buffer_final_size);

    // detect protocol
    classify_protocol(&protocol, local_buffer_copy, buffer_final_size);
    if (protocol != PROTOCOL_UNKNOWN && protocol != PROTOCOL_UNCLASSIFIED) {
        log_debug("classified protocol %d", protocol);
        bpf_map_update_with_telemetry(connection_protocol, &args.conn_tuple, &protocol, BPF_NOEXIST);
    }

final:
    bpf_map_update_with_telemetry(tcp_sendmsg_args, &pid_tgid, &args, BPF_ANY);
}

#endif // __TRACER_EVENTS_H
