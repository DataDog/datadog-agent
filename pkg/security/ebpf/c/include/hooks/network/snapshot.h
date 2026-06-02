#ifndef _HOOKS_NETWORK_SNAPSHOT_H_
#define _HOOKS_NETWORK_SNAPSHOT_H_

#include "constants/offsets/network.h"
#include "constants/offsets/process.h"
#include "helpers/network/flow.h"

#ifndef DO_NOT_USE_TC

// bpf_iter__task_file is the context passed to `iter/task_file` programs. It is
// defined in the kernel (kernel/bpf/task_iter.c) but is not exported through any
// UAPI header, so we declare it here for the non-CO-RE build. The field layout
// matches the kernel ABI (meta@0, task@8, fd@16, file@24 on 64-bit); the verifier
// validates ctx field accesses against the real BTF type by offset.
#ifndef COMPILE_CORE
struct bpf_iter_meta;

struct bpf_iter__task_file {
    struct bpf_iter_meta *meta;
    struct task_struct *task;
    u32 fd;
    struct file *file;
};
#endif

// bpf_iter__task_file_resolve_flow_pid walks every (task, fd, file) tuple open on
// the system and, for each socket file, records a
// (netns, source addr, source port, l4 protocol) -> pid entry in the flow_pid map.
//
// It is attached and run once, from userspace, during the snapshot phase (see
// EBPFResolvers.snapshotFlowPid). This is the bulk equivalent of the
// security_sk_classify_flow hook, which only fires for sockets created *after* the
// probe is attached: without this snapshot, packets of sockets that pre-existed the
// probe load have no PID attribution — most visibly on ingress, where the
// bpf_get_current_pid_tgid() fallback runs in softirq context and flow_pid is the
// only reliable source of the owning PID.
SEC("iter/task_file")
int bpf_iter__task_file_resolve_flow_pid(struct bpf_iter__task_file *ctx) {
    struct task_struct *task = ctx->task;
    struct file *file = ctx->file;
    if (task == NULL || file == NULL) {
        return 0;
    }

    // bpf_sock_from_file (kernel 5.11+) returns NULL for non-socket fds
    struct socket *sock = bpf_sock_from_file(file);
    if (sock == NULL) {
        return 0;
    }
    struct sock *sk = get_sock_from_socket(sock);

    // resolve the thread group id (tgid) from the task.
    // (the iter runs in the context of system-probe, so we can't use
    // bpf_get_current_pid_tgid() to identify the socket owner)
    u32 tgid = get_root_nr_from_task_struct(task);

    register_flow_pid_for_sock(sk, tgid);

    return 0;
}

#endif // DO_NOT_USE_TC

#endif
