#ifndef DI_MAPS_H
#define DI_MAPS_H

#include "map-defs.h"

// The events map is the ringbuffer used for communicating events from
// bpf to user space.
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 24);
} events SEC(".maps");


// The param_stack map is used as a stack for the location expressions
// to operate on values and addresses.
struct inner_param_stack {
    __uint(type, BPF_MAP_TYPE_STACK);
    __uint(max_entries, 2048);
    __uint(value_size, sizeof(__u64));
};

// The param_stacks map is to set up a unique stack for each CPU.
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY_OF_MAPS);
    __uint(max_entries, 0);
    __uint(key_size, sizeof(__u32));
    __array(values, struct inner_param_stack);
} param_stacks SEC(".maps");

// The zeroval map is used to have pre-zero'd data which bpf code can
// use to zero out event buffers (similar to memset, but verifier friendly).
BPF_ARRAY_MAP(zeroval, char[PARAM_BUFFER_SIZE], 1);

// The temp_storage_array map is used as a temporary location in memory
// not on the bpf stack that location expressions can use for temporarily
// caching data while they operate on it without worrying about exceeding
// the 512 byte bpf stack limit.
BPF_PERCPU_ARRAY_MAP(temp_storage_array, __u64[4000], 1);

// The collection_limits map is used for setting the known length limit
// of collections such as slices so that they can later be referenced
// when reading the values in the collection.
BPF_HASH_MAP(collection_limits, char[6], __u16, 1024);
#endif
