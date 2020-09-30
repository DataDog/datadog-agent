#ifndef _FILTERS_H
#define _FILTERS_H

enum policy_mode
{
    ACCEPT = 1,
    DENY = 2,
    NO_FILTER = 3,
};

enum policy_flags
{
    BASENAME = 1,
    FLAGS = 2,
    MODE = 4,
    PARENT_NAME = 8,
};

struct policy_t {
    char mode;
    char flags;
};

struct filter_t {
    char value;
};

void __attribute__((always_inline)) remove_inode_discarders(struct file_t *file);
void __attribute__((always_inline)) remove_pid_discarders(u32 tgid);

#define POLICY_MAP_PTR(name) &name##_policy

#define POLICY_MAP(name) struct bpf_map_def SEC("maps/"#name"_policy") name##_policy = { \
    .type = BPF_MAP_TYPE_ARRAY, \
    .key_size = sizeof(u32), \
    .value_size = sizeof(struct policy_t), \
    .max_entries = 1, \
    .pinning = 0, \
    .namespace = "", \
}

#define PROCESS_DISCARDERS_MAP_PTR(name) &name##_process_discarders

#define PROCESS_DISCARDERS_MAP(name, size) struct bpf_map_def SEC("maps/"#name"_process_discarders") name##_process_discarders = { \
    .type = BPF_MAP_TYPE_LRU_HASH, \
    .key_size = sizeof(u32), \
    .value_size = sizeof(struct filter_t), \
    .max_entries = size, \
    .pinning = 0, \
    .namespace = "", \
}

int __attribute__((always_inline)) discard_by_pid(struct bpf_map_def *discarders_map) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct filter_t *filter = bpf_map_lookup_elem(discarders_map, &tgid);
    if (filter) {
#ifdef DEBUG
        bpf_printk("process with pid %d discarded\n", tgid);
#endif
        return 1;
    }
    return 0;
}

#define INODE_DISCARDERS_MAP_PTR(name) &name##_inode_discarders

#define INODE_DISCARDERS_MAP(name, size) struct bpf_map_def SEC("maps/"#name"_inode_discarders") name##_inode_discarders = { \
    .type = BPF_MAP_TYPE_LRU_HASH, \
    .key_size = sizeof(struct path_key_t), \
    .value_size = sizeof(struct filter_t), \
    .max_entries = size, \
    .pinning = 0, \
    .namespace = "", \
}

#endif
