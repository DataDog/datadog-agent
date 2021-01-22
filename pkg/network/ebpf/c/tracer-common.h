#ifndef __TRACER_COMMON_H
#define __TRACER_COMMON_H

#include "tracer-maps.h"

static __always_inline int get_proto(conn_tuple_t * t) {
    return (t->metadata & CONN_TYPE_TCP) ? CONN_TYPE_TCP : CONN_TYPE_UDP;
}

static __always_inline void increment_telemetry_count(enum telemetry_counter counter_name) {
    __u64 key = 0;
    telemetry_t empty = {};
    telemetry_t* val;
    bpf_map_update_elem(&telemetry, &key, &empty, BPF_NOEXIST);
    val = bpf_map_lookup_elem(&telemetry, &key);

    if (val == NULL) {
        return;
    }
    switch (counter_name) {
    case tcp_sent_miscounts:
        __sync_fetch_and_add(&val->tcp_sent_miscounts, 1);
        break;
    case missed_tcp_close:
        __sync_fetch_and_add(&val->missed_tcp_close, 1);
        break;
    case missed_udp_close:
        __sync_fetch_and_add(&val->missed_udp_close, 1);
    case udp_send_processed:
        __sync_fetch_and_add(&val->udp_sends_processed, 1);
        break;
    case udp_send_missed:
        __sync_fetch_and_add(&val->udp_sends_missed, 1);
        break;
    }
    return;
}

static __always_inline void cleanup_conn(conn_tuple_t* tup) {
    u32 cpu = bpf_get_smp_processor_id();

    // Will hold the full connection data to send through the perf buffer
    conn_t conn = { .tup = *tup };
    tcp_stats_t* tst = NULL;
    conn_stats_ts_t* cst = NULL;
    bool is_tcp = get_proto(&conn.tup) == CONN_TYPE_TCP;
    bool is_udp = get_proto(&conn.tup) == CONN_TYPE_UDP;

    // TCP stats don't have the PID
    if (is_tcp) {
        conn.tup.pid = 0;
        tst = bpf_map_lookup_elem(&tcp_stats, &(conn.tup));
        bpf_map_delete_elem(&tcp_stats, &(conn.tup));
        conn.tup.pid = tup->pid;

        if (tst) {
            conn.tcp_stats = *tst;
        }

        conn.tcp_stats.state_transitions |= (1 << TCP_CLOSE);
    }

    cst = bpf_map_lookup_elem(&conn_stats, &(conn.tup));
    // Delete this connection from our stats map
    bpf_map_delete_elem(&conn_stats, &(conn.tup));

    if (cst) {
        cst->timestamp = bpf_ktime_get_ns();
        conn.conn_stats = *cst;
    }

    // Batch TCP closed connections before generating a perf event
    batch_t* batch_ptr = bpf_map_lookup_elem(&conn_close_batch, &cpu);
    if (batch_ptr == NULL) {
        return;
    }

    // TODO: Can we turn this into a macro based on TCP_CLOSED_BATCH_SIZE?
    switch (batch_ptr->pos) {
    case 0:
        batch_ptr->c0 = conn;
        batch_ptr->pos++;
        return;
    case 1:
        batch_ptr->c1 = conn;
        batch_ptr->pos++;
        return;
    case 2:
        batch_ptr->c2 = conn;
        batch_ptr->pos++;
        return;
    case 3:
        batch_ptr->c3 = conn;
        batch_ptr->pos++;
        return;
    case 4:
        // In this case the batch is ready to be flushed, which we defer to kretprobe/tcp_close
        // in order to cope with the eBPF stack limitation of 512 bytes.
        batch_ptr->c4 = conn;
        batch_ptr->pos++;
        return;
    }

    // If we hit this section it means we had one or more interleaved tcp_close calls.
    // This could result in a missed tcp_close event, so we track it using our telemetry map.
    if (is_tcp) increment_telemetry_count(missed_tcp_close);
    if (is_udp) increment_telemetry_count(missed_udp_close);
}

static __always_inline void sockaddr_to_addr(struct sockaddr * sa, u64 * addr_h, u64 * addr_l, u16 * port) {
    if (!sa) return;

    u16 family;
    bpf_probe_read(&family, sizeof(family), &sa->sa_family);

    struct sockaddr_in * sin;
    struct sockaddr_in6 * sin6;
    switch (family) {
    case AF_INET:
        sin = (struct sockaddr_in *) sa;
        if (addr_l) {
            bpf_probe_read(addr_l, sizeof(__be32), &(sin->sin_addr.s_addr));
        }
        if (port) {
            bpf_probe_read(port, sizeof(__be16), &sin->sin_port);
            *port = bpf_ntohs(*port);
        }
        break;
    case AF_INET6:
        sin6 = (struct sockaddr_in6 *) sa;
        if (addr_l && addr_h) {
            bpf_probe_read(addr_h, sizeof(u64), sin6->sin6_addr.s6_addr);
            bpf_probe_read(addr_l, sizeof(u64), &(sin6->sin6_addr.s6_addr[8]));
        }
        if (port) {
            bpf_probe_read(port, sizeof(u16), &sin6->sin6_port);
            *port = ntohs(*port);
        }
        break;
    }
}

static __always_inline void flush_conn_close_if_full(struct pt_regs * ctx) {
    u32 cpu = bpf_get_smp_processor_id();
    batch_t * batch_ptr = bpf_map_lookup_elem(&conn_close_batch, &cpu);
    if (!batch_ptr) {
        return;
    }

    if (batch_ptr->pos == CONN_CLOSED_BATCH_SIZE) {
        // Here we copy the batch data to a variable allocated in the eBPF stack
        // This is necessary for older Kernel versions only (we validated this behavior on 4.4.0),
        // since you can't directly write a map entry to the perf buffer.
        batch_t batch_copy = {};
        __builtin_memcpy(&batch_copy, batch_ptr, sizeof(batch_copy));
        batch_ptr->pos = 0;
        bpf_perf_event_output(ctx, &conn_close_event, cpu, &batch_copy, sizeof(batch_copy));
    }
}

#endif // __TRACER_COMMON_H
