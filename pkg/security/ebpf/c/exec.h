#ifndef _EXEC_H_
#define _EXEC_H_

#include <linux/tty.h>

#include "filters.h"
#include "syscalls.h"
#include "container.h"

struct exec_event_t {
    struct kevent_t event;
    struct proc_cache_t cache_entry;
    u32 pid;
    u32 padding;
};

struct exit_event_t {
    struct kevent_t event;
    u32 pid;
    u32 padding;
};

struct _tracepoint_sched_process_fork
{
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;

    char parent_comm[16];
    pid_t parent_pid;
    char child_comm[16];
    pid_t child_pid;
};

void __attribute__((always_inline)) copy_proc_cache(struct proc_cache_t *dst, struct proc_cache_t *src) {
    dst->executable = src->executable;
    copy_container_id(dst->container.container_id, src->container.container_id);
    return;
}

int __attribute__((always_inline)) trace__sys_execveat() {
    struct syscall_cache_t syscall = {
        .type = SYSCALL_EXEC,
    };

    cache_syscall(&syscall, EVENT_EXEC);
    return 0;
}

SYSCALL_KPROBE0(execve) {
    return trace__sys_execveat();
}

SYSCALL_KPROBE0(execveat) {
    return trace__sys_execveat();
}

int __attribute__((always_inline)) handle_exec_event(struct pt_regs *ctx, struct syscall_cache_t *syscall) {
    struct file *file = (struct file *)PT_REGS_PARM1(ctx);
    struct inode *inode = (struct inode *)PT_REGS_PARM2(ctx);
    struct path *path = &file->f_path;

    syscall->open.dentry = get_file_dentry(file);
    syscall->open.path_key = get_inode_key_path(inode, &file->f_path);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    u32 cookie = bpf_get_prandom_u32();

    struct proc_cache_t entry = {
        .executable = {
            .inode = syscall->open.path_key.ino,
            .overlay_numlower = get_overlay_numlower(get_path_dentry(path)),
            .mount_id = get_path_mount_id(path),
            .path_id = cookie,
        },
        .container = {},
        .timestamp = bpf_ktime_get_ns(),
        .cookie = cookie,
    };

    // select parent cache entry
    struct proc_cache_t *parent_entry = get_pid_cache(tgid);
    if (parent_entry) {
        // inherit container ID
        copy_container_id(entry.container.container_id, parent_entry->container.container_id);
    }
    syscall->open.path_key.path_id = cookie;

    // cache dentry
    resolve_dentry(syscall->open.dentry, syscall->open.path_key, 0);

    // insert new proc cache entry
    bpf_map_update_elem(&proc_cache, &cookie, &entry, BPF_ANY);

    // insert pid <-> cookie mapping
    bpf_map_update_elem(&pid_cookie, &tgid, &cookie, BPF_ANY);

    pop_syscall(SYSCALL_EXEC);

    return 0;
}

SEC("tracepoint/sched/sched_process_fork")
int sched_process_fork(struct _tracepoint_sched_process_fork *args) {
    u32 pid = 0;
    u32 ppid = 0;
    bpf_probe_read(&pid, sizeof(pid), &args->child_pid);
    bpf_probe_read(&ppid, sizeof(ppid), &args->parent_pid);

    struct proc_cache_t *parent_entry = get_pid_cache(ppid);
    if (parent_entry) {
        u32 cookie = parent_entry->cookie;

        // Ensures pid and ppid point to the same cookie
        bpf_map_update_elem(&pid_cookie, &pid, &cookie, BPF_ANY);
    }

    return 0;
}

SEC("kprobe/do_exit")
int kprobe_do_exit(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;
    u32 pid = pid_tgid;

    if (tgid == pid) {
        // send the entry to maintain userspace cache
        struct exit_event_t event = {
            .event.type = EVENT_EXIT,
            .pid = tgid
        };

        send_process_events(ctx, event);
    }
    return 0;
}

SEC("kprobe/exit_itimers")
int kprobe_exit_itimers(struct pt_regs *ctx) {
    struct signal_struct *signal = (struct signal_struct *)PT_REGS_PARM1(ctx);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct proc_cache_t *entry = get_pid_cache(tgid);
    if (entry) {
        struct tty_struct *tty;
        bpf_probe_read(&tty, sizeof(tty), &signal->tty);

        bpf_probe_read_str(entry->tty_name, TTY_NAME_LEN, tty->name);
    }

    return 0;
}

static __attribute__((always_inline)) u32 copy_tty_name(char dst[TTY_NAME_LEN], char src[TTY_NAME_LEN]) {
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

SEC("kprobe/do_close_on_exec")
int kprobe_do_close_on_exec(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct proc_cache_t *entry = get_pid_cache(tgid);
    if (entry) {
        struct exec_event_t event = {
            .event.type = EVENT_EXEC,
            .pid = tgid,
            .cache_entry.executable = {
                .inode = entry->executable.inode,
                .overlay_numlower = entry->executable.overlay_numlower,
                .mount_id = entry->executable.mount_id,
                .path_id = entry->executable.path_id,
            },
            .cache_entry.container = {},
            .cache_entry.timestamp = entry->timestamp,
            .cache_entry.cookie = entry->cookie,
        };

        copy_tty_name(event.cache_entry.tty_name, entry->tty_name);
        copy_container_id(event.cache_entry.container.container_id, entry->container.container_id);

        // send the entry to maintain userspace cache
        send_process_events(ctx, event);
    }

    return 0;
}
#endif
