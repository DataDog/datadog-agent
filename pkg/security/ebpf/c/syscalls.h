#ifndef _SYSCALLS_H_
#define _SYSCALLS_H_

#include "filters.h"

#define FSTYPE_LEN 16

struct ktimeval {
    long tv_sec;
    long tv_nsec;
};

struct syscall_cache_t {
    struct policy_t policy;

    u16 type;

    union {
        struct {
            int flags;
            umode_t mode;
            struct dentry *dentry;
            struct path_key_t path_key;
            u64 real_inode;
        } open;

        struct {
            umode_t mode;
            struct dentry *dentry;
            struct dentry *real_dentry;
            struct path *path;
            struct path_key_t path_key;
        } mkdir;

        struct {
            struct path_key_t path_key;
            int overlay_numlower;
            int flags;
            u64 real_inode;
        } unlink;

        struct {
            struct path_key_t path_key;
            int overlay_numlower;
            u64 real_inode;
        } rmdir;

        struct {
            struct path_key_t src_key;
            unsigned long src_inode;
            struct dentry *src_dentry;
            struct dentry *real_src_dentry;
            struct path_key_t target_key;
            int src_overlay_numlower;
        } rename;

        struct {
            struct dentry *dentry;
            struct path *path;
            struct path_key_t path_key;
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
            u64 real_inode;
        } setattr;

        struct {
            struct mount *src_mnt;
            struct mount *dest_mnt;
            struct mountpoint *dest_mountpoint;
            struct path_key_t root_key;
            const char *fstype;
        } mount;

        struct {
            struct vfsmount *vfs;
        } umount;

        struct {
            struct path_key_t src_key;
            struct path *target_path;
            struct dentry *target_dentry;
            struct path_key_t target_key;
            int src_overlay_numlower;
            u64 real_src_inode;
        } link;

        struct {
            struct dentry *dentry;
            struct path_key_t path_key;
            const char *name;
            u64 real_inode;
        } setxattr;
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

void __attribute__((always_inline)) cache_syscall(struct syscall_cache_t *syscall) {
    u64 key = bpf_get_current_pid_tgid();
    bpf_map_update_elem(&syscalls, &key, syscall, BPF_ANY);
}

struct syscall_cache_t * __attribute__((always_inline)) peek_syscall(u16 type) {
    u64 key = bpf_get_current_pid_tgid();
    struct syscall_cache_t *syscall = (struct syscall_cache_t *) bpf_map_lookup_elem(&syscalls, &key);
    if (syscall && (syscall->type & type) > 0)
        return syscall;
    return NULL;
}

struct syscall_cache_t * __attribute__((always_inline)) pop_syscall(u16 type) {
    u64 key = bpf_get_current_pid_tgid();
    struct syscall_cache_t *syscall = (struct syscall_cache_t *) bpf_map_lookup_elem(&syscalls, &key);
    if (syscall && (syscall->type & type) > 0) {
        bpf_map_delete_elem(&syscalls, &key);
        return syscall;
    }
    return NULL;
}

#endif
