#ifndef _CONSTANTS_OFFSETS_MODULE_H_
#define _CONSTANTS_OFFSETS_MODULE_H_

#include "constants/macros.h"

void __attribute__((always_inline)) read_module_name(void *dst, u64 size, struct module *mod) {
    u64 module_name_offset;
    LOAD_CONSTANT("module_name_offset", module_name_offset);
    bpf_probe_read_str(dst, size, (void *)mod + module_name_offset);
}

#endif
