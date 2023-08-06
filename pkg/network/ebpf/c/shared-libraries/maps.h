#ifndef __SHARED_LIBRARIES_MAPS_H
#define __SHARED_LIBRARIES_MAPS_H

#include "shared-libraries/types.h"
#include "map-defs.h"

BPF_LRU_MAP(open_at_args, __u64, lib_path_t, 1024)

/* This map used for notifying userspace of a shared library being loaded */
BPF_PERF_EVENT_ARRAY_MAP(shared_libraries, __u32)

#endif
