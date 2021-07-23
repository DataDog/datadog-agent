#ifndef _DENTRY_RESOLVER_H_
#define _DENTRY_RESOLVER_H_

#include <linux/dcache.h>
#include <linux/types.h>
#include <linux/mount.h>
#include <linux/fs.h>

#include "defs.h"
#include "filters.h"
#include "dentry.h"

#define DENTRY_INVALID -1
#define DENTRY_DISCARDED -2

#define FAKE_INODE_MSW 0xdeadc001UL

#define DR_MAX_TAIL_CALL          30
#define DR_MAX_ITERATION_DEPTH    50
#define DR_MAX_SEGMENT_LENGTH     255

struct path_leaf_t {
  struct path_key_t parent;
  char name[DR_MAX_SEGMENT_LENGTH + 1];
  u16 len;
};

struct bpf_map_def SEC("maps/pathnames") pathnames = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct path_key_t),
    .value_size = sizeof(struct path_leaf_t),
    .max_entries = 64000,
    .pinning = 0,
    .namespace = "",
};

#define DR_NO_CALLBACK -1

enum dr_kprobe_progs
{
    DR_OPEN_CALLBACK_KPROBE_KEY = 1,
    DR_SETATTR_CALLBACK_KPROBE_KEY,
    DR_MKDIR_CALLBACK_KPROBE_KEY,
    DR_MOUNT_CALLBACK_KPROBE_KEY,
    DR_SECURITY_INODE_RMDIR_CALLBACK_KPROBE_KEY,
    DR_SETXATTR_CALLBACK_KPROBE_KEY,
    DR_UNLINK_CALLBACK_KPROBE_KEY,
    DR_LINK_SRC_CALLBACK_KPROBE_KEY,
    DR_LINK_DST_CALLBACK_KPROBE_KEY,
    DR_RENAME_CALLBACK_KPROBE_KEY,
    DR_SELINUX_CALLBACK_KPROBE_KEY,
};

struct bpf_map_def SEC("maps/dentry_resolver_kprobe_callbacks") dentry_resolver_kprobe_callbacks = {
    .type = BPF_MAP_TYPE_PROG_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = EVENT_MAX,
};

enum dr_tracepoint_progs
{
    DR_OPEN_CALLBACK_TRACEPOINT_KEY = 1,
    DR_MKDIR_CALLBACK_TRACEPOINT_KEY,
    DR_MOUNT_CALLBACK_TRACEPOINT_KEY,
    DR_LINK_DST_CALLBACK_TRACEPOINT_KEY,
    DR_RENAME_CALLBACK_TRACEPOINT_KEY,
};

struct bpf_map_def SEC("maps/dentry_resolver_tracepoint_callbacks") dentry_resolver_tracepoint_callbacks = {
    .type = BPF_MAP_TYPE_PROG_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = EVENT_MAX,
};

#define DR_KPROBE     1
#define DR_TRACEPOINT 2

#define DR_ERPC_KEY                        0
#define DR_KPROBE_DENTRY_RESOLVER_KERN_KEY 1

struct bpf_map_def SEC("maps/dentry_resolver_kprobe_progs") dentry_resolver_kprobe_progs = {
    .type = BPF_MAP_TYPE_PROG_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 2,
};

#define DR_TRACEPOINT_DENTRY_RESOLVER_KERN_KEY 0

struct bpf_map_def SEC("maps/dentry_resolver_tracepoint_progs") dentry_resolver_tracepoint_progs = {
    .type = BPF_MAP_TYPE_PROG_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
};

int __attribute__((always_inline)) resolve_dentry_tail_call(struct dentry_resolver_input_t *input) {
    struct path_leaf_t map_value = {};
    struct path_key_t key = input->key;
    struct path_key_t next_key = input->key;
    struct qstr qstr;
    struct dentry *dentry = input->dentry;
    struct dentry *d_parent;
    struct inode *d_inode = NULL;
    int segment_len = 0;

    if (key.ino == 0 || key.mount_id == 0) {
        return DENTRY_INVALID;
    }

#pragma unroll
    for (int i = 0; i < DR_MAX_ITERATION_DEPTH; i++)
    {
        d_parent = NULL;
        bpf_probe_read(&d_parent, sizeof(d_parent), &dentry->d_parent);

        key = next_key;
        if (dentry != d_parent) {
            write_dentry_inode(d_parent, &d_inode);
            write_inode_ino(d_inode, &next_key.ino);
        }

        // discard filename and its parent only in order to limit the number of lookup
        if (input->discarder_type && i < 2) {
            if (is_discarded_by_inode(input->discarder_type, key.mount_id, key.ino, i == 0)) {
                return DENTRY_DISCARDED;
            }
        }

        bpf_probe_read(&qstr, sizeof(qstr), &dentry->d_name);
        segment_len = bpf_probe_read_str(&map_value.name, sizeof(map_value.name), (void *)qstr.name);
        if (segment_len > 0) {
            map_value.len = (u16) segment_len;
        } else {
            map_value.len = 0;
        }

        if (map_value.name[0] == '/' || map_value.name[0] == 0) {
            map_value.name[0] = '/';
            next_key.ino = 0;
            next_key.mount_id = 0;
        }

        map_value.parent = next_key;

        bpf_map_update_elem(&pathnames, &key, &map_value, BPF_ANY);

        dentry = d_parent;
        if (next_key.ino == 0) {
            input->dentry = d_parent;
            input->key = next_key;
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
    syscall->resolver.ret = resolve_dentry_tail_call(&syscall->resolver);                                              \
                                                                                                                       \
    if (syscall->resolver.ret > 0) {                                                                                   \
        if (syscall->resolver.iteration < DR_MAX_TAIL_CALL && syscall->resolver.key.ino != 0) {                        \
            bpf_tail_call(ctx, progs_map, dentry_resolver_kern_key);                                                   \
        }                                                                                                              \
                                                                                                                       \
        syscall->resolver.ret += DR_MAX_ITERATION_DEPTH * (syscall->resolver.iteration - 1);                           \
    }                                                                                                                  \
                                                                                                                       \
    if (syscall->resolver.callback >= 0) {                                                                             \
        bpf_tail_call(ctx, callbacks_map, syscall->resolver.callback);                                                 \
    }                                                                                                                  \

SEC("kprobe/dentry_resolver_kern")
int kprobe__dentry_resolver_kern(struct pt_regs *ctx) {
    dentry_resolver_kern(ctx, &dentry_resolver_kprobe_progs, &dentry_resolver_kprobe_callbacks, DR_KPROBE_DENTRY_RESOLVER_KERN_KEY);
    return 0;
}

SEC("tracepoint/dentry_resolver_kern")
int tracepoint__dentry_resolver_kern(void *ctx) {
    dentry_resolver_kern(ctx, &dentry_resolver_tracepoint_progs, &dentry_resolver_tracepoint_callbacks, DR_TRACEPOINT_DENTRY_RESOLVER_KERN_KEY);
    return 0;
}

struct dr_erpc_state_t {
    char *userspace_buffer;
    struct path_key_t key;
    int ret;
    int iteration;
    u32 buffer_size;
    u16 cursor;
};

struct bpf_map_def SEC("maps/dr_erpc_state") dr_erpc_state = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct dr_erpc_state_t),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

#define DR_ERPC_OK                0
#define DR_ERPC_CACHE_MISS        1
#define DR_ERPC_BUFFER_SIZE       2
#define DR_ERPC_WRITE_PAGE_FAULT  3
#define DR_ERPC_TAIL_CALL_ERROR   4
#define DR_ERPC_READ_PAGE_FAULT   5
#define DR_ERPC_UNKNOWN_ERROR     6

struct dr_erpc_stats_t {
    u64 count;
};

struct bpf_map_def SEC("maps/dr_erpc_stats_fb") dr_erpc_stats_fb = {
    .type = BPF_MAP_TYPE_PERCPU_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct dr_erpc_stats_t),
    .max_entries = 6,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/dr_erpc_stats_bb") dr_erpc_stats_bb = {
    .type = BPF_MAP_TYPE_PERCPU_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct dr_erpc_stats_t),
    .max_entries = 6,
    .pinning = 0,
    .namespace = "",
};

SEC("kprobe/dentry_resolver_erpc")
int kprobe__dentry_resolver_erpc(struct pt_regs *ctx) {
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
        if (state->key.ino == 0)
            goto exit;
    }
    if (state->iteration < DR_MAX_TAIL_CALL) {
        bpf_tail_call(ctx, &dentry_resolver_kprobe_progs, DR_ERPC_KEY);
        resolution_err = DR_ERPC_TAIL_CALL_ERROR;
    }

exit:
    if (resolution_err > 0) {
        struct bpf_map_def *erpc_stats = select_buffer(&dr_erpc_stats_fb, &dr_erpc_stats_bb, ERPC_MONITOR_KEY);
        if (erpc_stats == NULL) {
            return 0;
        }

        struct dr_erpc_stats_t *stats = bpf_map_lookup_elem(erpc_stats, &resolution_err);
        if (stats == NULL) {
            return 0;
        }
        __sync_fetch_and_add(&stats->count, 1);
    }
    return 0;
}

int __attribute__((always_inline)) handle_resolve_path(struct pt_regs* ctx, void *data) {
    u32 key = 0;
    u32 err = 0;
    struct dr_erpc_state_t *state = bpf_map_lookup_elem(&dr_erpc_state, &key);
    if (state == NULL) {
        return 0;
    }

    int ret = bpf_probe_read(&state->key, sizeof(state->key), data);
    if (ret < 0) {
        err = DR_ERPC_READ_PAGE_FAULT;
        goto error;
    }
    ret = bpf_probe_read(&state->userspace_buffer, sizeof(state->userspace_buffer), data + sizeof(state->key));
    if (ret < 0) {
        err = DR_ERPC_READ_PAGE_FAULT;
        goto error;
    }
    ret = bpf_probe_read(&state->buffer_size, sizeof(state->buffer_size), data + sizeof(state->key) + sizeof(state->userspace_buffer));
    if (ret < 0) {
        err = DR_ERPC_READ_PAGE_FAULT;
        goto error;
    }

    state->iteration = 0;
    state->ret = 0;
    state->cursor = 0;

    bpf_tail_call(ctx, &dentry_resolver_kprobe_progs, DR_ERPC_KEY);

error:
    if (err > 0) {
        struct bpf_map_def *erpc_stats = select_buffer(&dr_erpc_stats_fb, &dr_erpc_stats_bb, ERPC_MONITOR_KEY);
        if (erpc_stats == NULL) {
            return 0;
        }

        struct dr_erpc_stats_t *stats = bpf_map_lookup_elem(erpc_stats, &err);
        if (stats == NULL) {
            return 0;
        }
        __sync_fetch_and_add(&stats->count, 1);
    }
    return 0;
}

int __attribute__((always_inline)) handle_resolve_segment(void *data) {
    struct path_key_t key = {};
    char *userspace_buffer = 0;
    u32 buffer_size = 0;
    u32 resolution_err = 0;

    int ret = bpf_probe_read(&key, sizeof(key), data);
    if (ret < 0) {
        resolution_err = DR_ERPC_READ_PAGE_FAULT;
        goto exit;
    }
    ret = bpf_probe_read(&userspace_buffer, sizeof(userspace_buffer), data + sizeof(key));
    if (ret < 0) {
        resolution_err = DR_ERPC_READ_PAGE_FAULT;
        goto exit;
    }
    ret = bpf_probe_read(&buffer_size, sizeof(buffer_size), data + sizeof(key) + sizeof(userspace_buffer));
    if (ret < 0) {
        resolution_err = DR_ERPC_READ_PAGE_FAULT;
        goto exit;
    }

    // resolve segment and write in buffer
    struct path_leaf_t *map_value = bpf_map_lookup_elem(&pathnames, &key);
    if (map_value == NULL) {
        resolution_err = DR_ERPC_CACHE_MISS;
        goto exit;
    }

    if (map_value->len + sizeof(key) > buffer_size) {
        // make sure we do not write outside of the provided buffer
        resolution_err = DR_ERPC_BUFFER_SIZE;
        goto exit;
    }

    ret = bpf_probe_write_user((void *) userspace_buffer, &key, sizeof(key));
    if (ret < 0) {
        resolution_err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
        goto exit;
    }

    ret = bpf_probe_write_user((void *) userspace_buffer + sizeof(key), map_value->name, DR_MAX_SEGMENT_LENGTH + 1);
    if (ret < 0) {
        resolution_err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
        goto exit;
    }

exit:
    if (resolution_err > 0) {
        struct bpf_map_def *erpc_stats = select_buffer(&dr_erpc_stats_fb, &dr_erpc_stats_bb, ERPC_MONITOR_KEY);
        if (erpc_stats == NULL) {
            return 0;
        }

        struct dr_erpc_stats_t *stats = bpf_map_lookup_elem(erpc_stats, &resolution_err);
        if (stats == NULL) {
            return 0;
        }
        __sync_fetch_and_add(&stats->count, 1);
    }
    return 0;
}

int __attribute__((always_inline)) handle_resolve_parent(void *data) {
    struct path_key_t key = {};
    char *userspace_buffer = 0;
    u32 buffer_size = 0;
    u32 resolution_err = 0;

    int ret = bpf_probe_read(&key, sizeof(key), data);
    if (ret < 0) {
        resolution_err = DR_ERPC_READ_PAGE_FAULT;
        goto exit;
    }
    ret = bpf_probe_read(&userspace_buffer, sizeof(userspace_buffer), data + sizeof(key));
    if (ret < 0) {
        resolution_err = DR_ERPC_READ_PAGE_FAULT;
        goto exit;
    }
    ret = bpf_probe_read(&buffer_size, sizeof(buffer_size), data + sizeof(key) + sizeof(userspace_buffer));
    if (ret < 0) {
        resolution_err = DR_ERPC_READ_PAGE_FAULT;
        goto exit;
    }

    // resolve segment and write in buffer
    struct path_leaf_t *map_value = bpf_map_lookup_elem(&pathnames, &key);
    if (map_value == NULL) {
        resolution_err = DR_ERPC_CACHE_MISS;
        goto exit;
    }

    if (sizeof(map_value->parent) > buffer_size) {
        // make sure we do not write outside of the provided buffer
        resolution_err = DR_ERPC_BUFFER_SIZE;
        goto exit;
    }

    ret = bpf_probe_write_user((void *) userspace_buffer, &map_value->parent, sizeof(map_value->parent));
    if (ret < 0) {
        resolution_err = ret == -14 ? DR_ERPC_WRITE_PAGE_FAULT : DR_ERPC_UNKNOWN_ERROR;
        goto exit;
    }

exit:
    if (resolution_err > 0) {
        struct bpf_map_def *erpc_stats = select_buffer(&dr_erpc_stats_fb, &dr_erpc_stats_bb, ERPC_MONITOR_KEY);
        if (erpc_stats == NULL) {
            return 0;
        }

        struct dr_erpc_stats_t *stats = bpf_map_lookup_elem(erpc_stats, &resolution_err);
        if (stats == NULL) {
            return 0;
        }
        __sync_fetch_and_add(&stats->count, 1);
    }
    return 0;
}

int __attribute__((always_inline)) resolve_dentry(void *ctx, int dr_type) {
    if (dr_type == DR_KPROBE) {
        bpf_tail_call(ctx, &dentry_resolver_kprobe_progs, DR_KPROBE_DENTRY_RESOLVER_KERN_KEY);
    } else if (dr_type == DR_TRACEPOINT) {
        bpf_tail_call(ctx, &dentry_resolver_tracepoint_progs, DR_TRACEPOINT_DENTRY_RESOLVER_KERN_KEY);
    }
    return 0;
}

#endif
