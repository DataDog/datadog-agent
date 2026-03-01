#ifndef __USM_FLUSH_H
#define __USM_FLUSH_H

#include "bpf_bypass.h"

#include "protocols/http/http.h"
#include "protocols/http2/decoding.h"
#include "protocols/kafka/kafka-parsing.h"
#include "protocols/postgres/decoding.h"
#include "protocols/redis/decoding.h"
#include "protocols/tls/connection-close-events.h"

/**
Note - We used to have a single tracepoint to flush all the protocols, but we had to split it
to enable telemetry for all protocols.

However, kernel 4.14 does not support multiple programs to hook the same tracepoint, hence
we move into kprobes to workaround that.

The kprobe we use is '__netif_receive_skb_core', which is hook-able in several kernels
including 4.14, but it is not supported for kprobe hooking in kernels 6+.

To simplify the scenario, we have a support for 4.14 based on kprobes, and 4.15+ will be using
the tracepoints.

http2 is supported only from kernel 5.2, therefore it does not have the kprobe version
*/

SEC("tracepoint/net/netif_receive_skb")
int tracepoint__net__netif_receive_skb_http(void *ctx) {
    http_batch_flush_with_telemetry(ctx);
    return 0;
}

SEC("kprobe/__netif_receive_skb_core")
int netif_receive_skb_core_http_4_14(void *ctx) {
    http_batch_flush_with_telemetry(ctx);
    return 0;
}

SEC("tracepoint/net/netif_receive_skb")
int tracepoint__net__netif_receive_skb_http2(void *ctx) {
    http2_batch_flush_with_telemetry(ctx);
    terminated_http2_batch_flush_with_telemetry(ctx);
    return 0;
}

SEC("tracepoint/net/netif_receive_skb")
int tracepoint__net__netif_receive_skb_kafka(void *ctx) {
    kafka_batch_flush_with_telemetry(ctx);
    return 0;
}

SEC("kprobe/__netif_receive_skb_core")
int netif_receive_skb_core_kafka_4_14(void *ctx) {
    kafka_batch_flush_with_telemetry(ctx);
    return 0;
}

SEC("tracepoint/net/netif_receive_skb")
int tracepoint__net__netif_receive_skb_postgres(void *ctx) {
    postgres_batch_flush_with_telemetry(ctx);
    return 0;
}

SEC("kprobe/__netif_receive_skb_core")
int netif_receive_skb_core_postgres_4_14(void *ctx) {
    postgres_batch_flush_with_telemetry(ctx);
    return 0;
}

SEC("tracepoint/net/netif_receive_skb")
int tracepoint__net__netif_receive_skb_redis(void *ctx) {
    if (is_redis_with_key_monitoring_enabled()) {
        redis_with_key_batch_flush_with_telemetry(ctx);
    } else {
        redis_batch_flush_with_telemetry(ctx);
    }
    return 0;
}

SEC("kprobe/__netif_receive_skb_core")
int netif_receive_skb_core_redis_4_14(void *ctx) {
    if (is_redis_with_key_monitoring_enabled()) {
        redis_with_key_batch_flush_with_telemetry(ctx);
    } else {
        redis_batch_flush_with_telemetry(ctx);
    }
    return 0;
}

SEC("tracepoint/net/netif_receive_skb")
int tracepoint__net__netif_receive_skb_tcp_close(void *ctx) {
    tcp_close_batch_flush_with_telemetry(ctx);
    return 0;
}

//SEC("kprobe/__netif_receive_skb_core")
//int netif_receive_skb_core_tcp_close_4_14(void *ctx) {
//    tcp_close_batch_flush_with_telemetry(ctx);
//    return 0;
//}

#endif // __USM_FLUSH_H
