#ifndef _DEFS_H
#define _DEFS_H 1

#include "../../ebpf/c/bpf_helpers.h"

#define TTY_NAME_LEN 64

# define printk(fmt, ...)						\
		({							\
			char ____fmt[] = fmt;				\
			bpf_trace_printk(____fmt, sizeof(____fmt),	\
				     ##__VA_ARGS__);			\
		})

enum event_type
{
    EVENT_MAY_OPEN = 1,
    EVENT_VFS_MKDIR,
    EVENT_VFS_LINK,
    EVENT_VFS_RENAME,
    EVENT_VFS_SETATTR,
    EVENT_VFS_UNLINK,
    EVENT_VFS_RMDIR,
};

struct event_t {
    u64 type;
    u64 timestamp;
    u64 retval;
};

struct event_context_t {
    char comm[TASK_COMM_LEN];
};

struct process_data_t {
    // Process data
    u64  pidns;
    char comm[TASK_COMM_LEN];
    char tty_name[TTY_NAME_LEN];
    u32  pid;
    u32  tid;
    u32  uid;
    u32  gid;
};

struct dentry_event_cache_t {
    struct event_context_t event_context;
    struct inode *src_dir;
    struct dentry *src_dentry;
    struct inode *target_dir;
    struct dentry *target_dentry;
    int mode;
    int flags;
};

struct process_discriminator_t {
    char comm[TASK_COMM_LEN];
};

struct path_leaf_t {
  u32 parent;
  char name[NAME_MAX];
};

struct bpf_map_def SEC("maps/dentry_event_cache") dentry_event_cache = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(u64),
    .value_size = sizeof(struct dentry_event_cache_t),
    .max_entries = 256,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/process_discriminators") process_discriminators = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(struct process_discriminator_t),
    .value_size = sizeof(u8),
    .max_entries = 256,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/dentry_events") dentry_events = {
    .type = BPF_MAP_TYPE_PERF_EVENT_ARRAY,
    .key_size = sizeof(__u32),
    .value_size = sizeof(__u32),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

void __attribute__((always_inline)) push_dentry_event_cache(struct dentry_event_cache_t *event) {
    u64 key = bpf_get_current_pid_tgid();
    bpf_map_update_elem(&dentry_event_cache, &key, event, BPF_ANY);
}

struct dentry_event_cache_t* __attribute__((always_inline)) pop_dentry_event_cache() {
    u64 key = bpf_get_current_pid_tgid();
    struct dentry_event_cache_t *event = bpf_map_lookup_elem(&dentry_event_cache, &key);
    if (!event)
        return NULL;
    bpf_map_delete_elem(&dentry_event_cache, &key);
    return event;
}

void __attribute__((always_inline)) fill_event_context(struct event_context_t *event_context) {
    bpf_get_current_comm(&event_context->comm, sizeof(event_context->comm));
}

static inline int filter(struct event_context_t *event_context) {
    int found = bpf_map_lookup_elem(&process_discriminators, &event_context->comm) != 0;
    if (found) {
        printk("Process filter found for %s\n", event_context->comm);
    }
    return !found;
}

#define send_event(ctx, event) \
    bpf_perf_event_output(ctx, &dentry_events, bpf_get_smp_processor_id(), &event, sizeof(event))

#endif