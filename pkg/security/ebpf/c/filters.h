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

#define POLICY_MAP_PTR(name) &name##_policy

#define POLICY_MAP(name) struct bpf_map_def SEC("maps/"#name"_policy") name##_policy = { \
    .type = BPF_MAP_TYPE_ARRAY, \
    .key_size = sizeof(u32), \
    .value_size = sizeof(struct policy_t), \
    .max_entries = 1, \
    .pinning = 0, \
    .namespace = "", \
}

#endif
