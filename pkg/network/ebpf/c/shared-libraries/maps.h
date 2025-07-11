#ifndef __SHARED_LIBRARIES_MAPS_H
#define __SHARED_LIBRARIES_MAPS_H

#include "shared-libraries/types.h"
#include "map-defs.h"

// This map is used with 3 different probes, each can be called up to 1024 times each.
// Thus we need to have a map that can store 1024*3 entries. I'm using a larger map to be safe.
BPF_HASH_MAP(open_at_args, __u64, lib_path_t, 10240)

/*
 * These maps are used for notifying userspace of a shared library being loaded
 * There is one for each library set, so that userspace isn't overwhelmed with
 * events for libraries it doesn't care about.
 */
BPF_PERF_EVENT_ARRAY_MAP(crypto_shared_libraries, __u32)
BPF_PERF_EVENT_ARRAY_MAP(gpu_shared_libraries, __u32)
BPF_PERF_EVENT_ARRAY_MAP(libc_shared_libraries, __u32)

#endif
