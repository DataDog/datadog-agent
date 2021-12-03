#ifndef __PREBUILT_GET_TLS_BASE_H
#define __PREBUILT_GET_TLS_BASE_H

#include <linux/sched.h>

#include "bpf_helpers.h"

static __always_inline void* get_tls_base(struct task_struct* task) {
    // TODO implement
    //      Offset guessing should be doable
    //      since the value of the TLS base is easily accessible from user-space,
    //      which can be scanned for in the `struct task_struct` value
    //      (since `struct thread_struct` is embedded within)
    return NULL;
}

#endif //__PREBUILT_GET_TLS_BASE_H

