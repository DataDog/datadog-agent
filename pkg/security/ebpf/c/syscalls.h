#ifndef _SYSCALLS_H_
#define _SYSCALLS_H_

#include "filters.h"
#include "process.h"
#include "bpf_const.h"

#define FSTYPE_LEN 16

struct str_array_ref_t {
    u32 id;
    u8 index;
    u8 truncated;
    const char **array;
};

struct dentry_resolver_input_t {
    struct path_key_t key;
    struct dentry *dentry;
    u64 discarder_type;
    int callback;
    int ret;
    int iteration;
};

union selinux_write_payload_t {
    // 1 for true, 0 for false, -1 (max) for error
    u32 bool_value;
    struct {
        u16 disable_value;
        u16 enforce_value;
    } status;
};

struct syscall_cache_t {
    struct policy_t policy;
    u64 type;
    u32 discarded;

    struct dentry_resolver_input_t resolver;

    union {
        struct {
            int flags;
            umode_t mode;
            struct dentry *dentry;
            struct file_t file;
        } open;

        struct {
            umode_t mode;
            struct dentry *dentry;
            struct path *path;
            struct file_t file;
        } mkdir;

        struct {
            struct dentry *dentry;
            struct file_t file;
            int flags;
        } unlink;

        struct {
            struct dentry *dentry;
            struct file_t file;
        } rmdir;

        struct {
            struct file_t src_file;
            unsigned long src_inode;
            struct dentry *src_dentry;
            struct dentry *target_dentry;
            struct file_t target_file;
        } rename;

        struct {
            struct dentry *dentry;
            struct path *path;
            struct file_t file;
            union {
                umode_t mode;
                struct {
                    uid_t user;
                    gid_t group;
                };
                struct {
                    struct ktimeval atime;
                    struct ktimeval mtime;
                };
            };
        } setattr;

        struct {
            struct mount *src_mnt;
            struct mount *dest_mnt;
            struct mountpoint *dest_mountpoint;
            struct path_key_t root_key;
            struct path_key_t path_key;
            const char *fstype;
        } mount;

        struct {
            struct vfsmount *vfs;
        } umount;

        struct {
            struct file_t src_file;
            struct path *target_path;
            struct dentry *src_dentry;
            struct dentry *target_dentry;
            struct file_t target_file;
        } link;

        struct {
            struct dentry *dentry;
            struct file_t file;
            const char *name;
        } xattr;

        struct {
            struct dentry *dentry;
            struct file_t file;
            struct str_array_ref_t args;
            struct str_array_ref_t envs;
            struct span_context_t span_context;
            u32 next_tail;
            u8 is_parsed;
        } exec;

        struct {
            struct dentry *dentry;
            struct file_t file;
            u32 event_kind;
            union selinux_write_payload_t payload;
        } selinux;

        struct {
            int cmd;
            u32 map_id;
            u32 prog_id;
            int retval;
            u64 helpers[3];
            union bpf_attr_def *attr;
        } bpf;
    };
};

struct bpf_map_def SEC("maps/syscalls") syscalls = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u64),
    .value_size = sizeof(struct syscall_cache_t),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

struct policy_t __attribute__((always_inline)) fetch_policy(u64 event_type) {
    struct policy_t *policy = bpf_map_lookup_elem(&filter_policy, &event_type);
    if (policy) {
        return *policy;
    }
    struct policy_t empty_policy = { };
    return empty_policy;
}

// cache_syscall checks the event policy in order to see if the syscall struct can be cached
void __attribute__((always_inline)) cache_syscall(struct syscall_cache_t *syscall) {
    u64 key = bpf_get_current_pid_tgid();
    bpf_map_update_elem(&syscalls, &key, syscall, BPF_ANY);
}

struct syscall_cache_t * __attribute__((always_inline)) peek_syscall(u64 type) {
    u64 key = bpf_get_current_pid_tgid();
    struct syscall_cache_t *syscall = (struct syscall_cache_t *) bpf_map_lookup_elem(&syscalls, &key);
    if (!syscall) {
        return NULL;
    }
    if (!type || syscall->type == type) {
        return syscall;
    }
    return NULL;
}

struct syscall_cache_t * __attribute__((always_inline)) peek_syscall_with(int (*predicate)(u64 type)) {
    u64 key = bpf_get_current_pid_tgid();
    struct syscall_cache_t *syscall = (struct syscall_cache_t *) bpf_map_lookup_elem(&syscalls, &key);
    if (!syscall) {
        return NULL;
    }
    if (predicate(syscall->type)) {
        return syscall;
    }
    return NULL;
}

struct syscall_cache_t * __attribute__((always_inline)) pop_syscall_with(int (*predicate)(u64 type)) {
    u64 key = bpf_get_current_pid_tgid();
    struct syscall_cache_t *syscall = (struct syscall_cache_t *) bpf_map_lookup_elem(&syscalls, &key);
    if (!syscall) {
        return NULL;
    }
    if (predicate(syscall->type)) {
        bpf_map_delete_elem(&syscalls, &key);
        return syscall;
    }
    return NULL;
}

struct syscall_cache_t * __attribute__((always_inline)) pop_syscall(u64 type) {
    u64 key = bpf_get_current_pid_tgid();
    struct syscall_cache_t *syscall = (struct syscall_cache_t *) bpf_map_lookup_elem(&syscalls, &key);
    if (!syscall) {
        return NULL;
    }
    if (!type || syscall->type == type) {
        bpf_map_delete_elem(&syscalls, &key);
        return syscall;
    }
    return NULL;
}

int __attribute__((always_inline)) discard_syscall(struct syscall_cache_t *syscall) {
    u64 key = bpf_get_current_pid_tgid();
    bpf_map_delete_elem(&syscalls, &key);
    return 0;
}

int __attribute__((always_inline)) mark_as_discarded(struct syscall_cache_t *syscall) {
    syscall->discarded = 1;
    return 0;
}

int __attribute__((always_inline)) filter_syscall(struct syscall_cache_t *syscall, int (*check_approvers)(struct syscall_cache_t *syscall)) {
    if (syscall->policy.mode == NO_FILTER)
        return 0;

    char pass_to_userspace = syscall->policy.mode == ACCEPT ? 1 : 0;

    if (syscall->policy.mode == DENY) {
        pass_to_userspace = check_approvers(syscall);
    }

    return !pass_to_userspace;
}

#endif
