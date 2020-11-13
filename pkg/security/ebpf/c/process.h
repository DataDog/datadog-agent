#ifndef _PROCESS_H_
#define _PROCESS_H_

#include <linux/tty.h>
#include <linux/sched.h>

struct bpf_map_def SEC("maps/proc_cache") proc_cache = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct proc_cache_t),
    .max_entries = 4095,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/pid_cookie") pid_cookie = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 4097,
    .pinning = 0,
    .namespace = "",
};

struct proc_cache_t * __attribute__((always_inline)) get_pid_cache(u32 tgid) {
    struct proc_cache_t *entry = NULL;

    u32 *cookie = (u32 *) bpf_map_lookup_elem(&pid_cookie, &tgid);
    if (cookie) {
        // Select the old cache entry
        u32 cookie_key = *cookie;
        entry = bpf_map_lookup_elem(&proc_cache, &cookie_key);
    }
    return entry;
}

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
