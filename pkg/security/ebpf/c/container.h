#ifndef _CONTAINER_H_
#define _CONTAINER_H_

#include "defs.h"

struct proc_cache_t* get_pid_cache(u64 pid);

static __attribute__((always_inline)) u32 copy_container_id(char dst[CONTAINER_ID_LEN], char src[CONTAINER_ID_LEN]) {
    if (src[0] == 0) {
        return 0;
    }

#pragma unroll
    for (int i = 0; i < CONTAINER_ID_LEN; i++)
    {
        dst[i] = src[i];
    }
    return CONTAINER_ID_LEN;
}

static void __attribute__((always_inline)) fill_container_data(struct proc_cache_t *entry, struct container_context_t *context) {
    if (entry) {
        copy_container_id(context->container_id, entry->container_id);
    }
}

#endif
