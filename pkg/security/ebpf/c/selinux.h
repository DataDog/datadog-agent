#ifndef _SELINUX_H_
#define _SELINUX_H_

#include "defs.h"
#include "filters.h"
#include "syscalls.h"
#include "process.h"

enum selinux_event_kind_t {
    SELINUX_BOOL_CHANGE_EVENT_KIND,
    SELINUX_ENFORCE_CHANGE_EVENT_KIND,
    SELINUX_DISABLE_CHANGE_EVENT_KIND,
    SELINUX_BOOL_COMMIT_EVENT_KIND,
};

struct selinux_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
    struct file_t file;
    u32 event_kind;
    u32 buf_size;
    char buf[SELINUX_BUF_LEN];
};

int __attribute__((always_inline)) handle_selinux_event(void *ctx, struct file *file, const char *buf, size_t count, enum selinux_event_kind_t kind) {
    struct syscall_cache_t syscall = {
        .type = EVENT_SELINUX,
        .policy = fetch_policy(EVENT_SELINUX),
        .selinux = {
            .event_kind = kind,
        },
    };

    struct dentry *dentry = get_file_dentry(file);
    syscall.selinux.dentry = dentry;
    syscall.selinux.file.path_key.mount_id = get_file_mount_id(file);

    size_t buf_size = SELINUX_BUF_LEN;
    if (count < SELINUX_BUF_LEN) {
        buf_size = count;
    }

    bpf_probe_read(syscall.selinux.buf, buf_size, (void *)buf);
    syscall.selinux.buf_size = buf_size;

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
    event.buf_size = syscall->selinux.buf_size;
    memcpy(event.buf, syscall->selinux.buf, SELINUX_BUF_LEN);
    event.file = syscall->selinux.file;

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

PROBE_SEL_WRITE_FUNC(sel_write_disable, SELINUX_DISABLE_CHANGE_EVENT_KIND)
PROBE_SEL_WRITE_FUNC(sel_write_enforce, SELINUX_ENFORCE_CHANGE_EVENT_KIND)
PROBE_SEL_WRITE_FUNC(sel_write_bool, SELINUX_BOOL_CHANGE_EVENT_KIND)
PROBE_SEL_WRITE_FUNC(sel_commit_bools_write, SELINUX_BOOL_COMMIT_EVENT_KIND)

#endif
