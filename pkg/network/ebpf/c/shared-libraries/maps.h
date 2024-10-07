#ifndef __SHARED_LIBRARIES_MAPS_H
#define __SHARED_LIBRARIES_MAPS_H

#include "shared-libraries/types.h"
#include "map-defs.h"

#define def_open_at_args_map(libset) BPF_LRU_MAP(open_at_args_##libset, __u64, lib_path_t, 1024)

/* This map used for notifying userspace of a shared library being loaded */
#define def_perf_event_map(libset) BPF_PERF_EVENT_ARRAY_MAP(shared_libraries_##libset, __u32)

#define def_libset_maps(libset)   \
    def_open_at_args_map(libset); \
    def_perf_event_map(libset);

#endif
