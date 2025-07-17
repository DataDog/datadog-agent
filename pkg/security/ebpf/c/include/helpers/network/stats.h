#ifndef _HELPERS_NETWORK_STATS_H_
#define _HELPERS_NETWORK_STATS_H_

#include "context.h"
#include "utils.h"

__attribute__((always_inline)) struct network_flow_monitor_event_t *get_network_flow_monitor_event() {
    u32 key = 0;
    struct network_flow_monitor_event_t *evt = bpf_map_lookup_elem(&network_flow_monitor_event_gen, &key);
    // __builtin_memset doesn't work here because evt is too large and memset is allocating too much memory
    return evt;
}

__attribute__((always_inline)) struct active_flows_t *get_empty_active_flows() {
    u32 key = 0;
    return bpf_map_lookup_elem(&active_flows_gen, &key);
}

__attribute__((always_inline)) int flush_network_stats(u32 pid, struct active_flows_t *entry, void *ctx, enum FLUSH_NETWORK_STATS_TYPE type) {
    u64 now = bpf_ktime_get_ns();
    struct network_stats_t *stats = NULL;
    struct namespaced_flow_t ns_flow_tmp = {};

    if (entry == NULL || ctx == NULL) {
        return 0;
    }

    if ((type == NETWORK_STATS_TICKER) && (now < entry->last_sent + get_network_monitor_period())) {
        // we'll flush later, move on
        return 0;
    }

    struct network_flow_monitor_event_t *evt = get_network_flow_monitor_event();
    if (evt == NULL) {
        // should never happen
        return 0;
    }
    evt->event.flags = EVENT_FLAGS_ACTIVITY_DUMP_SAMPLE;

    // Delete the entry now to try to limit race conditions with exiting processes.
    // Two races may happen here:
    // - we may send the same flows twice if both the ticker and the PID_EXIT hook points call this function
    // at the same time and both get a hold of the same active_flows_t *entry.
    // - we may miss some flows if a packet with a new flow is sent right when this function is called by the ticker,
    // and if the TC program that captures the new flow appends it to the ticker active_flows_t *entry after the end
    // of the unrolled loop.
    bpf_map_delete_elem(&active_flows, &pid);

    // process context
    fill_network_process_context(&evt->process, pid, entry->netns);

    u64 sched_cls_has_current_pid_tgid_helper = 0;
    LOAD_CONSTANT("sched_cls_has_current_pid_tgid_helper", sched_cls_has_current_pid_tgid_helper);
    if (sched_cls_has_current_pid_tgid_helper) {
        // reset and fill span context
        reset_span_context(&evt->span);
        fill_span_context(&evt->span);
    }

    // network context
    fill_network_device_context(&evt->device, entry->netns, entry->ifindex);

    struct proc_cache_t *proc_cache_entry = get_proc_cache(pid);
    if (proc_cache_entry == NULL) {
        evt->container.container_id[0] = 0;
    } else {
        copy_container_id_no_tracing(proc_cache_entry->container.container_id, &evt->container.container_id);
        evt->container.cgroup_context = proc_cache_entry->container.cgroup_context;
    }

    evt->flows_count = 0;

#pragma unroll
    for (int i = 0; i < ACTIVE_FLOWS_MAX_SIZE; i++) {
        if (i >= entry->cursor) {
            break;
        }
        ns_flow_tmp.netns = entry->netns;
        ns_flow_tmp.flow = entry->flows[i & (ACTIVE_FLOWS_MAX_SIZE - 1)];

        // start by copying the flow
        evt->flows[evt->flows_count & (ACTIVE_FLOWS_MAX_SIZE - 1)].flow = ns_flow_tmp.flow;

        // query the stats
        stats = bpf_map_lookup_elem(&ns_flow_to_network_stats, &ns_flow_tmp);
        if (stats != NULL) {
            // Delete entry now to try to limit race conditions with "count_pkt" with other CPUs.
            // Note that the "worse" that can happen with this race is that we miss a couple of bytes / packets for the
            // current flow.
            bpf_map_delete_elem(&ns_flow_to_network_stats, &ns_flow_tmp);
            evt->flows[evt->flows_count & (ACTIVE_FLOWS_MAX_SIZE - 1)].stats = *stats;
        } else {
            // we copied only the flow without the stats - better to get at least the flow than nothing at all
#if defined(DEBUG_NETWORK_FLOW)
            bpf_printk("no stats for sp:%d sa0:%lu sa1:%lu", ns_flow_tmp.flow.sport, ns_flow_tmp.flow.saddr[0], ns_flow_tmp.flow.saddr[1]);
            bpf_printk("             dp:%d da0:%lu da1:%lu", ns_flow_tmp.flow.dport, ns_flow_tmp.flow.daddr[0], ns_flow_tmp.flow.daddr[1]);
            bpf_printk("             netns:%lu l3:%d l4:%d", ns_flow_tmp.netns, ns_flow_tmp.flow.l3_protocol, ns_flow_tmp.flow.l4_protocol);
#endif
        }

        evt->flows_count += 1;
    }

    // send event
    send_event_with_size_ptr(ctx, EVENT_NETWORK_FLOW_MONITOR, evt, offsetof(struct network_flow_monitor_event_t, flows) + (evt->flows_count & (ACTIVE_FLOWS_MAX_SIZE - 1)) * sizeof(struct flow_stats_t));

#if defined(DEBUG_NETWORK_FLOW)
    bpf_printk("sent %d (out of %d) flows for pid %d!", evt->flows_count, entry->cursor, pid);
    bpf_printk("   - type: %d", type);
#endif

    return 0;
}

__attribute__((always_inline)) void flush_pid_network_stats(u32 pid, void *ctx, enum FLUSH_NETWORK_STATS_TYPE type) {
    struct active_flows_t *entry = bpf_map_lookup_elem(&active_flows, &pid);
    flush_network_stats(pid, entry, ctx, type);
}

__attribute__((always_inline)) void count_pkt(struct __sk_buff *skb, struct packet_t *pkt) {
    struct namespaced_flow_t ns_flow = pkt->translated_ns_flow;
    if (pkt->network_direction == INGRESS) {
        // EGRESS was arbitrarily chosen as "the 5-tuple order for indexing flow statistics".
        // Reverse ingress flow now
        flip(&ns_flow.flow);
    }

    u8 should_register_flow = 0;
    struct network_stats_t *stats = NULL;
    struct network_stats_t stats_zero = {};
    u64 now = bpf_ktime_get_ns();
    int ret = bpf_map_update_elem(&ns_flow_to_network_stats, &ns_flow, &stats_zero, BPF_NOEXIST);
    if (ret == 0) {
        // register flow in active_flows
        should_register_flow = 1;
    }

    // lookup the existing (or new) entry (now that it has been created)
    stats = bpf_map_lookup_elem(&ns_flow_to_network_stats, &ns_flow);
    if (stats == NULL) {
        // should never happen, ignore
        return;
    }

#if defined(DEBUG_NETWORK_FLOW)
    bpf_printk("added stats for sp:%d sa0:%lu sa1:%lu", ns_flow.flow.sport, ns_flow.flow.saddr[0], ns_flow.flow.saddr[1]);
    bpf_printk("                dp:%d da0:%lu da1:%lu", ns_flow.flow.dport, ns_flow.flow.daddr[0], ns_flow.flow.daddr[1]);
    bpf_printk("                netns:%lu l3:%d l4:%d", ns_flow.netns, ns_flow.flow.l3_protocol, ns_flow.flow.l4_protocol);
#endif

    // update stats
    switch (pkt->network_direction) {
        case EGRESS: {
            __sync_fetch_and_add(&stats->egress.pkt_count, 1);
            __sync_fetch_and_add(&stats->egress.data_size, skb->len);
            break;
        }
        case INGRESS: {
            __sync_fetch_and_add(&stats->ingress.pkt_count, 1);
            __sync_fetch_and_add(&stats->ingress.data_size, skb->len);
            break;
        }
    }

    if (should_register_flow) {
        // make sure we hold the spin lock for the active flows entry
        struct active_flows_spin_lock_t init_value = {};
        struct active_flows_spin_lock_t *active_flows_lock;
        bpf_map_update_elem(&active_flows_spin_locks, &pkt->pid, &init_value, BPF_NOEXIST);
        active_flows_lock = bpf_map_lookup_elem(&active_flows_spin_locks, &pkt->pid);
        if (active_flows_lock == NULL) {
            // shouldn't happen, ignore
            return;
        }

        struct active_flows_t *entry;
        struct active_flows_t *zero = get_empty_active_flows();
        if (zero == NULL) {
            // should never happen, ignore
            return;
        }
        zero->netns = ns_flow.netns;
        zero->ifindex = skb->ifindex;
        zero->last_sent = now;

        // make sure the active_flows entry for the current pid exists
        ret = bpf_map_update_elem(&active_flows, &pkt->pid, zero, BPF_NOEXIST);
        if (ret < 0 && ret != -EEXIST) {
            // no more space in the map, ignore for now
            return;
        }

        // lookup active_flows for current pid
        entry = bpf_map_lookup_elem(&active_flows, &pkt->pid);
        if (entry == NULL) {
            // should not happen, ignore
            return;
        }

        // is the entry full ?
        bpf_spin_lock(&active_flows_lock->lock);
        if (entry->cursor < ACTIVE_FLOWS_MAX_SIZE) {
            // add new flow to the list
            entry->flows[entry->cursor & (ACTIVE_FLOWS_MAX_SIZE - 1)] = ns_flow.flow;
            entry->cursor = entry->cursor + 1;
        } else {
            // TODO: send early and reset entry ?
            // for now, drop the flow.
        }
        bpf_spin_unlock(&active_flows_lock->lock);
        bpf_map_delete_elem(&active_flows_spin_locks, &pkt->pid);
    }
}

#endif
