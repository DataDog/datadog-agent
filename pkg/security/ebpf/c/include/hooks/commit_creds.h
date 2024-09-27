#ifndef _HOOKS_COMMIT_CREDS_H_
#define _HOOKS_COMMIT_CREDS_H_

#include "constants/syscall_macro.h"
#include "helpers/syscalls.h"
#include "helpers/events_predicates.h"

int __attribute__((always_inline)) credentials_update(u64 type) {
    struct syscall_cache_t syscall = {
        .type = type,
    };

    cache_syscall(&syscall);
    return 0;
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

HOOK_SYSCALL_ENTRY0(setuid) {
    return credentials_update(EVENT_SETUID);
}

HOOK_SYSCALL_EXIT(setuid) {
    int retval = SYSCALL_PARMRET(ctx);
    return credentials_update_ret(ctx, retval);
}

HOOK_SYSCALL_ENTRY0(setfsuid) {
    return credentials_update(EVENT_SETUID);
}

HOOK_SYSCALL_EXIT(setfsuid) {
    int retval = SYSCALL_PARMRET(ctx);
    return credentials_update_ret(ctx, retval);
}

HOOK_SYSCALL_ENTRY0(setreuid) {
    return credentials_update(EVENT_SETUID);
}

HOOK_SYSCALL_EXIT(setreuid) {
    int retval = SYSCALL_PARMRET(ctx);
    return credentials_update_ret(ctx, retval);
}

HOOK_SYSCALL_ENTRY0(setresuid) {
    return credentials_update(EVENT_SETUID);
}

HOOK_SYSCALL_EXIT(setresuid) {
    int retval = SYSCALL_PARMRET(ctx);
    return credentials_update_ret(ctx, retval);
}

HOOK_SYSCALL_ENTRY0(setuid16) {
    return credentials_update(EVENT_SETUID);
}

HOOK_SYSCALL_EXIT(setuid16) {
    int retval = SYSCALL_PARMRET(ctx);
    return credentials_update_ret(ctx, retval);
}

HOOK_SYSCALL_ENTRY0(setfsuid16) {
    return credentials_update(EVENT_SETUID);
}

HOOK_SYSCALL_EXIT(setfsuid16) {
    int retval = SYSCALL_PARMRET(ctx);
    return credentials_update_ret(ctx, retval);
}

HOOK_SYSCALL_ENTRY0(setreuid16) {
    return credentials_update(EVENT_SETUID);
}

HOOK_SYSCALL_EXIT(setreuid16) {
    int retval = SYSCALL_PARMRET(ctx);
    return credentials_update_ret(ctx, retval);
}

HOOK_SYSCALL_ENTRY0(setresuid16) {
    return credentials_update(EVENT_SETUID);
}

HOOK_SYSCALL_EXIT(setresuid16) {
    int retval = SYSCALL_PARMRET(ctx);
    return credentials_update_ret(ctx, retval);
}

HOOK_SYSCALL_ENTRY0(setgid) {
    return credentials_update(EVENT_SETGID);
}

HOOK_SYSCALL_EXIT(setgid) {
    int retval = SYSCALL_PARMRET(ctx);
    return credentials_update_ret(ctx, retval);
}

HOOK_SYSCALL_ENTRY0(setfsgid) {
    return credentials_update(EVENT_SETGID);
}

HOOK_SYSCALL_EXIT(setfsgid) {
    int retval = SYSCALL_PARMRET(ctx);
    return credentials_update_ret(ctx, retval);
}

HOOK_SYSCALL_ENTRY0(setregid) {
    return credentials_update(EVENT_SETGID);
}

HOOK_SYSCALL_EXIT(setregid) {
    int retval = SYSCALL_PARMRET(ctx);
    return credentials_update_ret(ctx, retval);
}

HOOK_SYSCALL_ENTRY0(setresgid) {
    return credentials_update(EVENT_SETGID);
}

HOOK_SYSCALL_EXIT(setresgid) {
    int retval = SYSCALL_PARMRET(ctx);
    return credentials_update_ret(ctx, retval);
}

HOOK_SYSCALL_ENTRY0(setgid16) {
    return credentials_update(EVENT_SETGID);
}

HOOK_SYSCALL_EXIT(setgid16) {
    int retval = SYSCALL_PARMRET(ctx);
    return credentials_update_ret(ctx, retval);
}

HOOK_SYSCALL_ENTRY0(setfsgid16) {
    return credentials_update(EVENT_SETGID);
}

HOOK_SYSCALL_EXIT(setfsgid16) {
    int retval = SYSCALL_PARMRET(ctx);
    return credentials_update_ret(ctx, retval);
}

HOOK_SYSCALL_ENTRY0(setregid16) {
    return credentials_update(EVENT_SETGID);
}

HOOK_SYSCALL_EXIT(setregid16) {
    int retval = SYSCALL_PARMRET(ctx);
    return credentials_update_ret(ctx, retval);
}

HOOK_SYSCALL_ENTRY0(setresgid16) {
    return credentials_update(EVENT_SETGID);
}

HOOK_SYSCALL_EXIT(setresgid16) {
    int retval = SYSCALL_PARMRET(ctx);
    return credentials_update_ret(ctx, retval);
}

HOOK_SYSCALL_ENTRY0(capset) {
    return credentials_update(EVENT_CAPSET);
}

HOOK_SYSCALL_EXIT(capset) {
    int retval = SYSCALL_PARMRET(ctx);
    return credentials_update_ret(ctx, retval);
}

SEC("tracepoint/handle_sys_commit_creds_exit")
int tracepoint_handle_sys_commit_creds_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return credentials_update_ret(args, args->ret);
}

struct __attribute__((__packed__)) cred_ids {
    kuid_t uid;
    kgid_t gid;
    kuid_t suid;
    kgid_t sgid;
    kuid_t euid;
    kgid_t egid;
    kuid_t fsuid;
    kgid_t fsgid;
};

struct __attribute__((__packed__)) cred_caps {
    kernel_cap_t cap_inheritable;
    kernel_cap_t cap_permitted;
    kernel_cap_t cap_effective;
    kernel_cap_t cap_bset;
    kernel_cap_t cap_ambient;
};

HOOK_ENTRY("commit_creds")
int hook_commit_creds(ctx_t *ctx) {
    u64 creds_uid_offset;
    LOAD_CONSTANT("creds_uid_offset", creds_uid_offset);
    struct cred_ids *credentials = (struct cred_ids *)(CTX_PARM1(ctx) + creds_uid_offset);

    u64 creds_cap_inheritable_offset;
    LOAD_CONSTANT("creds_cap_inheritable_offset", creds_cap_inheritable_offset);
    struct cred_caps *capabilities = (struct cred_caps *)(CTX_PARM1(ctx) + creds_cap_inheritable_offset);

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
    bpf_probe_read(&pid_entry->credentials.cap_effective, sizeof(pid_entry->credentials.cap_effective), &capabilities->cap_effective);
    bpf_probe_read(&pid_entry->credentials.cap_permitted, sizeof(pid_entry->credentials.cap_permitted), &capabilities->cap_permitted);

    if (new_entry) {
        bpf_map_update_elem(&pid_cache, &pid, &new_pid_entry, BPF_ANY);
    }
    return 0;
}

#endif
