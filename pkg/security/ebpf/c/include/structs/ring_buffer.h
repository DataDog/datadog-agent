#ifndef _STRUCTS_RING_BUFFER_H_
#define _STRUCTS_RING_BUFFER_H_

#define RING_BUFFER_SIZE 524288

struct ring_buffer_t {
    char buffer[RING_BUFFER_SIZE];
};

// struct stored in per-cpu map
struct ring_buffer_ctx {
    u64 watermark;
    u32 write_cursor;
    u32 read_cursor;
    u32 len;
    u32 cpu;
};

// struct used by events structs
struct ring_buffer_ref_t {
    u64 watermark;
    u32 read_cursor;
    u32 len;
    u32 cpu;
    u32 padding;
};

#endif