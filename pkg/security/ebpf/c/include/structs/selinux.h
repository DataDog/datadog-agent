#ifndef _STRUCTS_SELINUX_H_
#define _STRUCTS_SELINUX_H_

#include "constants/custom.h"

struct selinux_write_buffer_t {
    char buffer[SELINUX_WRITE_BUFFER_LEN];
};

#endif
