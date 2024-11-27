#ifndef __SHARED_LIBRARIES_MAPS_H
#define __SHARED_LIBRARIES_MAPS_H

#include "shared-libraries/types.h"
#include "map-defs.h"

BPF_LRU_MAP(open_at_args, __u64, lib_path_t, 1024)

/*
 * These maps are used for notifying userspace of a shared library being loaded
 * There is one for each library set, so that userspace isn't overwhelmed with
 * events for libraries it doesn't care about.
 */
BPF_PERF_EVENT_ARRAY_MAP(crypto_shared_libraries, __u32)
BPF_PERF_EVENT_ARRAY_MAP(gpu_shared_libraries, __u32)

#endif
