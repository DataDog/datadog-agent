#ifndef _STRUCTS_SECURITY_PROFILE_H_
#define _STRUCTS_SECURITY_PROFILE_H_

#include "constants/custom.h"

struct security_profile_t {
    u64 cookie;
    u32 state;
};

struct security_profile_syscalls_t {
    char syscalls[SYSCALL_ENCODING_TABLE_SIZE];
};

#endif
