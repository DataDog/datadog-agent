#ifndef __GO_TLS_GOID_H
#define __GO_TLS_GOID_H

#include "ktypes.h"
#if defined(COMPILE_PREBUILT) || defined(COMPILE_RUNTIME)
#include "kconfig.h"
#include <linux/sched.h>
#endif

#include "bpf_helpers.h"

#include "protocols/http/maps.h"

#include "protocols/tls/go-tls-types.h"
#include "protocols/tls/go-tls-location.h"

// Implemented either in c/runtime/runtime-get-tls-base.h or in ____ (TODO add offset-guessed version)
static void* get_tls_base(struct task_struct* task);

// This function was adapted from https://github.com/go-delve/delve:
// - https://github.com/go-delve/delve/blob/cd9e6c02a6ca5f0d66c1f770ee10a0d8f4419333/pkg/proc/internal/ebpf/bpf/trace.bpf.c#L144
// which is licensed under MIT.
static __always_inline int read_goroutine_id_from_tls(goroutine_id_metadata_t* m, int64_t* dest) {
    // Get the current task.
    struct task_struct* task = (struct task_struct*) bpf_get_current_task();
    if (task == NULL) {
        return 1;
    }

    // Get the Goroutine ID, which is stored in thread local storage.
    uintptr_t g_addr = 0;

    void* base = get_tls_base(task);
    if (base == NULL) {
        return 1;
    }

    if (bpf_probe_read_user(&g_addr, sizeof(uintptr_t), base + m->runtime_g_tls_addr_offset)) {
        return 1;
    }
    void* goroutine_id_ptr = (void*) (g_addr + m->goroutine_id_offset);
    return bpf_probe_read_user(dest, sizeof(int64_t), goroutine_id_ptr) < 0;
}

static __always_inline int read_goroutine_id_from_register(struct pt_regs *ctx, goroutine_id_metadata_t* m, int64_t* dest) {
    // Get a pointer to the register field itself (i.e. &ctx->dx)
    // and bpf_probe_read in the register value
    // (which in turn is a pointer to the current runtime.g).
    // Otherwise, the verifier rejects directly using the register value.
    uint64_t* reg_ptr = (uint64_t*)read_register_indirect(ctx, m->runtime_g_register);
    if (!reg_ptr) {
        return 1;
    }

    uint64_t runtime_g_ptr = 0;
    if (bpf_probe_read_kernel(&runtime_g_ptr, sizeof(int64_t), reg_ptr) < 0) {
        return 1;
    }

    return bpf_probe_read_user(dest, sizeof(int64_t), (void*)(runtime_g_ptr + m->goroutine_id_offset));
}

static __always_inline int read_goroutine_id(struct pt_regs *ctx, goroutine_id_metadata_t* m, int64_t* dest) {
    if (m->runtime_g_in_register) {
        return read_goroutine_id_from_register(ctx, m, dest);
    }
    return read_goroutine_id_from_tls(m, dest);
}

#endif //__GO_TLS_GOID_H
