#ifndef DI_MAPS_H
#define DI_MAPS_H

// The events map is the ringbuffer used for communicating events from
// bpf to user space.
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 24);
} events SEC(".maps");

// The zeroval map is used to have pre-zero'd data which bpf code can
// use to zero out event buffers (similar to memset, but verifier friendly).
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(key_size, sizeof(__u32));
    __uint(value_size, sizeof(char[PARAM_BUFFER_SIZE]));
    __uint(max_entries, 1);
} zeroval SEC(".maps");

// The param_stack map is used as a stack for the location expressions
// to operate on values and addresses.
struct {
    __uint(type, BPF_MAP_TYPE_STACK);
    __uint(max_entries, 2048);
    __type(value, __u64);
} param_stack SEC(".maps");

// The temp_storage_array map is used as a temporary location in memory
// not on the bpf stack that location expressions can use for temporarily
// caching data while they operate on it without worrying about exceeding
// the 512 byte bpf stack limit.
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u64[4000]);
} temp_storage_array SEC(".maps");

// The collection_limits map is used for setting the known length limit
// of collections such as slices so that they can later be referenced
// when reading the values in the collection.
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, char[6]);
    __type(value, __u16);
} collection_limits SEC(".maps");

#endif
