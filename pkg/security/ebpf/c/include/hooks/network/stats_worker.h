#ifndef _HOOKS_NETWORK_WORKER_H_
#define _HOOKS_NETWORK_WORKER_H_

struct ctx_holder {
    struct bpf_perf_event_data *ctx;
};

static long active_flows_callback_fn(struct bpf_map *map, const void *key, void *value, void *callback_ctx) {
    u32 pid = *(u32 *)key;
    struct active_flows_t *entry = (struct active_flows_t *) value;
    struct bpf_perf_event_data *ctx = ((struct ctx_holder *) callback_ctx)->ctx;
    return flush_network_stats(pid, entry, ctx, NETWORK_STATS_TICKER);
}

SEC("perf_event/cpu_clock")
int network_stats_worker(struct bpf_perf_event_data *ctx)
{
    // we want only one worker for network stats
    if (bpf_get_smp_processor_id() > 0) {
        return 0;
    }
    struct ctx_holder holder = {};
    holder.ctx = ctx;

    // iterate over the list of active flows, send when need be
    bpf_for_each_map_elem(&active_flows, &active_flows_callback_fn, &holder, 0);

    return 0;
};

#endif
