#ifndef _HELPERS_NETWORK_CONTEXT_H_
#define _HELPERS_NETWORK_CONTEXT_H_

__attribute__((always_inline)) void fill_network_process_context(struct process_context_t *process, u32 pid, u32 netns) {
    if (pid >= 0) {
        process->pid = pid;
        process->tid = pid;
    } else {
        process->pid = 0;
        process->tid = 0;
    }
    process->netns = netns;
}

__attribute__((always_inline)) void fill_network_process_context_from_pkt(struct process_context_t *process, struct packet_t *pkt) {
    fill_network_process_context(process, pkt->pid, pkt->translated_ns_flow.netns);
}

__attribute__((always_inline)) void fill_network_device_context(struct network_device_context_t *device_ctx, u32 netns, u32 ifindex) {
    device_ctx->netns = netns;
    device_ctx->ifindex = ifindex;
}

__attribute__((always_inline)) void fill_network_device_context_from_pkt(struct network_device_context_t *device_ctx, struct __sk_buff *skb, struct packet_t *pkt) {
    fill_network_device_context(device_ctx, pkt->translated_ns_flow.netns, skb->ifindex);
}

__attribute__((always_inline)) void fill_network_context(struct network_context_t *net_ctx, struct __sk_buff *skb, struct packet_t *pkt) {
    net_ctx->size = skb->len;
    net_ctx->network_direction = pkt->network_direction;
    net_ctx->flow = pkt->translated_ns_flow.flow;

    fill_network_device_context_from_pkt(&net_ctx->device, skb, pkt);
}

#endif
