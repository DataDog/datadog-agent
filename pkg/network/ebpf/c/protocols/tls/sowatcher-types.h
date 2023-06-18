#ifndef __SOWATCHER_TYPES_H
#define __SOWATCHER_TYPES_H

#include "ktypes.h"

#define SO_SUFFIX_SIZE 3
#define LIB_PATH_MAX_SIZE 120

typedef struct {
    __u32 pid;
    __u32 len;
    char buf[LIB_PATH_MAX_SIZE];
} lib_path_t;

typedef struct {
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;
    long syscall_nr;
} trace_common;

typedef struct {
    trace_common unsued;
    int dfd;
    char* filename;
} enter_sys_openat_ctx;

typedef struct {
    trace_common unsued;
    long ret;
} exit_sys_openat_ctx;

#endif
