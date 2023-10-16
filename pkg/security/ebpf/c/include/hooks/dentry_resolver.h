#ifndef _HOOKS_DENTRY_RESOLVER_H_
#define _HOOKS_DENTRY_RESOLVER_H_

#include "constants/offsets/filesystem.h"
#include "helpers/dentry_resolver.h"
#include "helpers/discarders.h"
#include "helpers/syscalls.h"

// progs called from events hooks

int __attribute__((always_inline)) resolve_dentry_chain(void *ctx, struct dentry_resolver_input_t *input, struct ring_buffer_t *rb, struct ring_buffer_ctx *rb_ctx) {
    u32 zero = 0;
    struct dentry_leaf_t map_value = {};
    struct dentry_key_t key = input->key;
    struct dentry_key_t next_key = input->key;
    struct qstr qstr;
    struct dentry *dentry = input->dentry;
    struct dentry *d_parent = NULL;
    char name[DR_MAX_DENTRY_NAME_LENGTH + 1] = {0};

    if (key.ino == 0) {
        return DENTRY_INVALID;
    }

    struct is_discarded_by_inode_t *params = bpf_map_lookup_elem(&is_discarded_by_inode_gen, &zero);
    if (!params) {
        return DENTRY_ERROR;
    }
    *params = (struct is_discarded_by_inode_t){
        .discarder_type = input->discarder_type,
        .now = bpf_ktime_get_ns(),
    };

#pragma unroll
    for (int i = 0; i < DR_MAX_ITERATION_DEPTH; i++) {
        bpf_probe_read(&d_parent, sizeof(d_parent), &dentry->d_parent);

        key = next_key;
        if (dentry != d_parent) {
            next_key.ino = get_dentry_ino(d_parent);
        } else {
            next_key.ino = 0;
            next_key.mount_id = 0;
        }

        if (input->discarder_type && i <= 3) {
            params->discarder.dentry_key.ino = key.ino;
            params->discarder.dentry_key.mount_id = key.mount_id;
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
        long len = bpf_probe_read_str(&name, sizeof(name), (void *)qstr.name);
        // len -= 1; // do not count trailing zero
        if (len <= 0 || name[0] == 0) {
            map_value.parent.ino = 0;
            map_value.parent.mount_id = 0;
            bpf_map_update_elem(&dentries, &key, &map_value, BPF_ANY);
            return DENTRY_BAD_NAME;
        }

        if (len == 2 && name[0] == '/') {
            // we only want to push '/' if we are resolving the root path
            // and we resolve the root path if it's the first dentry name being push to the ring buffer
            if (rb_ctx->len == sizeof(rb_ctx->watermark)) {
                rb_push_char(rb, rb_ctx, '/');
            }
            rb_push_char(rb, rb_ctx, '\0');
            // mark the path resolution as complete which will stop the tail calls
            input->key.ino = 0;
            map_value.parent.ino = 0;
            map_value.parent.mount_id = 0;
            bpf_map_update_elem(&dentries, &key, &map_value, BPF_ANY);
            return i + 1;
        }

        u32 rb_tail_len = rb_get_tail_length(rb_ctx);
        if (rb_tail_len < sizeof(name)) {
            rb->buffer[rb_ctx->write_cursor % RING_BUFFER_SIZE] = '\0';
            rb_ctx->len += rb_tail_len;
            rb_ctx->write_cursor = 0;
        }

        rb_push_str(rb, rb_ctx, &name[0], sizeof(name));
        rb_push_char(rb, rb_ctx, '/');

        map_value.parent = next_key;
        bpf_map_update_elem(&dentries, &key, &map_value, BPF_ANY);
        dentry = d_parent;
    }

    if (input->iteration == DR_MAX_TAIL_CALL) {
        map_value.parent.mount_id = 0;
        map_value.parent.ino = 0;
        bpf_map_update_elem(&dentries, &next_key, &map_value, BPF_ANY);
        return DENTRY_MAX_TAIL_CALL;
    }

    // prepare for the next iteration
    input->dentry = d_parent;
    input->key = next_key;
    return DR_MAX_ITERATION_DEPTH;
}

int __attribute__((always_inline)) dentry_resolver_loop(void *ctx, int dr_type) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_ANY);
    if (!syscall) {
        return 0;
    }

    u32 zero = 0;
    struct ring_buffer_ctx *rb_ctx = bpf_map_lookup_elem(&dr_ringbufs_ctx, &zero);
    if (!rb_ctx) {
        return 0;
    }

    u32 cpu = bpf_get_smp_processor_id();
    struct ring_buffer_t *rb = bpf_map_lookup_elem(&dr_ringbufs, &cpu);
    if (!rb) {
        return 0;
    }

    syscall->resolver.iteration++;
    syscall->resolver.ret = resolve_dentry_chain(ctx, &syscall->resolver, rb, rb_ctx);

    if (syscall->resolver.ret > 0) {
        if (syscall->resolver.iteration < DR_MAX_TAIL_CALL && syscall->resolver.key.ino != 0) {
            tail_call_dr_progs(ctx, dr_type, DR_LOOP);
        }

        syscall->resolver.ret += DR_MAX_ITERATION_DEPTH * (syscall->resolver.iteration - 1);
        rb_push_watermark(rb, rb_ctx);
    } else {
        rb_cleanup_ctx(rb_ctx);
        rb_ctx->len = ~0 + syscall->resolver.ret;
    }

    if (syscall->resolver.callback >= 0) {
        tail_call_dr_progs(ctx, dr_type, syscall->resolver.callback);
    }

    return 0;
}

SEC("tracepoint/dentry_resolver_loop")
int tracepoint_dentry_resolver_loop(void *ctx) {
    return dentry_resolver_loop(ctx, DR_TRACEPOINT);
}

TAIL_CALL_TARGET("dentry_resolver_loop")
int tail_call_target_dentry_resolver_loop(void *ctx) {
    return dentry_resolver_loop(ctx, DR_KPROBE_OR_FENTRY);
}

int __attribute__((always_inline)) dentry_resolver_entrypoint(void *ctx, int dr_type) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_ANY);
    if (!syscall) {
        return 0;
    }

    if (is_activity_dump_running(ctx, bpf_get_current_pid_tgid() >> 32, bpf_ktime_get_ns(), syscall->type)) {
        syscall->resolver.flags |= ACTIVITY_DUMP_RUNNING;
    }

    if (init_dr_ringbuf_ctx()) {
        return 0;
    }

    syscall->resolver.iteration = 0;
    tail_call_dr_progs(ctx, dr_type, DR_LOOP);
    return 0;
}

TAIL_CALL_TARGET("dentry_resolver_entrypoint")
int tail_call_target_dentry_resolver_entrypoint(void *ctx) {
    return dentry_resolver_entrypoint(ctx, DR_KPROBE_OR_FENTRY);
}

SEC("tracepoint/dentry_resolver_entrypoint")
int tracepoint_dentry_resolver_entrypoint(void *ctx) {
    return dentry_resolver_entrypoint(ctx, DR_TRACEPOINT);
}

// progs called from eRPC resolution

TAIL_CALL_TARGET("erpc_resolve_parent_mmap")
int tail_call_target_erpc_resolve_parent_mmap(void *ctx) {
    u32 key = 0;
    u32 resolution_err = 0;
    char *mmapped_userspace_buffer = NULL;

    struct dr_erpc_state_t *state = bpf_map_lookup_elem(&dr_erpc_state, &key);
    if (state == NULL) {
        return 0;
    }

    mmapped_userspace_buffer = bpf_map_lookup_elem(&dr_erpc_buffer, &key);
    if (mmapped_userspace_buffer == NULL) {
        resolution_err = DR_ERPC_UNKNOWN_ERROR;
        goto exit;
    }

    // resolve segment and write in buffer
    struct dentry_key_t dentry_key = state->key;
    struct dentry_leaf_t *map_value = bpf_map_lookup_elem(&dentries, &dentry_key);
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
    ret = bpf_probe_read((void *) mmapped_userspace_buffer + (offsetof(struct dentry_key_t, path_id) & 0x7FFF), sizeof(state->challenge), &state->challenge);
    if (ret < 0) {
        resolution_err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
        goto exit;
    }

exit:
    monitor_resolution_err(resolution_err);
    return 0;
}

TAIL_CALL_TARGET("erpc_resolve_parent_write_user")
int tail_call_target_erpc_resolve_parent_write_user(void *ctx) {
    u32 key = 0;
    u32 resolution_err = 0;

    struct dr_erpc_state_t *state = bpf_map_lookup_elem(&dr_erpc_state, &key);
    if (state == NULL) {
        return 0;
    }

    // resolve segment and write in buffer
    struct dentry_key_t dentry_key = state->key;
    struct dentry_leaf_t *map_value = bpf_map_lookup_elem(&dentries, &dentry_key);
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
    ret = bpf_probe_write_user((void *) state->userspace_buffer + offsetof(struct dentry_key_t, path_id), &state->challenge, sizeof(state->challenge));
    if (ret < 0) {
        resolution_err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
        goto exit;
    }

exit:
    monitor_resolution_err(resolution_err);
    return 0;
}

TAIL_CALL_TARGET("erpc_resolve_path_watermark_reader")
int tail_call_target_erpc_resolve_path_watermark_reader(void *ctx) {
    u32 zero = 0, err = 0;
    struct dr_erpc_state_t *state = bpf_map_lookup_elem(&dr_erpc_state, &zero);
    if (!state) {
        return 0;
    }

    struct ring_buffer_t *rb = bpf_map_lookup_elem(&dr_ringbufs, &state->path_ref.cpu);
    if (!rb) {
        err = DR_ERPC_CACHE_MISS; // TODO: use a specific error type for malformed request
        goto exit;
    }

    if (state->path_reader_state == READ_FRONTWATERMARK) {
        // write the challenge here so that the main eRPC eBPF program doesn't use the `bpf_probe_write_user` helper.
        int ret = bpf_probe_write_user((void *)state->userspace_buffer, &state->challenge, sizeof(state->challenge));
        if (ret < 0) {
            err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
            goto exit;
        }
        state->cursor += sizeof(state->challenge);
    }

    if (state->path_ref.read_cursor + sizeof(state->path_ref.watermark) <= RING_BUFFER_SIZE) {
        int ret = bpf_probe_write_user((void *)state->userspace_buffer + state->cursor, &rb->buffer[state->path_ref.read_cursor], sizeof(state->path_ref.watermark));
        if (ret < 0) {
            err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
            goto exit;
        }
        state->path_ref.read_cursor += sizeof(state->path_ref.watermark);
        state->cursor += sizeof(state->path_ref.watermark);
    } else {
#pragma unroll
        for (int i = 0; i < sizeof(state->path_ref.watermark); i++) {
            int ret = bpf_probe_write_user((void *)state->userspace_buffer + state->cursor, &rb->buffer[state->path_ref.read_cursor % RING_BUFFER_SIZE], 1);
            if (ret < 0) {
                err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
                goto exit;
            }
            state->path_ref.read_cursor++;
            state->cursor++;
        }
    }

    if (state->path_reader_state == READ_FRONTWATERMARK) {
        state->path_reader_state = READ_PATHSEGMENT;
        tail_call_erpc_progs(ctx, ERPC_DR_RESOLVE_PATH_DATA_READER_KEY);
        err = DR_ERPC_TAIL_CALL_ERROR;
    }

exit:
    monitor_resolution_err(err);
    return 0;
}

TAIL_CALL_TARGET("erpc_resolve_path_segment_reader")
int tail_call_target_erpc_resolve_path_segment_reader(void *ctx) {
    u32 zero = 0, err = 0;
    char path_chunk[32] = {0};
    struct dr_erpc_state_t *state = bpf_map_lookup_elem(&dr_erpc_state, &zero);
    if (!state) {
        return 0;
    }

    struct ring_buffer_t *rb = bpf_map_lookup_elem(&dr_ringbufs, &state->path_ref.cpu);
    if (!rb) {
        err = DR_ERPC_CACHE_MISS; // TODO: use a specific error type for malformed request
        goto exit;
    }

#pragma unroll
    for (int i = 0; i < 32; i++) {
        if (state->path_ref.read_cursor == state->path_end_cursor) {
            state->path_reader_state = READ_BACKWATERMARK;
            tail_call_erpc_progs(ctx, ERPC_DR_RESOLVE_PATH_WATERMARK_READER_KEY);
            err = DR_ERPC_TAIL_CALL_ERROR;
            goto exit;
        }
        long len = bpf_probe_read_str(path_chunk, sizeof(path_chunk), &rb->buffer[state->path_ref.read_cursor % RING_BUFFER_SIZE]);
        if (len <= 0) {
            err = DR_ERPC_CACHE_MISS; // TODO: use a specific error type for this
            goto exit;
        }
        int ret = bpf_probe_write_user((void *)state->userspace_buffer + state->cursor, path_chunk, sizeof(path_chunk));
        if (ret < 0) {
            err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
            goto exit;
        }
        if (len == sizeof(path_chunk) && rb->buffer[(state->path_ref.read_cursor + sizeof(path_chunk) - 1) % RING_BUFFER_SIZE] != '\0') {
            state->path_ref.read_cursor -= 1;
            state->cursor -= 1;
        }
        state->path_ref.read_cursor += len;
        state->cursor += len;
    }

    tail_call_erpc_progs(ctx, ERPC_DR_RESOLVE_PATH_DATA_READER_KEY);
    err = DR_ERPC_TAIL_CALL_ERROR;

exit:
    monitor_resolution_err(err);
    return 0;
}

#endif
