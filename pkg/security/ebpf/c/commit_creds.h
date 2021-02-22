#ifndef _COMMIT_CREDS_H_
#define _COMMIT_CREDS_H_

struct credentials_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
    u32 chunk1;
    u32 chunk2;
    u32 chunk3;
    u32 chunk4;
};

int __attribute__((always_inline)) trace__credentials_update(u64 type) {
    struct syscall_cache_t syscall = {
        .type = type,
    };

    cache_syscall(&syscall);
    return 0;
}

int __attribute__((always_inline)) trace__credentials_update_ret(struct pt_regs *ctx) {
    int retval = PT_REGS_RC(ctx);
    if (retval < 0)
        return 0;

    struct syscall_cache_t *syscall = pop_syscall(SYSCALL_SETUID | SYSCALL_SETGID | SYSCALL_CAPSET);
    if (!syscall)
        return 0;

    u32 pid = bpf_get_current_pid_tgid() >> 32;
    struct pid_cache_t *pid_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &pid);
    if (!pid_entry) {
        return 0;
    }

    u64 event_type = 0;
    struct credentials_event_t event = {};
    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);

    switch (syscall->type) {
        case SYSCALL_SETUID:
            event_type = EVENT_SETUID;
            event.chunk1 = pid_entry->credentials.uid;
            event.chunk2 = pid_entry->credentials.euid;
            event.chunk3 = pid_entry->credentials.fsuid;
            break;
        case SYSCALL_SETGID:
            event_type = EVENT_SETGID;
            event.chunk1 = pid_entry->credentials.gid;
            event.chunk2 = pid_entry->credentials.egid;
            event.chunk3 = pid_entry->credentials.fsgid;
            break;
        case SYSCALL_CAPSET:
            event_type = EVENT_CAPSET;
            event.chunk1 = pid_entry->credentials.cap_effective >> 32;
            event.chunk2 = pid_entry->credentials.cap_effective;
            event.chunk3 = pid_entry->credentials.cap_permitted >> 32;
            event.chunk4 = pid_entry->credentials.cap_permitted;
            break;
    }

    send_event(ctx, event_type, event);
    return 0;
}

SYSCALL_KPROBE0(setuid) {
    return trace__credentials_update(SYSCALL_SETUID);
}

SYSCALL_KRETPROBE(setuid) {
    return trace__credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(seteuid) {
    return trace__credentials_update(SYSCALL_SETUID);
}

SYSCALL_KRETPROBE(seteuid) {
    return trace__credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setfsuid) {
    return trace__credentials_update(SYSCALL_SETUID);
}

SYSCALL_KRETPROBE(setfsuid) {
    return trace__credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setreuid) {
    return trace__credentials_update(SYSCALL_SETUID);
}

SYSCALL_KRETPROBE(setreuid) {
    return trace__credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setresuid) {
    return trace__credentials_update(SYSCALL_SETUID);
}

SYSCALL_KRETPROBE(setresuid) {
    return trace__credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setuid16) {
    return trace__credentials_update(SYSCALL_SETUID);
}

SYSCALL_KRETPROBE(setuid16) {
    return trace__credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(seteuid16) {
    return trace__credentials_update(SYSCALL_SETUID);
}

SYSCALL_KRETPROBE(seteuid16) {
    return trace__credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setfsuid16) {
    return trace__credentials_update(SYSCALL_SETUID);
}

SYSCALL_KRETPROBE(setfsuid16) {
    return trace__credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setreuid16) {
    return trace__credentials_update(SYSCALL_SETUID);
}

SYSCALL_KRETPROBE(setreuid16) {
    return trace__credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setresuid16) {
    return trace__credentials_update(SYSCALL_SETUID);
}

SYSCALL_KRETPROBE(setresuid16) {
    return trace__credentials_update_ret(ctx);
}



SYSCALL_KPROBE0(setgid) {
    return trace__credentials_update(SYSCALL_SETGID);
}

SYSCALL_KRETPROBE(setgid) {
    return trace__credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setegid) {
    return trace__credentials_update(SYSCALL_SETGID);
}

SYSCALL_KRETPROBE(setegid) {
    return trace__credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setfsgid) {
    return trace__credentials_update(SYSCALL_SETGID);
}

SYSCALL_KRETPROBE(setfsgid) {
    return trace__credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setregid) {
    return trace__credentials_update(SYSCALL_SETGID);
}

SYSCALL_KRETPROBE(setregid) {
    return trace__credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setresgid) {
    return trace__credentials_update(SYSCALL_SETGID);
}

SYSCALL_KRETPROBE(setresgid) {
    return trace__credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setgid16) {
    return trace__credentials_update(SYSCALL_SETGID);
}

SYSCALL_KRETPROBE(setgid16) {
    return trace__credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setegid16) {
    return trace__credentials_update(SYSCALL_SETGID);
}

SYSCALL_KRETPROBE(setegid16) {
    return trace__credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setfsgid16) {
    return trace__credentials_update(SYSCALL_SETGID);
}

SYSCALL_KRETPROBE(setfsgid16) {
    return trace__credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setregid16) {
    return trace__credentials_update(SYSCALL_SETGID);
}

SYSCALL_KRETPROBE(setregid16) {
    return trace__credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setresgid16) {
    return trace__credentials_update(SYSCALL_SETGID);
}

SYSCALL_KRETPROBE(setresgid16) {
    return trace__credentials_update_ret(ctx);
}



SYSCALL_KPROBE0(capset) {
    return trace__credentials_update(SYSCALL_CAPSET);
}

SYSCALL_KRETPROBE(capset) {
    return trace__credentials_update_ret(ctx);
}

SEC("kprobe/commit_creds")
int kprobe__commit_creds(struct pt_regs *ctx) {
    struct cred *credentials = (struct cred *)PT_REGS_PARM1(ctx);
    struct pid_cache_t new_pid_entry = {};

    // update pid_cache entry for the current process
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u8 new_entry = 0;
    struct pid_cache_t *pid_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &pid);
    if (!pid_entry) {
        new_entry = 1;
        pid_entry = &new_pid_entry;
    }
    if (!pid_entry) {
        return 0;
    }
    bpf_probe_read(&pid_entry->credentials.uid, sizeof(pid_entry->credentials.uid), &credentials->uid);
    bpf_probe_read(&pid_entry->credentials.gid, sizeof(pid_entry->credentials.uid), &credentials->gid);
    bpf_probe_read(&pid_entry->credentials.euid, sizeof(pid_entry->credentials.uid), &credentials->euid);
    bpf_probe_read(&pid_entry->credentials.egid, sizeof(pid_entry->credentials.uid), &credentials->egid);
    bpf_probe_read(&pid_entry->credentials.fsuid, sizeof(pid_entry->credentials.uid), &credentials->fsuid);
    bpf_probe_read(&pid_entry->credentials.fsgid, sizeof(pid_entry->credentials.uid), &credentials->fsgid);
    bpf_probe_read(&pid_entry->credentials.cap_effective, sizeof(pid_entry->credentials.uid), &credentials->cap_effective);
    bpf_probe_read(&pid_entry->credentials.cap_permitted, sizeof(pid_entry->credentials.uid), &credentials->cap_permitted);

    if (new_entry) {
        bpf_map_update_elem(&pid_cache, &pid, &new_pid_entry, BPF_ANY);
    }
    return 0;
}

#endif
