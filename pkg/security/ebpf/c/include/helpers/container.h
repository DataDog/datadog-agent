#ifndef _HELPERS_CONTAINER_H_
#define _HELPERS_CONTAINER_H_

#include "constants/custom.h"
#include "utils.h"

static void __attribute__((always_inline)) reset_cgroup_context(struct container_context_t *context) {
    context->cgroup_context.cgroup_file.ino = 0;
    context->cgroup_context.cgroup_file.mount_id = 0;
    context->cgroup_context.cgroup_file.path_id = 0;
    context->cgroup_context.cgroup_flags = 0;
}

static void __attribute__((always_inline)) fill_container_context(struct proc_cache_t *entry, struct container_context_t *context) {
    if (entry) {
        context->cgroup_context = entry->container.cgroup_context;
    } else {
        reset_cgroup_context(context);
    }
}

#endif
