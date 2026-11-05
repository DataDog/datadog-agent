#ifndef _HELPERS_NETWORK_IMDS_H
#define _HELPERS_NETWORK_IMDS_H

#include "constants/enums.h"
#include "helpers/network/context.h"
#include "helpers/network/credentials.h"
#include "helpers/network/pid_resolver.h"
#include "helpers/process.h"
#include "maps.h"


// Resolve the local recipient rather than trusting an skb cookie that could
// still identify the sender. Keep egress event attribution unchanged: user
// space must not attach credentials to the sender of a response.
static __always_inline void resolve_imds_ingress_pid(struct __sk_buff *skb, struct packet_t *pkt) {
    if (pkt->network_direction != INGRESS) {
        return;
    }

    pkt->pid = 0;
    struct namespaced_flow_t *nsf = &pkt->translated_ns_flow;
    if (nsf->netns != pkt->ns_flow.netns) {
        return;
    }
    if (is_sk_lookup_pid_enabled()) {
        resolve_pid_from_sk_lookup(skb, pkt);
    } else {
        // Legacy flow_pid resolution is best-effort. Only accept an exact
        // connected local route, never a wildcard listener or procfs guess.
        struct pid_route_t route = {
            .addr = {nsf->flow.daddr[0], nsf->flow.daddr[1]},
            .netns = pkt->ns_flow.netns,
            .port = nsf->flow.tcp_udp.dport,
            .l4_protocol = nsf->flow.l4_protocol,
        };
        struct pid_route_entry_t *entry = bpf_map_lookup_elem(&flow_pid, &route);
        if (entry != NULL && entry->type == FLOW_CLASSIFICATION_ENTRY && entry->owner_sk != NULL) {
            pkt->pid = entry->pid;
        }
    }

    u32 pid = (u32)pkt->pid;
    if (pid != 0 && IS_KERNEL_THREAD(pid)) {
        pkt->pid = 0;
    }
}

static __always_inline struct imds_event_t *get_imds_event() {
    u32 key = IMDS_EVENT_KEY;
    return bpf_map_lookup_elem(&imds_event, &key);
}

static __always_inline struct imds_event_t *reset_imds_event(struct __sk_buff *skb, struct packet_t *pkt) {
    struct imds_event_t *evt = get_imds_event();
    if (evt == NULL) {
        // should never happen
        return NULL;
    }

    // reset event flags
    evt->event.flags = 0;

    // process context
    resolve_imds_ingress_pid(skb, pkt);
    fill_network_process_context_from_pkt(&evt->process, pkt);

    // reset and fill span context unconditionally
    reset_span_context(&evt->span, &evt->go_labels);

    u64 sched_cls_has_current_pid_tgid_helper = 0;
    LOAD_CONSTANT("sched_cls_has_current_pid_tgid_helper", sched_cls_has_current_pid_tgid_helper);
    if (sched_cls_has_current_pid_tgid_helper) {
        fill_span_context(&evt->span, &evt->go_labels);
    }

    // network context
    fill_network_context(&evt->network, skb, pkt);

    // which credential endpoint served this event ?
    evt->credential_source = get_credential_source(pkt);

    struct proc_cache_t *entry = get_proc_cache(evt->process.pid);
    fill_cgroup_context(entry, &evt->cgroup);

    // should we sample this event for activity dumps ?
    struct activity_dump_config *config = lookup_or_delete_traced_pid(evt->process.pid, bpf_ktime_get_ns(), NULL);
    if (config) {
        if (mask_has_event(config->event_mask, EVENT_IMDS)) {
            evt->event.flags |= EVENT_FLAGS_ACTIVITY_DUMP_SAMPLE;
        }
    }

    return evt;
}

#endif
