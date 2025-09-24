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

__attribute__((always_inline)) int handle_pam_start(struct pt_regs *ctx)
{
    const char *service = (const char *)PT_REGS_PARM1(ctx);
    const char *user = (const char *)PT_REGS_PARM2(ctx);
    char service_name[3];
    bpf_probe_read(service_name, 3, (void *)service);
    // Register SSH Sessions
    if (bpf_strncmp(service_name,3 ,"ssh") == 0) {
        //bpf_printk("Event will be: %s, %s, %s\n", event->service, event->user, event->hostIP);
        register_ssh_user_session((char *) user);
    }    
    return 0;
}


// Handle pam_set_item via pt_regs; avoid pam_handle_t
__attribute__((always_inline)) int handle_pam_set_item(struct pt_regs *ctx)
{
    return 0;
}

__attribute__((always_inline)) int handle_pam_end(struct pt_regs *ctx)
{
    return 0;
}

__attribute__((always_inline)) int handle_pam_authenticate(struct pt_regs *ctx)
{
    bpf_printk("pam_authenticate called");
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


HOOK_UPROBE_ENTRY("pam_end")
int hook_pam_end(struct pt_regs *ctx)
{
    return handle_pam_end(ctx);
}

HOOK_UPROBE_ENTRY("pam_authenticate")
int hook_pam_authenticate(struct pt_regs *ctx)
{
    return handle_pam_authenticate(ctx);
}
#endif
