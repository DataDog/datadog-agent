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

static __attribute__((always_inline)) void bump_path_id(u32 mount_id) {
    u32 key = mount_id % PATH_ID_MAP_SIZE;

    u32 *id = bpf_map_lookup_elem(&path_id, &key);
    if (id) {
        __sync_fetch_and_add(id, 1);
    }
}

static __attribute__((always_inline)) u32 get_path_id(u32 mount_id, int invalidate) {
    u32 key = mount_id % PATH_ID_MAP_SIZE;

    u32 *id = bpf_map_lookup_elem(&path_id, &key);
    if (!id) {
        return 0;
    }

    u32 id_value = *id;

    // need to invalidate the current path id for event which may change the association inode/name like
    // unlink, rename, rmdir.
    if (invalidate) {
        __sync_fetch_and_add(id, 1);
    }

    return id_value;
}

static __attribute__((always_inline)) void update_path_id(struct path_key_t *path_key, int invalidate) {
    path_key->path_id = get_path_id(path_key->mount_id, invalidate);
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

static __attribute__((always_inline)) void dec_mount_ref(ctx_t *ctx, u32 mount_id) {
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
    bump_path_id(mount_id);

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
    bump_path_id(mount_id);

    struct mount_released_event_t event = {
        .mount_id = mount_id,
    };

    send_event(ctx, EVENT_MOUNT_RELEASED, event);
}

static __attribute__((always_inline)) void set_file_layer(struct dentry *dentry, struct file_t *file) {
    if (is_overlayfs(dentry)) {
        u32 flags = get_overlayfs_layer(dentry);
        file->flags |= flags;
    }
}

void __attribute__((always_inline)) fill_file(struct dentry *dentry, struct file_t *file) {
    struct inode *d_inode = get_dentry_inode(dentry);

    file->dev = get_dentry_dev(dentry);

    // nlink is mostly used userspace side to invalidate cache. use the higher value found
    u64 inode_nlink_offset;
    LOAD_CONSTANT("inode_nlink_offset", inode_nlink_offset);

    u32 nlink = 0;
    bpf_probe_read(&nlink, sizeof(nlink), (void *)d_inode + inode_nlink_offset);
    if (nlink > file->metadata.nlink) {
      file->metadata.nlink = nlink;
    }

    u64 inode_gid_offset;
    LOAD_CONSTANT("inode_gid_offset", inode_gid_offset);

    bpf_probe_read(&file->metadata.mode, sizeof(file->metadata.mode), &d_inode->i_mode);
    bpf_probe_read(&file->metadata.uid, sizeof(file->metadata.uid), &d_inode->i_uid);
    bpf_probe_read(&file->metadata.gid, sizeof(file->metadata.gid), (void *)d_inode + inode_gid_offset);

    u64 inode_ctime_sec_offset;
    LOAD_CONSTANT("inode_ctime_sec_offset", inode_ctime_sec_offset);
    u64 inode_ctime_nsec_offset;
    LOAD_CONSTANT("inode_ctime_nsec_offset", inode_ctime_nsec_offset);

	if (inode_ctime_sec_offset && inode_ctime_nsec_offset) {
		bpf_probe_read(&file->metadata.ctime.tv_sec, sizeof(file->metadata.ctime.tv_sec), (void *)d_inode + inode_ctime_sec_offset);
		u32 nsec;
		bpf_probe_read(&nsec, sizeof(nsec), (void *)d_inode + inode_ctime_nsec_offset);
		file->metadata.ctime.tv_nsec = nsec;
	} else {
#if LINUX_VERSION_CODE < KERNEL_VERSION(6, 11, 0)
    u64 inode_ctime_offset;
    LOAD_CONSTANT("inode_ctime_offset", inode_ctime_offset);
    bpf_probe_read(&file->metadata.ctime, sizeof(file->metadata.ctime), (void *)d_inode + inode_ctime_offset);
#else
    bpf_probe_read(&file->metadata.ctime.tv_sec, sizeof(file->metadata.ctime.tv_sec), &d_inode->i_ctime_sec);
    bpf_probe_read(&file->metadata.ctime.tv_nsec, sizeof(file->metadata.ctime.tv_nsec), &d_inode->i_ctime_nsec);
#endif
	}

    u64 inode_mtime_sec_offset;
    LOAD_CONSTANT("inode_mtime_sec_offset", inode_mtime_sec_offset);
    u64 inode_mtime_nsec_offset;
    LOAD_CONSTANT("inode_mtime_nsec_offset", inode_mtime_nsec_offset);

	if (inode_mtime_sec_offset && inode_mtime_nsec_offset) {
		bpf_probe_read(&file->metadata.mtime.tv_sec, sizeof(file->metadata.mtime.tv_sec), (void *)d_inode + inode_mtime_sec_offset);
		u32 nsec;
		bpf_probe_read(&nsec, sizeof(nsec), (void *)d_inode + inode_mtime_nsec_offset);
		file->metadata.mtime.tv_nsec = nsec;
	} else {
#if LINUX_VERSION_CODE < KERNEL_VERSION(6, 11, 0)
    u64 inode_mtime_offset;
    LOAD_CONSTANT("inode_mtime_offset", inode_mtime_offset);
    bpf_probe_read(&file->metadata.mtime, sizeof(file->metadata.mtime), (void *)d_inode + inode_mtime_offset);
#else
    bpf_probe_read(&file->metadata.mtime.tv_sec, sizeof(file->metadata.mtime.tv_sec), &d_inode->i_mtime_sec);
    bpf_probe_read(&file->metadata.mtime.tv_nsec, sizeof(file->metadata.mtime.tv_nsec), &d_inode->i_mtime_nsec);
#endif
	}

    // set again the layer here as after update a file will be moved to the upper layer
    set_file_layer(dentry, file);
}

#define get_dentry_key_path(dentry, path)                                  \
    (struct path_key_t) {                                                  \
        .ino = get_dentry_ino(dentry), .mount_id = get_path_mount_id(path) \
    }
#define get_inode_key_path(inode, path)                                  \
    (struct path_key_t) {                                                \
        .ino = get_inode_ino(inode), .mount_id = get_path_mount_id(path) \
    }

static __attribute__((always_inline)) void set_file_inode(struct dentry *dentry, struct file_t *file, int invalidate) {
    file->path_key.path_id = get_path_id(file->path_key.mount_id, invalidate);
    if (!file->path_key.ino) {
        file->path_key.ino = get_dentry_ino(dentry);
    }

    if (is_overlayfs(dentry)) {
        set_overlayfs_inode(dentry, file);
        set_overlayfs_nlink(dentry, file);
    }
}

#endif
