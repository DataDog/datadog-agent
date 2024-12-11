#ifndef _HOOKS_NAMESPACES_H_
#define _HOOKS_NAMESPACES_H_

#include "constants/offsets/netns.h"
#include "maps.h"

HOOK_ENTRY("switch_task_namespaces")
int hook_switch_task_namespaces(ctx_t *ctx) {
    struct nsproxy *new_ns = (struct nsproxy *)CTX_PARM2(ctx);
    if (new_ns == NULL) {
        return 0;
    }

    struct net *net;
    bpf_probe_read(&net, sizeof(net), &new_ns->net_ns);
    if (net == NULL) {
        return 0;
    }

    u32 netns = get_netns_from_net(net);
    u32 tid = bpf_get_current_pid_tgid();
    bpf_map_update_elem(&netns_cache, &tid, &netns, BPF_ANY);
    return 0;
}

#endif
