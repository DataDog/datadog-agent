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

static __attribute__((always_inline)) void bump_high_path_id(u32 mount_id) {
    u32 key = mount_id % PATH_ID_HIGH_MAP_SIZE;

    u32 *id = bpf_map_lookup_elem(&path_id_high, &key);
    if (id) {
        __sync_fetch_and_add(id, 1);
    }
}

#define PATH_ID_LOW_MASK 0xFFFF
#define PATH_ID(high, low) ((u32)((high << 16) | (low & PATH_ID_LOW_MASK)))

// get the path id for a given inode, mount id and nlink
// the path id is a 32 bit integer composed of a 16 bit high id and a 16 bit low id
// the high id is the mount id and the low id is the inode id
// the low id is incremented when the nlink is greater than 1 or when the invalidate type is PATH_ID_INVALIDATE_TYPE_LOCAL, meaning rename or unlink of a regular file
// the high id is incremented when the invalidate type is PATH_ID_INVALIDATE_TYPE_GLOBAL, meaning rename or unlink of a directory
static __attribute__((always_inline)) u32 get_path_id(u64 ino, u32 mount_id, int nlink, enum PATH_ID_INVALIDATE_TYPE invalidate_type) {
    u32 key = mount_id % PATH_ID_HIGH_MAP_SIZE;

<<<<<<< HEAD
    u32 *high_id_ptr = bpf_map_lookup_elem(&path_id_high, &key);
=======
    u32 *high_id_ptr = bpf_map_lookup_elem(&path_id, &key);
>>>>>>> f9b7a48cfdc (rever low/high)
    if (!high_id_ptr) {
        return 0;
    }

    u32 high_id_value = *high_id_ptr;

<<<<<<< HEAD
    key = ino % PATH_ID_LOW_MAP_SIZE;
=======
    key = ino % PATH_ID_HIGH_MAP_SIZE;
>>>>>>> f9b7a48cfdc (rever low/high)
    u32 *low_id_ptr = bpf_map_lookup_elem(&path_id_low, &key);
    if (!low_id_ptr) {
        // will never happen
        return PATH_ID(0, 0);
    }

    if (nlink > 1) {
        __sync_fetch_and_add(low_id_ptr, 1);
    }

<<<<<<< HEAD
    u32 low_id_value = *low_id_ptr % PATH_ID_LOW_MASK;
=======
    u32 low_id_value = *low_id_ptr % 0xFFFF;
>>>>>>> f9b7a48cfdc (rever low/high)

    // need to invalidate the current path id for event which may change the association inode/name like.
    // After the operation, the path id should be incremented.
    // unlink, rename, rmdir.
    if (invalidate_type == PATH_ID_INVALIDATE_TYPE_LOCAL) {
        __sync_fetch_and_add(low_id_ptr, 1);
    } else if (invalidate_type == PATH_ID_INVALIDATE_TYPE_GLOBAL) {
        __sync_fetch_and_add(high_id_ptr, 1);
    }

    return PATH_ID(high_id_value, low_id_value);
}

static __attribute__((always_inline)) void update_path_id(struct path_key_t *path_key, int nlink, enum PATH_ID_INVALIDATE_TYPE invalidate_type) {
    path_key->path_id = get_path_id(path_key->ino, path_key->mount_id, nlink, invalidate_type);
}

static __attribute__((always_inline)) void set_file_layer(struct dentry *dentry, struct file_t *file) {
    if (is_overlayfs(dentry)) {
        u32 flags = get_overlayfs_layer(dentry);
        file->flags |= flags;
    }
}

static __attribute__((always_inline)) void fill_file(struct dentry *dentry, struct file_t *file) {
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

static __attribute__((always_inline)) void set_file_inode(struct dentry *dentry, struct file_t *file, enum PATH_ID_INVALIDATE_TYPE invalidate_type) {
    if (!file->path_key.ino) {
        file->path_key.ino = get_dentry_ino(dentry);
    }

    int nlink = get_dentry_nlink(dentry);
    if (nlink > file->metadata.nlink) {
        file->metadata.nlink = nlink;
    }

    if (is_overlayfs(dentry)) {
        set_overlayfs_inode(dentry, file);
        set_overlayfs_nlink(dentry, file);
    }

    file->path_key.path_id = get_path_id(file->path_key.ino, file->path_key.mount_id, file->metadata.nlink, invalidate_type);
}

#endif
