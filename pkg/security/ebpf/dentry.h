#ifndef _DENTRY_H_
#define _DENTRY_H_

#include <linux/dcache.h>
#include <linux/types.h>

#define DENTRY_MAX_DEPTH 16

struct path_leaf_t {
  long parent;
  char name[64];
};

struct bpf_map_def SEC("maps/pathnames") pathnames = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(long),
    .value_size = sizeof(struct path_leaf_t),
    .max_entries = 32000,
    .pinning = 0,
    .namespace = "",
};

long __attribute__((always_inline)) get_inode(struct inode *inode) {
    long ino;
    bpf_probe_read(&ino, sizeof(inode), &inode->i_ino);
    return ino;
}

unsigned long __attribute__((always_inline)) get_inode_ino(struct inode *inode) {
    unsigned long ino;
    bpf_probe_read(&ino, sizeof(inode), &inode->i_ino);
    return ino;
}

int __attribute__((always_inline)) get_inode_mount_id(struct inode *dir) {
    // Mount ID
    int mount_id;
    struct super_block *spb;
    bpf_probe_read(&spb, sizeof(spb), &dir->i_sb);

    struct list_head s_mounts;
    bpf_probe_read(&s_mounts, sizeof(s_mounts), &spb->s_mounts);

    bpf_probe_read(&mount_id, sizeof(int), (void *) s_mounts.next + 172);
    // bpf_probe_read(&mount_id, sizeof(int), &((struct mount *) s_mounts.next)->mnt_id);

    return mount_id;
}

struct inode * __attribute__((always_inline)) get_dentry_inode(struct dentry *dentry) {
    struct inode *d_inode;
    bpf_probe_read(&d_inode, sizeof(d_inode), &dentry->d_inode);
    return d_inode;
}

unsigned long __attribute__((always_inline)) get_dentry_ino(struct dentry *dentry) {
    struct inode *d_inode;
    bpf_probe_read(&d_inode, sizeof(d_inode), &dentry->d_inode);
    return get_inode_ino(d_inode);
}

struct inode* __attribute__((always_inline)) get_file_inode(struct file *file) {
    struct inode *f_inode;
    bpf_probe_read(&f_inode, sizeof(f_inode), &file->f_inode);
    return f_inode;
}

struct dentry* __attribute__((always_inline)) get_file_dentry(struct file *file) {
    struct dentry *f_dentry;
    bpf_probe_read(&f_dentry, sizeof(f_dentry), &file->f_path.dentry);
    return f_dentry;
}

static __attribute__((always_inline)) int resolve_dentry(struct dentry *dentry, long inode) {
    struct path_leaf_t map_value = {};
    long next_inode = inode;
    struct qstr qstr;

#pragma unroll
    for (int i = 0; i < DENTRY_MAX_DEPTH; i++)
    {
        bpf_probe_read(&qstr, sizeof(qstr), &dentry->d_name);

        struct dentry *d_parent;
        bpf_probe_read(&d_parent, sizeof(d_parent), &dentry->d_parent);

        inode = next_inode;
        if (dentry == d_parent) {
            next_inode = 0;
        } else {
            next_inode = get_inode_ino(get_dentry_inode(d_parent));
        }

        bpf_probe_read_str(&map_value.name, sizeof(map_value.name), (void*) qstr.name);
        if (map_value.name[0] == 47 || map_value.name[0] == 0)
            next_inode = 0;

        map_value.parent = next_inode;

        if (bpf_map_lookup_elem(&pathnames, &inode) == NULL) {
            bpf_map_update_elem(&pathnames, &inode, &map_value, BPF_ANY);
        }

        dentry = d_parent;
        if (next_inode == 0)
            return i + 1;
    }

    // If the last next_id isn't null, this means that there are still other parents to fetch.
    // TODO: use BPF_PROG_ARRAY to recursively fetch 32 more times. For now, add a fake parent to notify
    // that we couldn't fetch everything.

    if (next_inode != 0) {
        map_value.name[0] = map_value.name[0];
        map_value.parent = 0;
        bpf_map_update_elem(&pathnames, &next_inode, &map_value, BPF_ANY);
    }

    return DENTRY_MAX_DEPTH;
}

#endif