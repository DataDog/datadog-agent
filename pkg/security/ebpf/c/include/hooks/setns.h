#ifndef _HOOKS_SETNS_H_
#define _HOOKS_SETNS_H_

#include "constants/syscall_macro.h"
#include "helpers/discarders.h"
#include "helpers/syscalls.h"
#include "events_definition.h"

// setns accepts a nstype of 0, in which case the kernel resolves the type from the file
// descriptor (`flags = ns->ops->type`). The syscall arguments alone therefore don't tell us which
// namespace was joined, so the type is recovered from the per-namespace install callbacks below
// and merged into the reported nstype. These values are uapi and can never change; guarded
// because linux/sched.h may also define them.
#ifndef CLONE_NEWTIME
#define CLONE_NEWTIME 0x00000080
#endif
#ifndef CLONE_NEWNS
#define CLONE_NEWNS 0x00020000
#endif
#ifndef CLONE_NEWCGROUP
#define CLONE_NEWCGROUP 0x02000000
#endif
#ifndef CLONE_NEWUTS
#define CLONE_NEWUTS 0x04000000
#endif
#ifndef CLONE_NEWIPC
#define CLONE_NEWIPC 0x08000000
#endif
#ifndef CLONE_NEWUSER
#define CLONE_NEWUSER 0x10000000
#endif
#ifndef CLONE_NEWPID
#define CLONE_NEWPID 0x20000000
#endif
#ifndef CLONE_NEWNET
#define CLONE_NEWNET 0x40000000
#endif

static int __attribute__((always_inline)) sys_setns_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_SETNS);
    if (!syscall) {
        return 0;
    }

    // keep the denied attempts, they are as interesting as the successful ones
    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    // report the types the kernel installed rather than what the caller asked for: the two only
    // differ when the caller passed 0, since a non-zero nstype has to match the namespace the file
    // descriptor refers to or the syscall fails with EINVAL. Fall back to the requested value if
    // no install callback was seen, which is all we know in that case.
    u32 nstype = syscall->setns.effective_nstype;
    if (nstype == 0) {
        nstype = (u32)syscall->setns.nstype;
    }

    struct setns_event_t event = {
        .syscall.retval = retval,
        .fd = syscall->setns.fd,
        .nstype = nstype,
        .mntns_id = syscall->setns.mntns_id,
        .netns_id = syscall->setns.netns_id,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_cgroup_context(entry, &event.cgroup);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_SETNS, event);
    return 0;
}

HOOK_SYSCALL_ENTRY2(setns, int, fd, int, nstype) {
    if (is_discarded_by_pid()) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_SETNS,
        .setns = {
            .fd = fd,
            .nstype = nstype,
        }
    };

    cache_syscall_update_cgroup(ctx, &syscall);
    return 0;
}

HOOK_SYSCALL_EXIT(setns) {
    return sys_setns_ret(ctx, (int)SYSCALL_PARMRET(ctx));
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_setns_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_setns_ret(args, args->ret);
}

// The proc_ns_operations install callbacks are only reached from the setns syscall, and the
// function that fired identifies the namespace type without reading any kernel struct. They run
// before commit_nsset, so the syscall cache is still live. The flag is recorded even when the
// callback goes on to fail, so a denied join still reports which namespace type was attempted.
// A pidfd target installs several namespaces in one call, hence the accumulating OR.
static int __attribute__((always_inline)) handle_ns_install(u32 nstype) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_SETNS);
    if (!syscall) {
        return 0;
    }

    syscall->setns.effective_nstype |= nstype;
    return 0;
}

HOOK_ENTRY("mntns_install")
int hook_mntns_install(ctx_t *ctx) {
    return handle_ns_install(CLONE_NEWNS);
}

HOOK_ENTRY("netns_install")
int hook_netns_install(ctx_t *ctx) {
    return handle_ns_install(CLONE_NEWNET);
}

HOOK_ENTRY("pidns_install")
int hook_pidns_install(ctx_t *ctx) {
    return handle_ns_install(CLONE_NEWPID);
}

HOOK_ENTRY("userns_install")
int hook_userns_install(ctx_t *ctx) {
    return handle_ns_install(CLONE_NEWUSER);
}

HOOK_ENTRY("utsns_install")
int hook_utsns_install(ctx_t *ctx) {
    return handle_ns_install(CLONE_NEWUTS);
}

HOOK_ENTRY("ipcns_install")
int hook_ipcns_install(ctx_t *ctx) {
    return handle_ns_install(CLONE_NEWIPC);
}

HOOK_ENTRY("cgroupns_install")
int hook_cgroupns_install(ctx_t *ctx) {
    return handle_ns_install(CLONE_NEWCGROUP);
}

HOOK_ENTRY("timens_install")
int hook_timens_install(ctx_t *ctx) {
    return handle_ns_install(CLONE_NEWTIME);
}

#endif // _HOOKS_SETNS_H_
