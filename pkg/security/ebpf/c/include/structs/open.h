#ifndef _STRUCTS_OPEN_H_
#define _STRUCTS_OPEN_H_

struct openat2_open_how {
    u64 flags;
    u64 mode;
    u64 resolve;
};

struct open_flags {
    int open_flag;
    umode_t mode;
};

struct io_open {
    struct file *file;
    int dfd;
    bool ignore_nonblock;
    struct filename *filename;
    struct openat2_open_how how;
};

#endif
