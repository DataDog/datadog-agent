#ifndef _HELPERS_FILESYSTEM_H_
#define _HELPERS_FILESYSTEM_H_

#include "constants/custom.h"
#include "constants/enums.h"
#include "constants/offsets/filesystem.h"
#include "events_definition.h"
#include "maps.h"
#include "perf_ring.h"

#include "dentry_resolver.h"
#include "discarders.h"

void __attribute__((always_inline)) invalidate_inode(struct pt_regs *ctx, u32 mount_id, u64 inode, int send_invalidate_event) {
    if (!inode || !mount_id) {
        return;
    }

    expire_inode_discarders(mount_id, inode);

    if (send_invalidate_event) {
        // invalidate dentry
        struct invalidate_dentry_event_t event = {
            .inode = inode,
            .mount_id = mount_id,
        };

        send_event(ctx, EVENT_INVALIDATE_DENTRY, event);
    }
}

static __attribute__((always_inline)) u32 get_path_id(int invalidate) {
    u32 key = 0;

    u32 *prev_id = bpf_map_lookup_elem(&path_id, &key);
    if (!prev_id) {
        return 0;
    }

    u32 id = *prev_id;

    // need to invalidate the current path id for event which may change the association inode/name like
    // unlink, rename, rmdir.
    if (invalidate) {
        __sync_fetch_and_add(prev_id, 1);
    }

    return id;
}

static __attribute__((always_inline)) void inc_mount_ref(u32 mount_id) {
    u32 key = mount_id;
    struct mount_ref_t zero = {};

    bpf_map_update_elem(&mount_ref, &key, &zero, BPF_NOEXIST);
    struct mount_ref_t *ref = bpf_map_lookup_elem(&mount_ref, &key);
    if (ref) {
        __sync_fetch_and_add(&ref->counter, 1);
    }
}

static __attribute__((always_inline)) void dec_mount_ref(struct pt_regs *ctx, u32 mount_id) {
    u32 key = mount_id;
    struct mount_ref_t *ref = bpf_map_lookup_elem(&mount_ref, &key);
    if (ref) {
        __sync_fetch_and_add(&ref->counter, -1);
        if (ref->counter > 0 || !ref->umounted) {
            return;
        }
        bpf_map_delete_elem(&mount_ref, &key);
    } else {
        return;
    }

    bump_mount_discarder_revision(mount_id);

    struct mount_released_event_t event = {
        .mount_id = mount_id,
    };

    send_event(ctx, EVENT_MOUNT_RELEASED, event);
}

static __attribute__((always_inline)) void umounted(struct pt_regs *ctx, u32 mount_id) {
    u32 key = mount_id;
    struct mount_ref_t *ref = bpf_map_lookup_elem(&mount_ref, &key);
    if (ref) {
        if (ref->counter <= 0) {
            bpf_map_delete_elem(&mount_ref, &key);
        } else {
            ref->umounted = 1;
            return;
        }
    }

    bump_mount_discarder_revision(mount_id);

    struct mount_released_event_t event = {
        .mount_id = mount_id,
    };

    send_event(ctx, EVENT_MOUNT_RELEASED, event);
}

void __attribute__((always_inline)) fill_resolver_mnt(struct pt_regs *ctx, struct syscall_cache_t *syscall) {
    struct dentry *dentry = get_vfsmount_dentry(get_mount_vfsmount(syscall->unshare_mntns.mnt));
    syscall->unshare_mntns.root_key.mount_id = get_mount_mount_id(syscall->unshare_mntns.mnt);
    syscall->unshare_mntns.root_key.ino = get_dentry_ino(dentry);

    struct super_block *sb = get_dentry_sb(dentry);
    struct file_system_type *s_type = get_super_block_fs(sb);
    bpf_probe_read(&syscall->unshare_mntns.fstype, sizeof(syscall->unshare_mntns.fstype), &s_type->name);

    syscall->resolver.key = syscall->unshare_mntns.root_key;
    syscall->resolver.dentry = dentry;
    syscall->resolver.discarder_type = 0;
    syscall->resolver.callback = DR_UNSHARE_MNTNS_STAGE_ONE_CALLBACK_KPROBE_KEY;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE);
}

int __attribute__((always_inline)) get_pipefs_mount_id(void) {
    u32 key = 0;
    u32* val = bpf_map_lookup_elem(&pipefs_mountid, &key);
    if (val) { return *val; }
    return 0;
}

int __attribute__((always_inline)) is_pipefs_mount_id(u32 id) {
    u32 pipefs_id = get_pipefs_mount_id();
    if (!pipefs_id) { return 0; }
    return (pipefs_id == id);
}

void __attribute__((always_inline)) fill_file_metadata(struct dentry* dentry, struct file_metadata_t* file) {
    struct inode *d_inode = get_dentry_inode(dentry);

    bpf_probe_read(&file->nlink, sizeof(file->nlink), (void *)&d_inode->i_nlink);
    bpf_probe_read(&file->mode, sizeof(file->mode), &d_inode->i_mode);
    bpf_probe_read(&file->uid, sizeof(file->uid), &d_inode->i_uid);
    bpf_probe_read(&file->gid, sizeof(file->gid), &d_inode->i_gid);

    bpf_probe_read(&file->ctime, sizeof(file->ctime), &d_inode->i_ctime);
    bpf_probe_read(&file->mtime, sizeof(file->mtime), &d_inode->i_mtime);
}

#define get_dentry_key_path(dentry, path) (struct path_key_t) { .ino = get_dentry_ino(dentry), .mount_id = get_path_mount_id(path) }
#define get_inode_key_path(inode, path) (struct path_key_t) { .ino = get_inode_ino(inode), .mount_id = get_path_mount_id(path) }

static __attribute__((always_inline)) void set_file_inode(struct dentry *dentry, struct file_t *file, int invalidate) {
    file->path_key.path_id = get_path_id(invalidate);
    if (!file->path_key.ino) {
        file->path_key.ino = get_dentry_ino(dentry);
    }

    if (is_overlayfs(dentry)) {
        set_overlayfs_ino(dentry, &file->path_key.ino, &file->flags);
    }
}

#endif
