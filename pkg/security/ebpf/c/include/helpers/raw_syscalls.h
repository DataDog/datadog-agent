#ifndef _HELPERS_RAW_SYSCALLS_H_
#define _HELPERS_RAW_SYSCALLS_H_

#include "maps.h"

__attribute__((always_inline)) u8 is_syscall(struct syscall_table_key_t *key) {
    u8 *ok = bpf_map_lookup_elem(&syscall_table, key);
    return (u8)(ok != NULL);
}

#endif
