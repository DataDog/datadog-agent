#ifndef _HOOKS_SYSCTL_H_
#define _HOOKS_SYSCTL_H_

#include "helpers/sysctl.h"
#include "helpers/sysctl.h"

HOOK_ENTRY("proc_sys_call_handler")
int hook_proc_sys_call_handler(ctx_t *ctx) {
    reset_sysctl_event();
    return 0;
}

SEC("cgroup/sysctl")
int cgroup_sysctl(struct bpf_sysctl *ctx) {
    handle_cgroup_sysctl(ctx);
    // make sure we don't disrupt the sysctl command
    return SYSCTL_OK;
}

#endif
