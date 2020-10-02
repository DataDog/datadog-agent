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
    PROCESS_INODE = 16,
};

struct policy_t {
    char mode;
    char flags;
};

struct filter_t {
    char value;
};

void __attribute__((always_inline)) remove_inode_discarders(struct file_t *file);

#endif
