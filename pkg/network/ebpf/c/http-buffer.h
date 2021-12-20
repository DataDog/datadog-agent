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
    bpf_probe_read(buffer, HTTP_BUFFER_SIZE, data);
    if (data_size >= HTTP_BUFFER_SIZE) {
        return;
    }

    // clean up garbage
#pragma unroll
    for (int i = 0; i < HTTP_BUFFER_SIZE; i++) {
        if (i >= data_size) {
            buffer[i] = 0;
        }
    }
}

#endif
