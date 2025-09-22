#ifndef _HOOKS_PAM_H_
#define _HOOKS_PAM_H_

#include "constants/syscall_macro.h"
#include "constants/fentry_macro.h"
#include "helpers/syscalls.h"
#include "helpers/process.h"
#include "ktypes.h"
#include "bpf_metadata.h"
#include "bpf_tracing.h"
#include "bpf_helpers.h"
#include "bpf_helpers_custom.h"

#  define PAM_RHOST        4

__attribute__((always_inline)) int handle_pam_start(struct pt_regs *ctx)
{
    int key = 0;
    struct pam_event_t *event = bpf_map_lookup_elem(&pam_event, &key);
    if (!event) {
        return 0;
    }
    const char *service = (const char *)PT_REGS_PARM1(ctx);
    const char *user = (const char *)PT_REGS_PARM2(ctx);
    bpf_probe_read_str(&event->service, sizeof(event->service), service);
    bpf_probe_read_str(&event->user, sizeof(event->user), user);
    return 0;
}

__attribute__((always_inline)) int handle_pam_start_ret(struct pt_regs *ctx)
{
    return 0;

}

// Handle pam_set_item via pt_regs; avoid pam_handle_t
__attribute__((always_inline)) int handle_pam_set_item(struct pt_regs *ctx)
{
    int item_type         = (int)PT_REGS_PARM2(ctx);
    const void *item      = (const void *)PT_REGS_PARM3(ctx);
    int key = 0;
    struct pam_event_t *event = bpf_map_lookup_elem(&pam_event, &key);
    if (!event) {
        return 0;
    }
    struct proc_cache_t *entry = fill_process_context(&event->process);
    if (item && item_type == PAM_RHOST) {
        bpf_probe_read_user_str(&event->hostIP, sizeof(event->hostIP), item);
    }
    fill_cgroup_context(entry, &event->cgroup);
    fill_span_context(&event->span);
    // Register SSH Sessions
    if (bpf_strncmp(event->service,3 ,"ssh") == 0) {
        bpf_printk("Event will be: %s, %s, %s\n", event->service, event->user, event->hostIP);
        register_ssh_user_session(event);
    }
    bpf_map_delete_elem(&pam_event, &key);
    send_event(ctx, EVENT_PAM, *event);
    return 0;
}

HOOK_UPROBE_ENTRY("pam_set_item")
int hook_pam_set_item(struct pt_regs *ctx)
{
    return handle_pam_set_item(ctx);
}

HOOK_UPROBE_ENTRY("pam_start")
int hook_pam_start(struct pt_regs *ctx)
{
    return handle_pam_start(ctx);
}

HOOK_UPROBE_EXIT("pam_start")
int rethook_pam_start(struct pt_regs *ctx)
{
    return handle_pam_start_ret(ctx);
}

#endif
