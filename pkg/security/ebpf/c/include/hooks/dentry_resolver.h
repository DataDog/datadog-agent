#ifndef _HOOKS_DENTRY_RESOLVER_H_
#define _HOOKS_DENTRY_RESOLVER_H_

#include "constants/offsets/filesystem.h"
#include "helpers/dentry_resolver.h"
#include "helpers/discarders.h"
#include "helpers/syscalls.h"

int __attribute__((always_inline)) resolve_dentry_tail_call(void *ctx, struct dentry_resolver_input_t *input) {
    struct path_leaf_t map_value = {};
    struct path_key_t key = input->key;
    struct path_key_t next_key = input->key;
    struct qstr qstr;
    struct dentry *dentry = input->dentry;
    struct dentry *d_parent = NULL;

    u32 zero = 0;
    struct is_discarded_by_inode_t *params = bpf_map_lookup_elem(&is_discarded_by_inode_gen, &zero);
    if (!params) {
        return DENTRY_ERROR;
    }
    *params = (struct is_discarded_by_inode_t){
        .discarder_type = input->discarder_type,
        .now = bpf_ktime_get_ns(),
    };

    if (key.ino == 0) {
        return DENTRY_INVALID;
    }

#pragma unroll
    for (int i = 0; i < DR_MAX_ITERATION_DEPTH; i++)
    {
        bpf_probe_read(&d_parent, sizeof(d_parent), &dentry->d_parent);

        key = next_key;
        if (dentry != d_parent) {
            next_key.ino = get_dentry_ino(d_parent);
        } else {
            next_key.ino = 0;
            next_key.mount_id = 0;
        }

        if (input->discarder_type && i <= 3) {
            params->discarder.path_key.ino = key.ino;
            params->discarder.path_key.mount_id = key.mount_id;
            params->discarder.is_leaf = i == 0;

            if (is_discarded_by_inode(params)) {
                if (input->flags & ACTIVITY_DUMP_RUNNING) {
                    input->flags |= SAVED_BY_ACTIVITY_DUMP;
                } else {
                    return DENTRY_DISCARDED;
                }
            }
        }

        bpf_probe_read(&qstr, sizeof(qstr), &dentry->d_name);

        long len = bpf_probe_read_str(&map_value.name, sizeof(map_value.name), (void *)qstr.name);
        if (len < 0) {
            len = 0;
        }
        map_value.len = len;

        if (map_value.name[0] == '/' || map_value.name[0] == 0) {
            next_key.ino = 0;
            next_key.mount_id = 0;
        }

        map_value.parent = next_key;

        bpf_map_update_elem(&pathnames, &key, &map_value, BPF_ANY);

        dentry = d_parent;
        if (next_key.ino == 0) {
            // mark the path resolution as complete which will stop the tail calls
            input->key.ino = 0;
            return i + 1;
        }
    }

    if (input->iteration == DR_MAX_TAIL_CALL) {
        map_value.name[0] = 0;
        map_value.parent.mount_id = 0;
        map_value.parent.ino = 0;
        bpf_map_update_elem(&pathnames, &next_key, &map_value, BPF_ANY);
    }

    // prepare for the next iteration
    input->dentry = d_parent;
    input->key = next_key;
    return DR_MAX_ITERATION_DEPTH;
}

#define dentry_resolver_kern(ctx, progs_map, callbacks_map, dentry_resolver_kern_key)                                  \
    struct syscall_cache_t *syscall = peek_syscall(EVENT_ANY);                                                         \
    if (!syscall)                                                                                                      \
        return 0;                                                                                                      \
                                                                                                                       \
    syscall->resolver.iteration++;                                                                                     \
    syscall->resolver.ret = resolve_dentry_tail_call(ctx, &syscall->resolver);                                         \
                                                                                                                       \
    if (syscall->resolver.ret > 0) {                                                                                   \
        if (syscall->resolver.iteration < DR_MAX_TAIL_CALL && syscall->resolver.key.ino != 0) {                        \
            bpf_tail_call_compat(ctx, progs_map, dentry_resolver_kern_key);                                            \
        }                                                                                                              \
                                                                                                                       \
        syscall->resolver.ret += DR_MAX_ITERATION_DEPTH * (syscall->resolver.iteration - 1);                           \
    }                                                                                                                  \
                                                                                                                       \
    if (syscall->resolver.callback >= 0) {                                                                             \
        bpf_tail_call_compat(ctx, callbacks_map, syscall->resolver.callback);                                          \
    }                                                                                                                  \

SEC("kprobe/dentry_resolver_kern")
int kprobe_dentry_resolver_kern(struct pt_regs *ctx) {
    dentry_resolver_kern(ctx, &dentry_resolver_kprobe_progs, &dentry_resolver_kprobe_callbacks, DR_KPROBE_DENTRY_RESOLVER_KERN_KEY);
    return 0;
}

SEC("tracepoint/dentry_resolver_kern")
int tracepoint_dentry_resolver_kern(void *ctx) {
    dentry_resolver_kern(ctx, &dentry_resolver_tracepoint_progs, &dentry_resolver_tracepoint_callbacks, DR_TRACEPOINT_DENTRY_RESOLVER_KERN_KEY);
    return 0;
}

SEC("kprobe/dentry_resolver_erpc_write_user")
int kprobe_dentry_resolver_erpc_write_user(struct pt_regs *ctx) {
    u32 key = 0;
    u32 resolution_err = 0;
    struct path_leaf_t *map_value = 0;
    struct path_key_t iteration_key = {};

    struct dr_erpc_state_t *state = bpf_map_lookup_elem(&dr_erpc_state, &key);
    if (state == NULL) {
        return 0;
    }

    state->iteration++;

#pragma unroll
    for (int i = 0; i < DR_MAX_ITERATION_DEPTH; i++)
    {
        iteration_key = state->key;
        map_value = bpf_map_lookup_elem(&pathnames, &iteration_key);
        if (map_value == NULL) {
            resolution_err = DR_ERPC_CACHE_MISS;
            goto exit;
        }

        // make sure we do not write outside of the provided buffer
        if (state->cursor + sizeof(state->key) >= state->buffer_size) {
            resolution_err = DR_ERPC_BUFFER_SIZE;
            goto exit;
        }

        state->ret = bpf_probe_write_user((void *) state->userspace_buffer + state->cursor, &state->key, sizeof(state->key));
        if (state->ret < 0) {
            resolution_err = state->ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
            goto exit;
        }
        state->ret = bpf_probe_write_user((void *) state->userspace_buffer + state->cursor + offsetof(struct path_key_t, path_id), &state->challenge, sizeof(state->challenge));
        if (state->ret < 0) {
            resolution_err = state->ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
            goto exit;
        }

        state->cursor += sizeof(state->key);

        // make sure we do not write outside of the provided buffer
        if (state->cursor + map_value->len >= state->buffer_size) {
            resolution_err = DR_ERPC_BUFFER_SIZE;
            goto exit;
        }

        state->ret = bpf_probe_write_user((void *) state->userspace_buffer + state->cursor, map_value->name, DR_MAX_SEGMENT_LENGTH + 1);
        if (state->ret < 0) {
            resolution_err = state->ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
            goto exit;
        }

        state->cursor += map_value->len;

        state->key.ino = map_value->parent.ino;
        state->key.path_id = map_value->parent.path_id;
        state->key.mount_id = map_value->parent.mount_id;
        if (state->key.ino == 0) {
            goto exit;
        }
    }
    if (state->iteration < DR_MAX_TAIL_CALL) {
        bpf_tail_call_compat(ctx, &dentry_resolver_kprobe_progs, DR_ERPC_KEY);
        resolution_err = DR_ERPC_TAIL_CALL_ERROR;
    }

exit:
    monitor_resolution_err(resolution_err);
    return 0;
}

SEC("kprobe/dentry_resolver_erpc_mmap")
int kprobe_dentry_resolver_erpc_mmap(struct pt_regs *ctx) {
    u32 key = 0;
    u32 resolution_err = 0;
    struct path_leaf_t *map_value = 0;
    struct path_key_t iteration_key = {};
    char *mmapped_userspace_buffer = NULL;

    struct dr_erpc_state_t *state = bpf_map_lookup_elem(&dr_erpc_state, &key);
    if (state == NULL) {
        return 0;
    }

    mmapped_userspace_buffer = bpf_map_lookup_elem(&dr_erpc_buffer, &key);
    if (mmapped_userspace_buffer == NULL) {
        return 0;
    }

    state->iteration++;

#pragma unroll
    for (int i = 0; i < DR_MAX_ITERATION_DEPTH; i++)
    {
        iteration_key = state->key;
        map_value = bpf_map_lookup_elem(&pathnames, &iteration_key);
        if (map_value == NULL) {
            resolution_err = DR_ERPC_CACHE_MISS;
            goto exit;
        }

        // make sure we do not write outside of the provided buffer
        if (state->cursor + sizeof(state->key) >= state->buffer_size) {
            resolution_err = DR_ERPC_BUFFER_SIZE;
            goto exit;
        }

        state->ret = bpf_probe_read((void *) mmapped_userspace_buffer + (state->cursor & 0x7FFF), sizeof(state->key), &state->key);
        if (state->ret < 0) {
            resolution_err = state->ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
            goto exit;
        }

        state->ret = bpf_probe_read((void *) mmapped_userspace_buffer + ((state->cursor + offsetof(struct path_key_t, path_id)) & 0x7FFF), sizeof(state->challenge), &state->challenge);
        if (state->ret < 0) {
            resolution_err = state->ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
            goto exit;
        }

        state->cursor += sizeof(state->key);

        // make sure we do not write outside of the provided buffer
        if (state->cursor + map_value->len >= state->buffer_size) {
            resolution_err = DR_ERPC_BUFFER_SIZE;
            goto exit;
        }

        state->ret = bpf_probe_read((void *) mmapped_userspace_buffer + (state->cursor & 0x7FFF), DR_MAX_SEGMENT_LENGTH + 1, map_value->name);
        if (state->ret < 0) {
            resolution_err = state->ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
            goto exit;
        }

        state->cursor += map_value->len;

        state->key.ino = map_value->parent.ino;
        state->key.path_id = map_value->parent.path_id;
        state->key.mount_id = map_value->parent.mount_id;
        if (state->key.ino == 0) {
            goto exit;
        }
    }
    if (state->iteration < DR_MAX_TAIL_CALL) {
        bpf_tail_call_compat(ctx, &dentry_resolver_kprobe_progs, DR_ERPC_KEY);
        resolution_err = DR_ERPC_TAIL_CALL_ERROR;
    }

exit:
    monitor_resolution_err(resolution_err);
    return 0;
}

SEC("kprobe/dentry_resolver_segment_erpc_write_user")
int kprobe_dentry_resolver_segment_erpc_write_user(struct pt_regs *ctx) {
    u32 key = 0;
    u32 resolution_err = 0;

    struct dr_erpc_state_t *state = bpf_map_lookup_elem(&dr_erpc_state, &key);
    if (state == NULL) {
        return 0;
    }

    // resolve segment and write in buffer
    struct path_key_t path_key = state->key;
    struct path_leaf_t *map_value = bpf_map_lookup_elem(&pathnames, &path_key);
    if (map_value == NULL) {
        resolution_err = DR_ERPC_CACHE_MISS;
        goto exit;
    }

    if (map_value->len + sizeof(key) > state->buffer_size) {
        // make sure we do not write outside of the provided buffer
        resolution_err = DR_ERPC_BUFFER_SIZE;
        goto exit;
    }

    int ret = bpf_probe_write_user((void *) state->userspace_buffer, &state->key, sizeof(state->key));
    if (ret < 0) {
        resolution_err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
        goto exit;
    }
    ret = bpf_probe_write_user((void *) state->userspace_buffer + offsetof(struct path_key_t, path_id), &state->challenge, sizeof(state->challenge));
    if (ret < 0) {
        resolution_err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
        goto exit;
    }

    ret = bpf_probe_write_user((void *) state->userspace_buffer + sizeof(state->key), map_value->name, DR_MAX_SEGMENT_LENGTH + 1);
    if (ret < 0) {
        resolution_err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
        goto exit;
    }

exit:
    monitor_resolution_err(resolution_err);
    return 0;
}

SEC("kprobe/dentry_resolver_segment_erpc_mmap")
int kprobe_dentry_resolver_segment_erpc_mmap(struct pt_regs *ctx) {
    u32 key = 0;
    u32 resolution_err = 0;
    char *mmapped_userspace_buffer = NULL;

    struct dr_erpc_state_t *state = bpf_map_lookup_elem(&dr_erpc_state, &key);
    if (state == NULL) {
        return 0;
    }

    mmapped_userspace_buffer = bpf_map_lookup_elem(&dr_erpc_buffer, &key);
    if (mmapped_userspace_buffer == NULL) {
        return DR_ERPC_UNKNOWN_ERROR;
    }

    // resolve segment and write in buffer
    struct path_key_t path_key = state->key;
    struct path_leaf_t *map_value = bpf_map_lookup_elem(&pathnames, &path_key);
    if (map_value == NULL) {
        resolution_err = DR_ERPC_CACHE_MISS;
        goto exit;
    }

    if (map_value->len + sizeof(key) > state->buffer_size) {
        // make sure we do not write outside of the provided buffer
        resolution_err = DR_ERPC_BUFFER_SIZE;
        goto exit;
    }

    int ret = bpf_probe_read((void *) mmapped_userspace_buffer, sizeof(state->key), &state->key);
    if (ret < 0) {
        resolution_err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
        goto exit;
    }

    ret = bpf_probe_read((void *) mmapped_userspace_buffer + (offsetof(struct path_key_t, path_id) & 0x7FFF), sizeof(state->challenge), &state->challenge);
    if (ret < 0) {
        resolution_err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
        goto exit;
    }

    ret = bpf_probe_read((void *) mmapped_userspace_buffer + (sizeof(state->key) & 0x7FFF), DR_MAX_SEGMENT_LENGTH + 1, map_value->name);
    if (ret < 0) {
        resolution_err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
        goto exit;
    }

exit:
    monitor_resolution_err(resolution_err);
    return 0;
}

SEC("kprobe/dentry_resolver_parent_erpc_write_user")
int kprobe_dentry_resolver_parent_erpc_write_user(struct pt_regs *ctx) {
    u32 key = 0;
    u32 resolution_err = 0;

    struct dr_erpc_state_t *state = bpf_map_lookup_elem(&dr_erpc_state, &key);
    if (state == NULL) {
        return 0;
    }

    // resolve segment and write in buffer
    struct path_key_t path_key = state->key;
    struct path_leaf_t *map_value = bpf_map_lookup_elem(&pathnames, &path_key);
    if (map_value == NULL) {
        resolution_err = DR_ERPC_CACHE_MISS;
        goto exit;
    }

    if (sizeof(map_value->parent) > state->buffer_size) {
        // make sure we do not write outside of the provided buffer
        resolution_err = DR_ERPC_BUFFER_SIZE;
        goto exit;
    }

    int ret = bpf_probe_write_user((void *) state->userspace_buffer, &map_value->parent, sizeof(map_value->parent));
    if (ret < 0) {
        resolution_err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
        goto exit;
    }
    ret = bpf_probe_write_user((void *) state->userspace_buffer + offsetof(struct path_key_t, path_id), &state->challenge, sizeof(state->challenge));
    if (ret < 0) {
        resolution_err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
        goto exit;
    }

exit:
    monitor_resolution_err(resolution_err);
    return 0;
}

SEC("kprobe/dentry_resolver_parent_erpc_mmap")
int kprobe_dentry_resolver_parent_erpc_mmap(struct pt_regs *ctx) {
    u32 key = 0;
    u32 resolution_err = 0;
    char *mmapped_userspace_buffer = NULL;

    struct dr_erpc_state_t *state = bpf_map_lookup_elem(&dr_erpc_state, &key);
    if (state == NULL) {
        return 0;
    }

    mmapped_userspace_buffer = bpf_map_lookup_elem(&dr_erpc_buffer, &key);
    if (mmapped_userspace_buffer == NULL) {
        return DR_ERPC_UNKNOWN_ERROR;
    }

    // resolve segment and write in buffer
    struct path_key_t path_key = state->key;
    struct path_leaf_t *map_value = bpf_map_lookup_elem(&pathnames, &path_key);
    if (map_value == NULL) {
        resolution_err = DR_ERPC_CACHE_MISS;
        goto exit;
    }

    if (sizeof(map_value->parent) > state->buffer_size) {
        // make sure we do not write outside of the provided buffer
        resolution_err = DR_ERPC_BUFFER_SIZE;
        goto exit;
    }

    int ret = bpf_probe_read((void *) mmapped_userspace_buffer, sizeof(map_value->parent), &map_value->parent);
    if (ret < 0) {
        resolution_err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
        goto exit;
    }
    ret = bpf_probe_read((void *) mmapped_userspace_buffer + (offsetof(struct path_key_t, path_id) & 0x7FFF), sizeof(state->challenge), &state->challenge);
    if (ret < 0) {
        resolution_err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
        goto exit;
    }

exit:
    monitor_resolution_err(resolution_err);
    return 0;
}

SEC("kprobe/dentry_resolver_ad_filter")
int kprobe_dentry_resolver_ad_filter(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_ANY);
    if (!syscall) {
        return 0;
    }

    if (is_activity_dump_running(ctx, bpf_get_current_pid_tgid() >> 32, bpf_ktime_get_ns(), syscall->type)) {
        syscall->resolver.flags |= ACTIVITY_DUMP_RUNNING;
    }

    bpf_tail_call_compat(ctx, &dentry_resolver_kprobe_progs, DR_KPROBE_DENTRY_RESOLVER_KERN_KEY);
    return 0;
}

SEC("tracepoint/dentry_resolver_ad_filter")
int tracepoint_dentry_resolver_ad_filter(void *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_ANY);
    if (!syscall) {
        return 0;
    }

    if (is_activity_dump_running(ctx, bpf_get_current_pid_tgid() >> 32, bpf_ktime_get_ns(), syscall->type)) {
        syscall->resolver.flags |= ACTIVITY_DUMP_RUNNING;
    }

    bpf_tail_call_compat(ctx, &dentry_resolver_tracepoint_progs, DR_TRACEPOINT_DENTRY_RESOLVER_KERN_KEY);
    return 0;
}

#endif
