#ifndef _PROCESS_H_
#define _PROCESS_H_

#include <linux/tty.h>
#include <linux/sched.h>

#include "container.h"
#include "span.h"

struct proc_cache_t {
    struct container_context_t container;
    struct file_t executable;

    u64 exec_timestamp;
    char tty_name[TTY_NAME_LEN];
    char comm[TASK_COMM_LEN];
};

static __attribute__((always_inline)) u32 copy_tty_name(char src[TTY_NAME_LEN], char dst[TTY_NAME_LEN]) {
    if (src[0] == 0) {
        return 0;
    }

#pragma unroll
    for (int i = 0; i < TTY_NAME_LEN; i++)
    {
        dst[i] = src[i];
    }
    return TTY_NAME_LEN;
}

void __attribute__((always_inline)) copy_proc_cache_except_comm(struct proc_cache_t* src, struct proc_cache_t* dst) {
    copy_container_id(src->container.container_id, dst->container.container_id);
    dst->executable = src->executable;
    dst->exec_timestamp = src->exec_timestamp;
    copy_tty_name(src->tty_name, dst->tty_name);
}

void __attribute__((always_inline)) copy_proc_cache(struct proc_cache_t *src, struct proc_cache_t *dst) {
    copy_proc_cache_except_comm(src, dst);
    bpf_probe_read(dst->comm, TASK_COMM_LEN, src->comm);
    return;
}

struct bpf_map_def SEC("maps/proc_cache") proc_cache = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct proc_cache_t),
    .max_entries = 4096,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/pid_ns") pid_ns = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u64),
    .value_size = sizeof(u64),
    .max_entries = 32768,
    .pinning = 0,
    .namespace = "",
};

static void __attribute__((always_inline)) fill_container_context(struct proc_cache_t *entry, struct container_context_t *context) {
    if (entry) {
        copy_container_id(entry->container.container_id, context->container_id);
    }
}

struct credentials_t {
    u32 uid;
    u32 gid;
    u32 euid;
    u32 egid;
    u32 fsuid;
    u32 fsgid;
    u64 cap_effective;
    u64 cap_permitted;
};

void __attribute__((always_inline)) copy_credentials(struct credentials_t* src, struct credentials_t* dst) {
    *dst = *src;
}

struct pid_cache_t {
    u32 cookie;
    u32 ppid;
    u64 fork_timestamp;
    u64 exit_timestamp;
    struct credentials_t credentials;
};

void __attribute__((always_inline)) copy_pid_cache_except_exit_ts(struct pid_cache_t* src, struct pid_cache_t* dst) {
    dst->cookie = src->cookie;
    dst->ppid = src->ppid;
    dst->fork_timestamp = src->fork_timestamp;
    dst->credentials = src->credentials;
}

struct bpf_map_def SEC("maps/pid_cache") pid_cache = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct pid_cache_t),
    .max_entries = 4096,
    .pinning = 0,
    .namespace = "",
};

// defined in exec.h
struct proc_cache_t *get_proc_from_cookie(u32 cookie);

struct proc_cache_t * __attribute__((always_inline)) get_proc_cache(u32 tgid) {
    struct proc_cache_t *entry = NULL;

    struct pid_cache_t *pid_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &tgid);
    if (pid_entry) {
        // Select the cache entry
        u32 cookie = pid_entry->cookie;
        entry = get_proc_from_cookie(cookie);
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

    return get_proc_cache(tgid);
}

void __attribute__((always_inline)) register_pid_ns(u64 pid_tgid, u64 pid_tgid_ns) {
   bpf_map_update_elem(&pid_ns, &pid_tgid, &pid_tgid_ns, BPF_NOEXIST);
}

u64 __attribute__((always_inline)) lookup_pid_ns(u64 pid_tgid) {
    u64 *pid = bpf_map_lookup_elem(&pid_ns, &pid_tgid);
    return pid ? *pid : 0;
}

#endif
