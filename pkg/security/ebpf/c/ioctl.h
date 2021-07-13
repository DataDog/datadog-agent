#ifndef _IOCTL_H
#define _IOCTL_H

#include "erpc.h"

// the default SYSCALL_KPROBE macros don't allow to probe a syscall with more than 4 parameters
// this hack is a "solution" to this problem

#define IOCTL_ABI_HOOKx(x,word_size,type,TYPE,prefix,syscall,suffix,body,...) \
    int __attribute__((always_inline)) type##__##sys##syscall(struct pt_regs *ctx __JOIN(x,__SC_DECL,__VA_ARGS__)); \
    SEC(#type "/" SYSCALL##word_size##_PREFIX #prefix SYSCALL_PREFIX #syscall #suffix) \
    int type##__ ##word_size##_##prefix ##sys##syscall##suffix(struct pt_regs *ctx) { \
        SYSCALL_##TYPE##_PROLOG(x,__SC_##word_size##_PARAM,syscall,__VA_ARGS__) \
        body \
    }

#if USE_SYSCALL_WRAPPER == 1
  #define IOCTL_HOOKx(x,type,TYPE,prefix,name,body,...) \
    IOCTL_ABI_HOOKx(x,32,type,TYPE,prefix,name,,body,__VA_ARGS__) \
    IOCTL_ABI_HOOKx(x,64,type,TYPE,,name,,body,__VA_ARGS__)
#else
  #define IOCTL_HOOKx(x,type,TYPE,prefix,name,body,...) \
    IOCTL_ABI_HOOKx(x,64,type,TYPE,compat_,name,,body,__VA_ARGS__) \
    IOCTL_ABI_HOOKx(x,64,type,TYPE,,name,,body,__VA_ARGS__)
#endif

#define IOCTL_KPROBE6(name, body, ...) IOCTL_HOOKx(6,kprobe,KPROBE,,_##name,body,__VA_ARGS__)


IOCTL_KPROBE6(ioctl, {
    u8 op = UNKNOWN_OP;
    if (is_erpc_request(fd, cmd, &op)) {
        bpf_printk("a4 = %llx, a5 = %llx, a6 = %llx\n", a4, a5, a6);

#if defined(__x86_64__)
        return handle_erpc_request(ctx, op, (void *)arg);
#elif defined(__aarch64__)
        u64 data[4];
        data[0] = (u64)arg;
        data[1] = a4;
        data[2] = a5;
        data[3] = a6;
        return handle_erpc_request_arch_non_overlapping(ctx, op, (void *)data);
#else
  #error "Unsupported platform"
#endif
    }

    return 0;
}, int, fd, unsigned int, cmd, unsigned long, arg, u64, a4, u64, a5, u64, a6)

#endif


