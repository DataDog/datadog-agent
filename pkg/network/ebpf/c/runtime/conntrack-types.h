#ifndef __CONNTRACK_TYPES_H
#define __CONNTRACK_TYPES_H

#include <linux/types.h>

typedef struct {
    __u64 registers;
    __u64 unregisters;
} conntrack_telemetry_t;

enum conntrack_telemetry_counter {
    registers,
    unregisters,
};

#endif
