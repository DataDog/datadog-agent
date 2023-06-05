#ifndef _HOOKS_PIPE_H_
#define _HOOKS_PIPE_H_

#include "constants/macros.h"

#include "helpers/filesystem.h"

DECLARE_EQUAL_TO(pipefs);

/* hook here to grab and cache the pipefs mount id */
SEC("kprobe/mntget")
int kprobe_mntget(struct pt_regs* ctx) {
    struct vfsmount* vfsm = (struct vfsmount*)PT_REGS_PARM1(ctx);

    // check if we already have the pipefs mount id
    if (get_pipefs_mount_id()) {
        return 0;
    }

    struct super_block* sb;
    bpf_probe_read(&sb, sizeof(sb), &vfsm->mnt_sb);

    struct file_system_type* fst = get_super_block_fs(sb);

    char* name;
    bpf_probe_read(&name, sizeof(name), &fst->name);

    if (IS_EQUAL_TO(name, pipefs)) {
        u32 mount_id = get_vfsmount_mount_id(vfsm);
        u32 key = 0;
        bpf_map_update_elem(&pipefs_mountid, &key, &mount_id, BPF_ANY);
    }
    return 0;
}

#endif /* _PIPE_H_ */
