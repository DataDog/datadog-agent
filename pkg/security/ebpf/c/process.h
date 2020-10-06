#ifndef _PROCESS_H_
#define _PROCESS_H_

#include <linux/tty.h>
#include <linux/sched.h>

static struct proc_cache_t * __attribute__((always_inline)) fill_process_data(struct process_context_t *data) {
    // Comm
    bpf_get_current_comm(&data->comm, sizeof(data->comm));

    // Pid & Tid
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    // https://github.com/iovisor/bcc/blob/master/docs/reference_guide.md#4-bpf_get_current_pid_tgid
    data->pid = tgid;
    data->tid = pid_tgid;

    // UID & GID
    u64 userid = bpf_get_current_uid_gid();
    data->uid = userid >> 32;
    data->gid = userid;

    return NULL;
}

#endif
