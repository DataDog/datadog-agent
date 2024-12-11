#ifndef __REDIS_DECODING_H
#define __REDIS_DECODING_H

#include "protocols/redis/decoding-maps.h"

SEC("socket/redis_process")
int socket__redis_process(struct __sk_buff *skb) {
    return 0;
}

SEC("uprobe/redis_tls_process")
int uprobe__redis_tls_process(struct pt_regs *ctx) {
    return 0;
}

SEC("uprobe/redis_tls_termination")
int uprobe__redis_tls_termination(struct pt_regs *ctx) {
    return 0;
}

#endif /* __REDIS_DECODING_H */
