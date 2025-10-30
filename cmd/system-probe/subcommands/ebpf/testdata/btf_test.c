// Test eBPF program for BTF dumping tests
// This program creates various maps with BTF type information

#include "ktypes.h"
#include "bpf_metadata.h"
#include "bpf_helpers.h"

// Simple integer types map
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, u32);
    __type(value, u64);
    __uint(max_entries, 10);
} int_map SEC(".maps");

// Struct types map
struct conn_key {
    u64 netns;
    u16 port;
    u16 pad;
};

struct conn_stats {
    u64 packets;
    u64 bytes;
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, struct conn_key);
    __type(value, struct conn_stats);
    __uint(max_entries, 10);
} struct_map SEC(".maps");

// Array map with integers
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __type(key, u32);
    __type(value, u64);
    __uint(max_entries, 5);
} array_map SEC(".maps");

// PerCPU hash map with integers
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_HASH);
    __type(key, u32);
    __type(value, u64);
    __uint(max_entries, 10);
} percpu_hash_map SEC(".maps");

// PerCPU array map with integers
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __type(key, u32);
    __type(value, u64);
    __uint(max_entries, 5);
} percpu_array_map SEC(".maps");

// Enum type map
enum connection_state {
    STATE_INIT = 0,
    STATE_CONNECTED = 1,
    STATE_CLOSED = 2,
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, u32);
    __type(value, enum connection_state);
    __uint(max_entries, 10);
} enum_map SEC(".maps");

char _license[] SEC("license") = "GPL";
