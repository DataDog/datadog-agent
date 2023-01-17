#ifndef _PIPE_H_
#define _PIPE_H_

struct bpf_map_def SEC("maps/pipefs_mountid") pipefs_mountid = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
};

#define DECLARE_EQUAL_TO_SUFFIXED(suffix, s) static __attribute__((always_inline)) int equal_to_##suffix(char *str) { \
        char s1[sizeof(#s)];                                            \
        bpf_probe_read(&s1, sizeof(s1), str);                           \
        char s2[] = #s;                                                 \
        for (int i = 0; i < sizeof(s1); ++i)                            \
            if (s2[i] != s1[i])                                         \
                return 0;                                               \
        return 1;                                                       \
    }                                                                   \

#define DECLARE_EQUAL_TO(s)                     \
    DECLARE_EQUAL_TO_SUFFIXED(s, s)

#define IS_EQUAL_TO(str, s) equal_to_##s(str)

DECLARE_EQUAL_TO(pipefs);


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
