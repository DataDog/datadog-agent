#ifndef __ERPC_DISPATCHER_H
#define __ERPC_DISPATCHER_H

#include "bpf_helpers.h"
#include "protocols/tls/java/types.h"
#include "protocols/tls/java/maps.h"

#define USM_IOCTL_ID 0xda7ad09

static int __always_inline is_usm_erpc_request(struct pt_regs *ctx) {
    u32 cmd = PT_REGS_PARM3(ctx);
    return cmd == USM_IOCTL_ID;
};

/*
  handle_erpc_request ioctl request format :

  struct {
      u8           operation;  // see erpc_message_type enum for supported operations
      u8           data[];     // payload data
  }
*/

static void __always_inline handle_erpc_request(struct pt_regs *ctx) {
    #ifdef DEBUG
        u64 pid_tgid = bpf_get_current_pid_tgid();
        u64 pid = pid_tgid >> 32;
    #endif

    void *req = (void *)PT_REGS_PARM4(ctx);

    u8 op = 0;
    if (0 != bpf_probe_read_user(&op, sizeof(op), req)){
        log_debug("[java_tls_handle_erpc_request] failed to parse opcode of java tls erpc request for: pid %d\n", pid);
        return;
    }

    //for easier troubleshooting in case we get out of sync between java tracer's side of the erpc and systemprobe's side
    #ifdef DEBUG
        log_debug("[java_tls_handle_erpc_request] received %d op\n", op);
        if (op >= MAX_MESSAGE_TYPE){
            log_debug("[java_tls_handle_erpc_request] got unsupported erpc request %x for: pid %d\n",op, pid);
        }
    #endif

    bpf_tail_call_compat(ctx, &java_tls_erpc_handlers, op);
}

SEC("kprobe/do_vfs_ioctl")
int kprobe__do_vfs_ioctl(struct pt_regs *ctx) {
    if (is_usm_erpc_request(ctx)) {
        handle_erpc_request(ctx);
    }

    return 0;
}

#endif // __ERPC_DISPATCHER_H
