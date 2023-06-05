#ifndef _MAP_DEFS_H_
#define _MAP_DEFS_H_

#include "bpf_helpers.h"

#define BPF_MAP(_name, _type, _key_type, _value_type, _max_entries, _pin, _map_flags) \
    struct {                                                                         \
        __uint(type, _type);                                                         \
        __uint(max_entries, _max_entries);                                           \
        __uint(pinning, _pin);                                                       \
        __type(key, _key_type);                                                      \
        __type(value, _value_type);                                                  \
        __uint(map_flags, _map_flags);                                               \
    } _name SEC(".maps");

#define BPF_PERF_EVENT_ARRAY_MAP_PINNED(name, value_type, max_entries) \
    BPF_MAP(name, BPF_MAP_TYPE_PERF_EVENT_ARRAY, u32, value_type, max_entries, 1, 0)

#define BPF_PERF_EVENT_ARRAY_MAP(name, value_type) \
    BPF_MAP(name, BPF_MAP_TYPE_PERF_EVENT_ARRAY, u32, value_type, 0, 0, 0)

#define BPF_ARRAY_MAP(name, value_type, max_entries) \
    BPF_MAP(name, BPF_MAP_TYPE_ARRAY, u32, value_type, max_entries, 0, 0)

#define BPF_HASH_MAP_PINNED(name, key_type, value_type, max_entries) \
    BPF_MAP(name, BPF_MAP_TYPE_HASH, key_type, value_type, max_entries, 1, 0)

#define BPF_HASH_MAP(name, key_type, value_type, max_entries) \
    BPF_MAP(name, BPF_MAP_TYPE_HASH, key_type, value_type, max_entries, 0, 0)

#define BPF_PROG_ARRAY(name, max_entries) \
    BPF_MAP(name, BPF_MAP_TYPE_PROG_ARRAY, u32, u32, max_entries, 0, 0)

#define BPF_LRU_MAP(name, key_type, value_type, max_entries) \
    BPF_MAP(name, BPF_MAP_TYPE_LRU_HASH, key_type, value_type, max_entries, 0, 0)

#define BPF_LRU_MAP_PINNED(name, key_type, value_type, max_entries) \
    BPF_MAP(name, BPF_MAP_TYPE_LRU_HASH, key_type, value_type, max_entries, 1, 0)

#define BPF_LRU_MAP_FLAGS(name, key_type, value_type, max_entries, map_flags) \
    BPF_MAP(name, BPF_MAP_TYPE_LRU_HASH, key_type, value_type, max_entries, 0, map_flags)

#define BPF_PERCPU_HASH_MAP(name, key_type, value_type, max_entries) \
    BPF_MAP(name, BPF_MAP_TYPE_PERCPU_HASH, key_type, value_type, max_entries, 0, 0)

#define BPF_PERCPU_ARRAY_MAP(name, key_type, value_type, max_entries) \
    BPF_MAP(name, BPF_MAP_TYPE_PERCPU_ARRAY, key_type, value_type, max_entries, 0, 0)

#endif
