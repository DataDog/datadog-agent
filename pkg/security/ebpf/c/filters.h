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

// implemented in the probe.c file
void __attribute__((always_inline)) remove_inode_discarders(struct file_t *file);

struct bpf_map_def SEC("maps/filter_policy") filter_policy = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct policy_t),
    .max_entries = EVENT_MAX,
    .pinning = 0,
    .namespace = "",
};

struct process_discarder_t {
    u64 event_type;
    u32 tgid;
    u32 padding;
};

struct inode_discarder_t {
    u64 event_type;
    struct path_key_t path_key;
};

struct bpf_map_def SEC("maps/inode_discarders") inode_discarders = { \
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct inode_discarder_t),
    .value_size = sizeof(struct filter_t),
    .max_entries = 512,
    .pinning = 0,
    .namespace = "",
};

int __attribute__((always_inline)) discarded_by_inode(u64 event_type, u32 mount_id, u64 inode) {
    struct inode_discarder_t key = {
        .event_type = event_type,
        .path_key = {
            .ino = inode,
            .mount_id = mount_id,
        }
    };

    struct filter_t *filter = bpf_map_lookup_elem(&inode_discarders, &key);
    if (filter) {
#ifdef DEBUG
        bpf_printk("file with inode %d discarded\n", inode);
#endif
        return 1;
    }
    return 0;
}

void __attribute__((always_inline)) remove_inode_discarder(u64 event_type, u32 mount_id, u64 inode) {
    struct inode_discarder_t key = {
        .event_type = event_type,
        .path_key = {
            .ino = inode,
            .mount_id = mount_id,
        }
    };

    bpf_map_delete_elem(&inode_discarders, &key);
}

#endif
