#ifndef _COMMIT_CREDS_H_
#define _COMMIT_CREDS_H_

struct setuid_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    u32 uid;
    u32 euid;
    u32 fsuid;
};

struct setgid_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    u32 gid;
    u32 egid;
    u32 fsgid;
};

struct capset_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    u64 cap_effective;
    u64 cap_permitted;
};

int __attribute__((always_inline)) credentials_update(u64 type) {
    struct syscall_cache_t syscall = {
        .type = type,
    };

    cache_syscall(&syscall);
    return 0;
}

int __attribute__((always_inline)) credentials_predicate(u64 type) {
    return type == EVENT_SETUID || type == EVENT_SETGID || type == EVENT_CAPSET;
}

int __attribute__((always_inline)) credentials_update_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall_with(credentials_predicate);
    if (!syscall) {
        return 0;
    }

    if (retval < 0) {
        return 0;
    }

    u32 pid = bpf_get_current_pid_tgid() >> 32;
    struct pid_cache_t *pid_entry = (struct pid_cache_t *)bpf_map_lookup_elem(&pid_cache, &pid);
    if (!pid_entry) {
        return 0;
    }

    switch (syscall->type) {
    case EVENT_SETUID: {
        struct setuid_event_t event = {};
        struct proc_cache_t *entry = fill_process_context(&event.process);
        fill_container_context(entry, &event.container);
        fill_span_context(&event.span);

        event.uid = pid_entry->credentials.uid;
        event.euid = pid_entry->credentials.euid;
        event.fsuid = pid_entry->credentials.fsuid;
        send_event(ctx, EVENT_SETUID, event);
        break;
    }
    case EVENT_SETGID: {
        struct setgid_event_t event = {};
        struct proc_cache_t *entry = fill_process_context(&event.process);
        fill_container_context(entry, &event.container);
        fill_span_context(&event.span);

        event.gid = pid_entry->credentials.gid;
        event.egid = pid_entry->credentials.egid;
        event.fsgid = pid_entry->credentials.fsgid;
        send_event(ctx, EVENT_SETGID, event);
        break;
    }
    case EVENT_CAPSET: {
        struct capset_event_t event = {};
        struct proc_cache_t *entry = fill_process_context(&event.process);
        fill_container_context(entry, &event.container);
        fill_span_context(&event.span);

        event.cap_effective = pid_entry->credentials.cap_effective;
        event.cap_permitted = pid_entry->credentials.cap_permitted;
        send_event(ctx, EVENT_CAPSET, event);
        break;
    }
    }

    return 0;
}

int __attribute__((always_inline)) kprobe_credentials_update_ret(struct pt_regs *ctx) {
    int retval = PT_REGS_RC(ctx);
    return credentials_update_ret(ctx, retval);
}

SYSCALL_KPROBE0(setuid) {
    return credentials_update(EVENT_SETUID);
}

SYSCALL_KRETPROBE(setuid) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(seteuid) {
    return credentials_update(EVENT_SETUID);
}

SYSCALL_KRETPROBE(seteuid) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setfsuid) {
    return credentials_update(EVENT_SETUID);
}

SYSCALL_KRETPROBE(setfsuid) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setreuid) {
    return credentials_update(EVENT_SETUID);
}

SYSCALL_KRETPROBE(setreuid) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setresuid) {
    return credentials_update(EVENT_SETUID);
}

SYSCALL_KRETPROBE(setresuid) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setuid16) {
    return credentials_update(EVENT_SETUID);
}

SYSCALL_KRETPROBE(setuid16) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(seteuid16) {
    return credentials_update(EVENT_SETUID);
}

SYSCALL_KRETPROBE(seteuid16) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setfsuid16) {
    return credentials_update(EVENT_SETUID);
}

SYSCALL_KRETPROBE(setfsuid16) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setreuid16) {
    return credentials_update(EVENT_SETUID);
}

SYSCALL_KRETPROBE(setreuid16) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setresuid16) {
    return credentials_update(EVENT_SETUID);
}

SYSCALL_KRETPROBE(setresuid16) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setgid) {
    return credentials_update(EVENT_SETGID);
}

SYSCALL_KRETPROBE(setgid) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setegid) {
    return credentials_update(EVENT_SETGID);
}

SYSCALL_KRETPROBE(setegid) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setfsgid) {
    return credentials_update(EVENT_SETGID);
}

SYSCALL_KRETPROBE(setfsgid) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setregid) {
    return credentials_update(EVENT_SETGID);
}

SYSCALL_KRETPROBE(setregid) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setresgid) {
    return credentials_update(EVENT_SETGID);
}

SYSCALL_KRETPROBE(setresgid) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setgid16) {
    return credentials_update(EVENT_SETGID);
}

SYSCALL_KRETPROBE(setgid16) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setegid16) {
    return credentials_update(EVENT_SETGID);
}

SYSCALL_KRETPROBE(setegid16) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setfsgid16) {
    return credentials_update(EVENT_SETGID);
}

SYSCALL_KRETPROBE(setfsgid16) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setregid16) {
    return credentials_update(EVENT_SETGID);
}

SYSCALL_KRETPROBE(setregid16) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(setresgid16) {
    return credentials_update(EVENT_SETGID);
}

SYSCALL_KRETPROBE(setresgid16) {
    return kprobe_credentials_update_ret(ctx);
}

SYSCALL_KPROBE0(capset) {
    return credentials_update(EVENT_CAPSET);
}

SYSCALL_KRETPROBE(capset) {
    return kprobe_credentials_update_ret(ctx);
}

SEC("tracepoint/handle_sys_commit_creds_exit")
int tracepoint_handle_sys_commit_creds_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return credentials_update_ret(args, args->ret);
}

struct cred_ids {
    kuid_t uid;
    kgid_t gid;
    kuid_t suid;
    kgid_t sgid;
    kuid_t euid;
    kgid_t egid;
    kuid_t fsuid;
    kgid_t fsgid;
    unsigned securebits;
    kernel_cap_t cap_inheritable;
    kernel_cap_t cap_permitted;
    kernel_cap_t cap_effective;
    kernel_cap_t cap_bset;
    kernel_cap_t cap_ambient;
};

SEC("kprobe/commit_creds")
int kprobe_commit_creds(struct pt_regs *ctx) {
    u64 creds_uid_offset;
    LOAD_CONSTANT("creds_uid_offset", creds_uid_offset);
    struct cred_ids *credentials = (struct cred_ids *)(PT_REGS_PARM1(ctx) + creds_uid_offset);
    struct pid_cache_t new_pid_entry = {};

    // update pid_cache entry for the current process
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u8 new_entry = 0;
    struct pid_cache_t *pid_entry = (struct pid_cache_t *)bpf_map_lookup_elem(&pid_cache, &pid);
    if (!pid_entry) {
        new_entry = 1;
        pid_entry = &new_pid_entry;
    }
    if (!pid_entry) {
        return 0;
    }
    bpf_probe_read(&pid_entry->credentials.uid, sizeof(pid_entry->credentials.uid), &credentials->uid);
    bpf_probe_read(&pid_entry->credentials.gid, sizeof(pid_entry->credentials.gid), &credentials->gid);
    bpf_probe_read(&pid_entry->credentials.euid, sizeof(pid_entry->credentials.euid), &credentials->euid);
    bpf_probe_read(&pid_entry->credentials.egid, sizeof(pid_entry->credentials.egid), &credentials->egid);
    bpf_probe_read(&pid_entry->credentials.fsuid, sizeof(pid_entry->credentials.fsuid), &credentials->fsuid);
    bpf_probe_read(&pid_entry->credentials.fsgid, sizeof(pid_entry->credentials.fsgid), &credentials->fsgid);
    bpf_probe_read(&pid_entry->credentials.cap_effective, sizeof(pid_entry->credentials.cap_effective), &credentials->cap_effective);
    bpf_probe_read(&pid_entry->credentials.cap_permitted, sizeof(pid_entry->credentials.cap_permitted), &credentials->cap_permitted);

    if (new_entry) {
        bpf_map_update_elem(&pid_cache, &pid, &new_pid_entry, BPF_ANY);
    }
    return 0;
}

#endif
