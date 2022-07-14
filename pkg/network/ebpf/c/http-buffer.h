#ifndef __HTTP_BUFFER_H
#define __HTTP_BUFFER_H

#include "http-types.h"

// read_into_buffer copies data from an arbitrary memory address into a (statically sized) HTTP buffer.
// Ideally we would only copy min(data_size, HTTP_BUFFER_SIZE) bytes, but the code below is the only way
// we found to handle data sizes smaller than HTTP_BUFFER_SIZE in Kernel 4.4.
// In a nutshell, we read HTTP_BUFFER_SIZE bytes no matter what and then get rid of garbage data.
// Please note that even though the memset could be removed with no semantic change to the code,
// it is still necessary to make the eBPF verifier happy.
static __always_inline void read_into_buffer(char *buffer, char *data, size_t data_size) {
    __builtin_memset(buffer, 0, HTTP_BUFFER_SIZE);
    if (bpf_probe_read_user(buffer, HTTP_BUFFER_SIZE, data) < 0) {
// note: arm64 bpf_probe_read_user() could page fault if the HTTP_BUFFER_SIZE overlap a page
#if defined(__aarch64__)
#pragma unroll
        for (int i = 0; i < HTTP_BUFFER_SIZE; i++) {
            bpf_probe_read_user(&buffer[i], 1, &data[i]);
            if (buffer[i] == 0) {
                return;
            }
        }
#endif
    }

    if (data_size >= HTTP_BUFFER_SIZE) {
        return;
    }

#define BLOCK_SIZE (8)

    u32 offset = HTTP_BUFFER_SIZE;
    buffer += (HTTP_BUFFER_SIZE - BLOCK_SIZE);

#pragma unroll
    for (int i = 0; i < HTTP_BUFFER_SIZE / BLOCK_SIZE; i++) {
        if (data_size > (offset - BLOCK_SIZE)) break;
        *(u64 *)buffer = 0;
        buffer -= BLOCK_SIZE;
        offset -= BLOCK_SIZE;
    }

    if (data_size <= (offset - 7)) {
        buffer[1] = 0;
    }
    if (data_size <= (offset - 6)) {
        buffer[2] = 0;
    }
    if (data_size <= (offset - 5)) {
        buffer[3] = 0;
    }
    if (data_size <= (offset - 4)) {
        buffer[4] = 0;
    }
    if (data_size <= (offset - 3)) {
        buffer[5] = 0;
    }
    if (data_size <= (offset - 2)) {
        buffer[6] = 0;
    }
    if (data_size <= (offset - 1)) {
        buffer[7] = 0;
    }
#undef BLOCK_SIZE
}

#endif
