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

    void *mnt_ns;
    bpf_probe_read(&mnt_ns, sizeof(mnt_ns), &new_ns->mnt_ns);
    if (mnt_ns != NULL) {
        u32 inum = 0;
        bpf_probe_read(&inum, sizeof(inum), (void *)mnt_ns + get_mount_offset_of_nscommon_inum());

        u32 pid = bpf_get_current_pid_tgid() >> 32;
        bpf_map_update_elem(&mntns_cache, &pid, &inum, BPF_ANY);
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
