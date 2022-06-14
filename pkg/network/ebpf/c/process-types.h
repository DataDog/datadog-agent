#ifndef __PROCESS_TYPES_H__
#define __PROCESS_TYPES_H__

#include <linux/types.h>

#include "container.h"

typedef enum
{
    EVENT_ANY = 0,
    EVENT_FORK,
    EVENT_EXEC,
    EVENT_EXIT,

    EVENT_ALL = 0xffffffff // used as a mask for all the events
} event_type;

typedef struct {
    __u64 cpu;
    __u64 timestamp;
    __u32 type;
    __u8 async;
    __u8 padding[3];
} kevent_t;

typedef struct {
    __u32 pid;
    __u32 tid;
    __u32 padding;
} process_context_t;

typedef struct {
    char container_id[CONTAINER_ID_LEN];
} container_context_t;

typedef struct {
    container_context_t container;
    __u64 exec_timestamp;
} proc_cache_t;

typedef struct {
    __u32 cookie;
    __u32 ppid;
    __u64 fork_timestamp;
    __u64 exit_timestamp;
} pid_cache_t;

typedef struct {
    kevent_t event;
    process_context_t process;
    proc_cache_t proc_entry;
    pid_cache_t pid_entry;
} exec_event_t;

typedef struct {
    kevent_t event;
    process_context_t process;
    container_context_t container;
} exit_event_t;

#endif // __PROCESS_TYPES_H__
