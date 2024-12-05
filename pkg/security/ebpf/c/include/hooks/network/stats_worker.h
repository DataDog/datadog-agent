#ifndef _HOOKS_NETWORK_WORKER_H_
#define _HOOKS_NETWORK_WORKER_H_

struct ctx_holder {
    struct bpf_perf_event_data *ctx;
};

static long active_flows_callback_fn(struct bpf_map *map, const void *key, void *value, void *callback_ctx) {
    u32 pid = *(u32 *)key;
    struct active_flows_t *entry = (struct active_flows_t *) value;
    struct bpf_perf_event_data *ctx = ((struct ctx_holder *) callback_ctx)->ctx;
    u64 now = bpf_ktime_get_ns();
    struct network_stats_t *stats = NULL;
    struct namespaced_flow_t ns_flow_tmp = {};

    if (now < entry->last_sent + get_network_monitor_period()) {
        // we'll flush later, move on
        return 0;
    }

    struct network_monitor_event_t *evt = get_network_monitor_event();
    if (evt == NULL) {
        // should never happen
        return 0;
    }

    // Delete the entry now to try to limit race conditions with exiting processes.
    // Note that the "worse" that can happen with this race is that we send the same flows twice.
    bpf_map_delete_elem(&active_flows, &pid);

    // process context
    fill_network_process_context(&evt->process, pid, entry->netns);

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
            goto send;
        }
        ns_flow_tmp.netns = entry->netns;
        ns_flow_tmp.flow = entry->flows[i & (ACTIVE_FLOWS_MAX_SIZE - 1)];

        // query the stats
        stats = bpf_map_lookup_elem(&ns_flow_to_network_stats, &ns_flow_tmp);
        if (stats != NULL) {
            // Delete entry now to try to limit race conditions with "count_pkt" with other CPUs.
            // Note that the "worse" that can happen with this race is that we miss a couple of bytes / packets for the
            // current flow.
            bpf_map_delete_elem(&ns_flow_to_network_stats, &ns_flow_tmp);

            evt->flows[evt->flows_count & (ACTIVE_FLOWS_MAX_SIZE - 1)].flow = entry->flows[i & (ACTIVE_FLOWS_MAX_SIZE - 1)];
            evt->flows[i & (ACTIVE_FLOWS_MAX_SIZE - 1)].stats = *stats;
            evt->flows_count += 1;
        }
    }

send:
    // send event
    send_event_with_size_ptr(ctx, EVENT_NETWORK_MONITOR, evt, offsetof(struct network_monitor_event_t, flows) + (evt->flows_count & (ACTIVE_FLOWS_MAX_SIZE - 1)) * sizeof(struct flow_stats_t));
    bpf_printk("sent %d flows for pid %d!\n", evt->flows_count, pid);

    return 0;
}

SEC("perf_event/cpu_clock")
int network_stats_worker(struct bpf_perf_event_data *ctx)
{
    // we want only one worker for network stats
    if (bpf_get_smp_processor_id() > 0) {
        return 0;
    }

//    bpf_printk("stats_worker running ...\n");

    struct flow_queue_msg_t flow_msg = {};
    struct active_flows_t *entry;
    struct active_flows_t *zero = get_empty_active_flows();
    if (zero == NULL) {
        // should never happen, ignore
        return 0;
    }
    zero->last_sent = bpf_ktime_get_ns();
    struct ctx_holder holder = {};
    holder.ctx = ctx;
    int pop_ret = 0;

    // consume the queue, update the pid <-> active_flows map
#pragma unroll
    for (int i = 0; i < FLOW_MSG_PER_TICK_COUNT; i++) {
        pop_ret = bpf_map_pop_elem(&flows_queue, &flow_msg);
        if (pop_ret < 0) {
            // ignore, we're at the end of the queue !
            break;
        }

        if (flow_msg.ns_flow.flow.l3_protocol == 0 && flow_msg.ns_flow.flow.l4_protocol == 0) {
            bpf_printk("Empty packet in queue !\n");
        }

        // fetch active flows for current flow_msg
        entry = bpf_map_lookup_elem(&active_flows, &flow_msg.pid);
        if (entry == NULL) {
            bpf_map_update_elem(&active_flows, &flow_msg.pid, zero, BPF_ANY);
            entry = bpf_map_lookup_elem(&active_flows, &flow_msg.pid);
            if (entry == NULL) {
                goto next;
            }

            // set netns and ifindex
            entry->netns = flow_msg.ns_flow.netns;
            entry->ifindex = flow_msg.ifindex;
        }

        // if the entry full ?
        if (entry->cursor < ACTIVE_FLOWS_MAX_SIZE) {
            // add new flow to the list
            entry->flows[entry->cursor & (ACTIVE_FLOWS_MAX_SIZE - 1)] = flow_msg.ns_flow.flow;
            entry->cursor = entry->cursor + 1;
        } else {
            // for now, drop the flow.
            // TODO: send early and reset event or requeue event ?
//            bpf_printk("Dropping 1 flow for pid %d sport:%d dport:%d\n", flow_msg.pid, flow_msg.ns_flow.flow.sport, flow_msg.ns_flow.flow.dport);
//            bpf_printk(" - saddr:%x daddr:%x\n", flow_msg.ns_flow.flow.saddr[0], flow_msg.ns_flow.flow.daddr[0]);
//            bpf_printk(" - l4:%d l3:%d\n", flow_msg.ns_flow.flow.l4_protocol, flow_msg.ns_flow.flow.l3_protocol);
            continue;
        }
    }

next:
    // iterate over the list of active flows, send when need be
    bpf_for_each_map_elem(&active_flows, &active_flows_callback_fn, &holder, 0);

    return 0;
};

#endif
