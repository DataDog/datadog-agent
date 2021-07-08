#ifndef _SELINUX_H_
#define _SELINUX_H_

#include "defs.h"
#include "filters.h"
#include "syscalls.h"
#include "process.h"

enum selinux_event_kind_t {
    SELINUX_BOOL_CHANGE_EVENT_KIND,
    SELINUX_STATUS_CHANGE_EVENT_KIND,
    SELINUX_BOOL_COMMIT_EVENT_KIND,
};

struct selinux_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
    struct file_t file;
    u32 event_kind;
    u32 value; // 1 for true, 0 for false, -1 (max) for error
};

#define SELINUX_WRITE_BUFFER_LEN 64

struct selinux_write_buffer_t {
    char buffer[SELINUX_WRITE_BUFFER_LEN];
};

struct bpf_map_def SEC("maps/selinux_write_buffer") selinux_write_buffer = {
    .type = BPF_MAP_TYPE_PERCPU_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct selinux_write_buffer_t),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

int __attribute__((always_inline)) parse_buf_to_bool(const char *buf) {
    u32 key = 0;
    struct selinux_write_buffer_t *copy = bpf_map_lookup_elem(&selinux_write_buffer, &key);
    if (!copy) {
        return -1;
    }
    int read_status = bpf_probe_read_str(&copy->buffer, SELINUX_WRITE_BUFFER_LEN, (void *)buf);
    if (!read_status) {
        return -1;
    }

    #pragma unroll
    for (size_t i = 0; i < SELINUX_WRITE_BUFFER_LEN; i++) {
        char curr = copy->buffer[i];
        if (curr == 0) {
            return 0;
        } else if ('0' < curr && curr <= '9') {
            return 1;
        } else if (curr != '0') {
            return 0;
        }
    }

    return 0;
}

int __attribute__((always_inline)) handle_selinux_event(void *ctx, struct file *file, const char *buf, size_t count, enum selinux_event_kind_t kind) {
    struct syscall_cache_t syscall = {
        .type = EVENT_SELINUX,
        .policy = fetch_policy(EVENT_SELINUX),
        .selinux = {
            .event_kind = kind,
            .value = -1,
        },
    };

    struct dentry *dentry = get_file_dentry(file);
    syscall.selinux.dentry = dentry;
    syscall.selinux.file.path_key.mount_id = get_file_mount_id(file);

    if (count < SELINUX_WRITE_BUFFER_LEN) {
        syscall.selinux.value = parse_buf_to_bool(buf);
    }
    // otherwise let's keep the value = error state.

    fill_file_metadata(syscall.selinux.dentry, &syscall.selinux.file.metadata);
    set_file_inode(syscall.selinux.dentry, &syscall.selinux.file, 0);

    syscall.resolver.key = syscall.selinux.file.path_key;
    syscall.resolver.dentry = syscall.selinux.dentry;
    syscall.resolver.discarder_type = syscall.policy.mode != NO_FILTER ? EVENT_SELINUX : 0;
    syscall.resolver.callback = DR_SELINUX_CALLBACK_KPROBE_KEY;
    syscall.resolver.iteration = 0;
    syscall.resolver.ret = 0;

    cache_syscall(&syscall);

    // tail call
    resolve_dentry(ctx, DR_KPROBE);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_SELINUX);

    return 0;
}

int __attribute__((always_inline)) dr_selinux_callback(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_SELINUX);
    if (!syscall)
        return 0;

    if (syscall->resolver.ret == DENTRY_DISCARDED || syscall->resolver.ret == DENTRY_INVALID)
        return 0;

    struct selinux_event_t event = {};
    event.event_kind = syscall->selinux.event_kind;
    event.file = syscall->selinux.file;
    event.value = syscall->selinux.value;

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);

    send_event(ctx, EVENT_SELINUX, event);
    return 0;
}

SEC("kprobe/dr_selinux_callback")
int __attribute__((always_inline)) kprobe_dr_selinux_callback(struct pt_regs *ctx) {
    int retval = PT_REGS_RC(ctx);
    return dr_selinux_callback(ctx, retval);
}

#define PROBE_SEL_WRITE_FUNC(func_name, kind)                  \
    SEC("kprobe/" #func_name)                                  \
    int kprobe__##func_name(struct pt_regs *ctx) {             \
        struct file *file = (struct file *)PT_REGS_PARM1(ctx); \
        const char *buf = (const char *)PT_REGS_PARM2(ctx);    \
        size_t count = (size_t)PT_REGS_PARM3(ctx);             \
        /* selinux only supports ppos = 0 */                   \
        return handle_selinux_event(ctx, file, buf, count, (kind));    \
    }

PROBE_SEL_WRITE_FUNC(sel_write_disable, SELINUX_STATUS_CHANGE_EVENT_KIND)
PROBE_SEL_WRITE_FUNC(sel_write_enforce, SELINUX_STATUS_CHANGE_EVENT_KIND)
PROBE_SEL_WRITE_FUNC(sel_write_bool, SELINUX_BOOL_CHANGE_EVENT_KIND)
PROBE_SEL_WRITE_FUNC(sel_commit_bools_write, SELINUX_BOOL_COMMIT_EVENT_KIND)

#endif
