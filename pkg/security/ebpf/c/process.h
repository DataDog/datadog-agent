#ifndef _PROCESS_H_
#define _PROCESS_H_

#include <linux/tty.h>
#include <linux/sched.h>

struct proc_cache_t {
    struct container_context_t container;
    struct file_t executable;

    u64 exec_timestamp;
    char tty_name[TTY_NAME_LEN];
    char comm[TASK_COMM_LEN];
};

struct bpf_map_def SEC("maps/proc_cache") proc_cache = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct proc_cache_t),
    .max_entries = 4096,
    .pinning = 0,
    .namespace = "",
};

struct pid_cache_t {
    u32 cookie;
    u32 ppid;
    u64 fork_timestamp;
    u64 exit_timestamp;
    u32 uid;
    u32 gid;
};

struct bpf_map_def SEC("maps/pid_cache") pid_cache = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct pid_cache_t),
    .max_entries = 4096,
    .pinning = 0,
    .namespace = "",
};

struct proc_cache_t * __attribute__((always_inline)) get_proc_cache(u32 tgid) {
    struct proc_cache_t *entry = NULL;

    struct pid_cache_t *pid_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &tgid);
    if (pid_entry) {
        // Select the cache entry
        u32 cookie = pid_entry->cookie;
        entry = bpf_map_lookup_elem(&proc_cache, &cookie);
    }
    return entry;
}

static struct proc_cache_t * __attribute__((always_inline)) fill_process_context(struct process_context_t *data) {
    // Pid & Tid
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    // https://github.com/iovisor/bcc/blob/master/docs/reference_guide.md#4-bpf_get_current_pid_tgid
    data->pid = tgid;
    data->tid = pid_tgid;

    // UID & GID
    u64 userid = bpf_get_current_uid_gid();
    data->uid = userid & 0xffffffff;
    data->gid = userid >> 32;

    return get_proc_cache(tgid);
}

#endif
