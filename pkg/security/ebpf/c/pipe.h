#ifndef _PIPE_H_
#define _PIPE_H_

struct bpf_map_def SEC("maps/pipefs_mountid") pipefs_mountid = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

#define DECLARE_EQUAL_TO_SUFFIXED(suffix, s) static inline int equal_to_##suffix(char *str) { \
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
    if (val) return *val;
    return 0;
}

int __attribute__((always_inline)) is_pipefs_mount_id(u32 id) {
    u32 pipefs_id = get_pipefs_mount_id();
    if (!pipefs_id)
        return 0;
    return (pipefs_id == id);
}

/* hook here to grab and cache the pipefs mount id */
SEC("kprobe/alloc_file_pseudo")
int kprobe_alloc_file_pseudo(struct pt_regs* ctx) {
    struct vfsmount* vfsm = (struct vfsmount*)PT_REGS_PARM2(ctx);

    // check if we already have the pipefs mount id
    if (get_pipefs_mount_id())
        return 0;
    
    struct super_block* sb;
    bpf_probe_read(&sb, sizeof(sb), &vfsm->mnt_sb);

    u64 sb_type_offset;
    LOAD_CONSTANT("sb_type_offset", sb_type_offset);
    struct file_system_type* fst;
    bpf_probe_read(&fst, sizeof(fst), (char *)sb + sb_type_offset);

    u64 file_system_type_name_offset;
    LOAD_CONSTANT("file_system_type_name_offset", file_system_type_name_offset);
    char* name;
    bpf_probe_read(&name, sizeof(name), (char *)fst + file_system_type_name_offset);

    if (IS_EQUAL_TO(name, pipefs)) {
        u32 mount_id = get_vfsmount_mount_id(vfsm);
        u32 key = 0;
        bpf_map_update_elem(&pipefs_mountid, &key, &mount_id, BPF_ANY);
    }
    
    return 0;
}

#endif /* _PIPE_H_ */
