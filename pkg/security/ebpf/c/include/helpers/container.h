#ifndef _HELPERS_CONTAINER_H_
#define _HELPERS_CONTAINER_H_

#include "constants/custom.h"
#include "utils.h"

static __attribute__((always_inline)) void copy_container_id(const char src[CONTAINER_ID_LEN], char dst[CONTAINER_ID_LEN]) {
    bpf_probe_read(dst, CONTAINER_ID_LEN, (void*)src);
}

#define copy_container_id_no_tracing(src, dst) __builtin_memmove(dst, src, CONTAINER_ID_LEN)

static void __attribute__((always_inline)) fill_container_context(struct proc_cache_t *entry, struct container_context_t *context) {
    if (entry) {
        copy_container_id(entry->container.container_id, context->container_id);
        context->flags = entry->container.flags;
    }
}

static __attribute__((always_inline)) int is_container_id_valid(const char id[CONTAINER_ID_LEN]) {
#pragma unroll
    for (int i = 0; i < CONTAINER_ID_LEN; i++)
    {
        if (!_isxdigit(id[i])) {
            return 0;
        }
    }

    return 1;
}

#endif
