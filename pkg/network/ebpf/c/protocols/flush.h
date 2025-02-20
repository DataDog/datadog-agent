#ifndef __USM_FLUSH_H
#define __USM_FLUSH_H

#include "bpf_bypass.h"

#include "protocols/http/http.h"
#include "protocols/http2/decoding.h"
#include "protocols/kafka/kafka-parsing.h"
#include "protocols/postgres/decoding.h"
#include "protocols/redis/decoding.h"
#include "protocols/tls/native-tls-maps.h"

// flush all batched events to userspace for all protocols.
// because perf events can't be sent from socket filter programs.
static __always_inline void flush(void *ctx) {
    http_batch_flush_with_telemetry(ctx);
    http2_batch_flush(ctx);
    terminated_http2_batch_flush(ctx);
    kafka_batch_flush(ctx);
    postgres_batch_flush(ctx);
    redis_batch_flush(ctx);
}

SEC("tracepoint/net/netif_receive_skb")
int tracepoint__net__netif_receive_skb(void *ctx) {
    CHECK_BPF_PROGRAM_BYPASSED()
    log_debug("tracepoint/net/netif_receive_skb");
    flush(ctx);
    return 0;
}

SEC("tracepoint/sched/sched_process_exit")
int tracepoint__sched__sched_process_exit(void *ctx) {
    CHECK_BPF_PROGRAM_BYPASSED()
    u64 pid_tgid = bpf_get_current_pid_tgid();

    bpf_map_delete_elem(&ssl_read_args, &pid_tgid);
    bpf_map_delete_elem(&ssl_read_ex_args, &pid_tgid);

    return 0;
}

#endif
