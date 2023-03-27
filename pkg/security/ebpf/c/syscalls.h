#ifndef _SYSCALLS_H_
#define _SYSCALLS_H_

#include "filters.h"
#include "process.h"
#include "bpf_const.h"

#define FSTYPE_LEN 16

enum {
    SYNC_SYSCALL = 0,
    ASYNC_SYSCALL
};

struct args_envs_t {
    u32 count;          // argc/envc retrieved from the kernel
    u32 counter;        // counter incremented while parsing args/envs
    u32 id;
    u8 truncated;
};

struct args_envs_parsing_context_t {
    const char *args_start;
    u64 envs_offset;
    u64 parsing_offset;
    u32 args_count;
};

enum {
    ACTIVITY_DUMP_RUNNING = 1<<0, // defines if an activity dump is running
    SAVED_BY_ACTIVITY_DUMP = 1<<1, // defines if the dentry should have been discarded, but was saved because of an activity dump
};

struct dentry_resolver_input_t {
    struct path_key_t key;
    struct dentry *dentry;
    u64 discarder_type;
    int callback;
    int ret;
    int iteration;
    u32 flags;
};

union selinux_write_payload_t {
    // 1 for true, 0 for false, -1 (max) for error
    u32 bool_value;
    struct {
        u16 disable_value;
        u16 enforce_value;
    } status;
};

// linux_binprm_t contains content from the linux_binprm struct, which holds the arguments used for loading binaries
// We only need enough information from the executable field to be able to resolve the dentry.
struct linux_binprm_t {
    struct path_key_t interpreter;
};

struct syscall_cache_t {
    struct policy_t policy;
    u64 type;
    u8 discarded;
    u8 async;

    struct dentry_resolver_input_t resolver;

    union {
        struct {
            int flags;
            umode_t mode;
            struct dentry *dentry;
            struct file_t file;
            u64 pid_tgid;
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
            struct mount *bind_src_mnt;
            struct mountpoint *dest_mountpoint;
            struct path_key_t root_key;
            struct path_key_t path_key;
            struct path_key_t bind_src_key;
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
            struct args_envs_t args;
            struct args_envs_t envs;
            struct args_envs_parsing_context_t args_envs_ctx;
            struct span_context_t span_context;
            struct linux_binprm_t linux_binprm;
            u8 is_parsed;
        } exec;

        struct {
            u32 is_thread;
            struct pid *pid;
        } fork;

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

        struct {
            u32 request;
            u32 pid;
            u64 addr;
        } ptrace;

        struct {
            u64 offset;
            u32 len;
            int protection;
            int flags;
            struct file_t file;
            struct dentry *dentry;
        } mmap;

        struct {
            u64 vm_start;
            u64 vm_end;
            u64 vm_protection;
            u64 req_protection;
        } mprotect;

        struct {
            struct file_t file;
            struct dentry *dentry;
            char name[MODULE_NAME_LEN];
            u32 loaded_from_memory;
        } init_module;

        struct {
            const char *name;
        } delete_module;

        struct {
            u32 namespaced_pid;
            u32 root_ns_pid;
            u32 type;
        } signal;

        struct {
            struct file_t file;
            struct dentry *dentry;
            struct pipe_buffer *bufs;
            u32 file_found;
            u32 pipe_entry_flag;
            u32 pipe_exit_flag;
        } splice;

        struct {
            u64 addr[2];
            u16 family;
            u16 port;
        } bind;

        struct {
            struct mount *mnt;
            struct mount *parent;
            struct dentry *mp_dentry;
            const char *fstype;
            struct path_key_t root_key;
            struct path_key_t path_key;
            unsigned long flags;
        } unshare_mntns;
    };
};

struct bpf_map_def SEC("maps/syscalls") syscalls = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u64),
    .value_size = sizeof(struct syscall_cache_t),
    .max_entries = 1024,
};

struct policy_t __attribute__((always_inline)) fetch_policy(u64 event_type) {
    struct policy_t *policy = bpf_map_lookup_elem(&filter_policy, &event_type);
    if (policy) {
        return *policy;
    }
    struct policy_t empty_policy = {};
    return empty_policy;
}

// cache_syscall checks the event policy in order to see if the syscall struct can be cached
void __attribute__((always_inline)) cache_syscall(struct syscall_cache_t *syscall) {
    u64 key = bpf_get_current_pid_tgid();
    bpf_map_update_elem(&syscalls, &key, syscall, BPF_ANY);
}

struct syscall_cache_t *__attribute__((always_inline)) peek_task_syscall(u64 pid_tgid, u64 type) {
    struct syscall_cache_t *syscall = (struct syscall_cache_t *)bpf_map_lookup_elem(&syscalls, &pid_tgid);
    if (!syscall) {
        return NULL;
    }
    if (!type || syscall->type == type) {
        return syscall;
    }
    return NULL;
}

struct syscall_cache_t *__attribute__((always_inline)) peek_syscall(u64 type) {
    u64 key = bpf_get_current_pid_tgid();
    return peek_task_syscall(key, type);
}

struct syscall_cache_t *__attribute__((always_inline)) peek_syscall_with(int (*predicate)(u64 type)) {
    u64 key = bpf_get_current_pid_tgid();
    struct syscall_cache_t *syscall = (struct syscall_cache_t *)bpf_map_lookup_elem(&syscalls, &key);
    if (!syscall) {
        return NULL;
    }
    if (predicate(syscall->type)) {
        return syscall;
    }
    return NULL;
}

struct syscall_cache_t *__attribute__((always_inline)) pop_syscall_with(int (*predicate)(u64 type)) {
    u64 key = bpf_get_current_pid_tgid();
    struct syscall_cache_t *syscall = (struct syscall_cache_t *)bpf_map_lookup_elem(&syscalls, &key);
    if (!syscall) {
        return NULL;
    }
    if (predicate(syscall->type)) {
        bpf_map_delete_elem(&syscalls, &key);
        return syscall;
    }
    return NULL;
}

struct syscall_cache_t *__attribute__((always_inline)) pop_task_syscall(u64 pid_tgid, u64 type) {
    struct syscall_cache_t *syscall = (struct syscall_cache_t *)bpf_map_lookup_elem(&syscalls, &pid_tgid);
    if (!syscall) {
        return NULL;
    }
    if (!type || syscall->type == type) {
        bpf_map_delete_elem(&syscalls, &pid_tgid);
        return syscall;
    }
    return NULL;
}

struct syscall_cache_t *__attribute__((always_inline)) pop_syscall(u64 type) {
    u64 key = bpf_get_current_pid_tgid();
    return pop_task_syscall(key, type);
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
    if (syscall->policy.mode == NO_FILTER) {
        return 0;
    }

    char pass_to_userspace = syscall->policy.mode == ACCEPT ? 1 : 0;

    if (syscall->policy.mode == DENY) {
        pass_to_userspace = check_approvers(syscall);
    }

    u32 tgid = bpf_get_current_pid_tgid() >> 32;
    u32 *cookie = bpf_map_lookup_elem(&traced_pids, &tgid);
    if (cookie != NULL) {
        u64 now = bpf_ktime_get_ns();
        struct activity_dump_config *config = lookup_or_delete_traced_pid(tgid, now, cookie);
        if (config != NULL) {
            // is this event type traced ?
            if (mask_has_event(config->event_mask, syscall->type)
                && activity_dump_rate_limiter_allow(config, *cookie, now, 0)) {
                if (!pass_to_userspace) {
                    syscall->resolver.flags |= SAVED_BY_ACTIVITY_DUMP;
                }
                return 0;
            }
        }
    }

    return !pass_to_userspace;
}

#endif
