#ifndef _HOOKS_PAM_H_
#define _HOOKS_PAM_H_

#include "constants/syscall_macro.h"
#include "constants/fentry_macro.h"
#include "helpers/syscalls.h"
#include "helpers/user_sessions.h"

__attribute__((always_inline)) int handle_pam_start(struct pt_regs *ctx)
{
    const char *service = (const char *)PT_REGS_PARM1(ctx);
    const char *user = (const char *)PT_REGS_PARM2(ctx);
    char service_name[3];
    bpf_probe_read(service_name, 3, (void *)service);
    // Register SSH User session 
    if (service_name[0] == 's' && service_name[1] == 's' && service_name[2] == 'h') {
        register_ssh_user_session((char *)user);
    }
    return 0;
}

HOOK_UPROBE_ENTRY("pam_start")
int hook_pam_start(struct pt_regs *ctx)
{
    return handle_pam_start(ctx);
}

#endif
