#ifndef __RUNTIME_GET_TLS_BASE_H
#define __RUNTIME_GET_TLS_BASE_H

#include <linux/sched.h>

#include "bpf_helpers.h"

static __always_inline void* get_tls_base(struct task_struct* task) {
    #if defined(__x86_64__)
        #if LINUX_VERSION_CODE < KERNEL_VERSION(4, 7, 0)
            return (void*) task->thread.fs;
        #else
            return (void*) task->thread.fsbase;
        #endif
    #elif defined(__aarch64__)
        #if LINUX_VERSION_CODE < KERNEL_VERSION(4, 17, 0)
            return (void*) task->thread.tp_value;
        #else
            return (void*) task->thread.uw.tp_value;
        #endif
    #else
        #error "Unsupported platform"
    #endif
}

#endif //__RUNTIME_GET_TLS_BASE_H

