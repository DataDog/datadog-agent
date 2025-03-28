#ifndef _HELPERS_SYSCTL_H_
#define _HELPERS_SYSCTL_H_

#include "constants/custom.h"
#include "helpers/approvers.h"
#include "helpers/container.h"
#include "helpers/process.h"

#include "maps.h"

__attribute__((always_inline)) struct sysctl_event_t *get_sysctl_event() {
    u32 key = SYSCTL_EVENT_GEN_KEY;
    return bpf_map_lookup_elem(&sysctl_event_gen, &key);
}

__attribute__((always_inline)) struct sysctl_event_t *reset_sysctl_event() {
    u32 key = SYSCTL_EVENT_GEN_KEY;
    struct sysctl_event_t *evt = bpf_map_lookup_elem(&sysctl_event_gen, &key);
    if (evt == NULL) {
        // should never happen
        return NULL;
    }

    // reset event
    evt->action = SYSCTL_UNKNOWN;
    evt->file_position = 0;
    evt->name_len = 0;
    evt->old_value_len = 0;
    evt->new_value_len = 0;
    evt->flags = 0;
    evt->sysctl_buffer[0] = 0;

    // process, container, span contexts
    struct proc_cache_t *entry = fill_process_context(&evt->process);
    fill_container_context(entry, &evt->container);
    fill_span_context(&evt->span);

    return evt;
}

__attribute__((always_inline)) void handle_cgroup_sysctl(struct bpf_sysctl *ctx) {
    struct sysctl_event_t *evt = NULL;
    if (has_tracing_helpers_in_cgroup_sysctl()) {
        evt = reset_sysctl_event();
    } else {
        evt = get_sysctl_event();
    }

    if (evt == NULL) {
        return;
    }

    // copy sysctl action and operation file position
    if (ctx->write) {
        evt->action = SYSCTL_WRITE;
    } else {
        evt->action = SYSCTL_READ;
    }
    evt->file_position = ctx->file_pos;

    // check approvers
    struct policy_t policy = fetch_policy(EVENT_SYSCTL);
    struct syscall_cache_t syscall = {
        .policy = policy,
        .type = EVENT_SYSCTL,
        .sysctl = {
            .action = evt->action,
        }
    };
    if (approve_syscall_with_tgid(evt->process.pid, &syscall, sysctl_approvers) == DISCARDED) {
        return;
    }

    // copy the name of the control parameter
    u32 cursor = 0;
    u32 ret = bpf_sysctl_get_name(ctx, &evt->sysctl_buffer[0], MAX_SYSCTL_OBJ_LEN - 2, 0);
    if ((int)ret == -E2BIG) {
        evt->flags |= SYSCTL_NAME_TRUNCATED;
        evt->name_len = MAX_SYSCTL_OBJ_LEN - 1;
    } else {
        evt->name_len = ret + 1;
    }

    // advance cursor in sysctl_buffer
    cursor += evt->name_len;

    // copy the current value of the control parameter
    ret = bpf_sysctl_get_current_value(ctx, &evt->sysctl_buffer[cursor & (MAX_SYSCTL_OBJ_LEN-1)], MAX_SYSCTL_OBJ_LEN - 1);
    switch ((int)ret) {
    case -E2BIG:
        evt->flags |= SYSCTL_OLD_VALUE_TRUNCATED;
        evt->old_value_len = MAX_SYSCTL_OBJ_LEN;
        break;
    case -EINVAL:
        evt->old_value_len = 1;
        evt->sysctl_buffer[cursor & (MAX_SYSCTL_BUFFER_LEN - 1)] = 0;
        break;
    default:
        evt->old_value_len = ret + 1;
        break;
    }

    // advance cursor in sysctl_buffer
    cursor += evt->old_value_len;

    // copy the new value for the control parameter
    ret = bpf_sysctl_get_new_value(ctx, &evt->sysctl_buffer[cursor & (2*MAX_SYSCTL_OBJ_LEN - 1)], MAX_SYSCTL_OBJ_LEN - 1);
    switch ((int)ret) {
    case -E2BIG:
        evt->flags |= SYSCTL_NEW_VALUE_TRUNCATED;
        evt->new_value_len = MAX_SYSCTL_OBJ_LEN;
        break;
    case -EINVAL:
        evt->new_value_len = 1;
        evt->sysctl_buffer[cursor & (MAX_SYSCTL_BUFFER_LEN - 1)] = 0;
        break;
    default:
        evt->new_value_len = ret + 1;
        break;
    }

    // advance cursor in sysctl_buffer
    cursor += evt->new_value_len;

    send_event_with_size_ptr(ctx, EVENT_SYSCTL, evt, offsetof(struct sysctl_event_t, sysctl_buffer) + (cursor & (MAX_SYSCTL_BUFFER_LEN - 1)));
}

#endif
