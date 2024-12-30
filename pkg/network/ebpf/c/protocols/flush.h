#ifndef __USM_FLUSH_H
#define __USM_FLUSH_H

#include "bpf_bypass.h"

#include "protocols/http/http.h"
#include "protocols/http2/decoding.h"
#include "protocols/kafka/kafka-parsing.h"
#include "protocols/postgres/decoding.h"
#include "protocols/redis/decoding.h"

// flush all batched events to userspace for all protocols.
// because perf events can't be sent from socket filter programs.
static __always_inline void flush(void *ctx) {
    http_batch_flush(ctx);
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

SEC("raw_tracepoint/net/netif_receive_skb")
int BPF_PROG(raw_tracepoint__net__netif_receive_skb) {
    CHECK_BPF_PROGRAM_BYPASSED()
    log_debug("raw_tracepoint/net/netif_receive_skb");
    flush(ctx);
    return 0;
}

#endif
