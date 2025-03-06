#ifndef __USM_FLUSH_H
#define __USM_FLUSH_H

#include "bpf_bypass.h"

#include "protocols/http/http.h"
#include "protocols/http2/decoding.h"
#include "protocols/kafka/kafka-parsing.h"
#include "protocols/postgres/decoding.h"
#include "protocols/redis/decoding.h"


SEC("tracepoint/net/netif_receive_skb")
int tracepoint__net__netif_receive_skb_http(void *ctx) {
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

SEC("tracepoint/net/netif_receive_skb")
int tracepoint__net__netif_receive_skb_postgres(void *ctx) {
    postgres_batch_flush_with_telemetry(ctx);
    return 0;
}

SEC("tracepoint/net/netif_receive_skb")
int tracepoint__net__netif_receive_skb_redis(void *ctx) {
    redis_batch_flush_with_telemetry(ctx);
    return 0;
}

#endif // __USM_FLUSH_H
