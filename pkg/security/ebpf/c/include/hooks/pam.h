#ifndef _HOOKS_SSH_H_
#define _HOOKS_SSH_H_

#include "constants/syscall_macro.h"
#include "helpers/syscalls.h"
#include "helpers/process.h"
#include "ktypes.h"
#include "bpf_metadata.h"
#include "bpf_tracing.h"
#include "bpf_helpers.h"
#include "bpf_helpers_custom.h"

__attribute__((always_inline)) int handle_pam_start(struct pt_regs *ctx)
{
        struct pam_event_t event = {
            .flags = syscall->async ? EVENT_FLAGS_ASYNC : 0;
    };

    struct policy_t policy = fetch_policy(EVENT_PAM);
    struct proc_cache_t *entry = fill_process_context(&event->process);
    fill_cgroup_context(entry, &event->cgroup);
    fill_span_context(&event->span);

    const char *service = (const char *)PT_REGS_PARM1(ctx);
    const char *user    = (const char *)PT_REGS_PARM2(ctx);

    
    bpf_probe_read_user_str(event.service, sizeof(svc), service);
    bpf_probe_read_user_str(event.user, sizeof(usr), user);

    bpf_printk("PAM_START: service=%s user=%s", svc, usr);

    send_event(ctx, EVENT_PAM, event);

    return 0;
}

// aarch64 - ARM64
SEC("uprobe/pam_start")
int hook_pam_start(struct pt_regs *ctx)
{
    return handle_pam_start(ctx);
}

#endif
