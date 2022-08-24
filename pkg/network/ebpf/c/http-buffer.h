#ifndef __HTTP_BUFFER_H
#define __HTTP_BUFFER_H

#include "http-types.h"

// This function reads a constant number of bytes into the fragment buffer of the http
// transaction object, and returns the number of bytes of the valid data. The number of
// bytes are used in userspace to zero out the garbage we may have read into the buffer.
static __always_inline void read_into_buffer(char *buffer, char *data, size_t data_size) {
    __builtin_memset(buffer, 0, HTTP_BUFFER_SIZE);

    // we read HTTP_BUFFER_SIZE-1 bytes to ensure that the string is always null terminated
    if (bpf_probe_read_user(buffer, HTTP_BUFFER_SIZE - 1, data) < 0) {
// note: arm64 bpf_probe_read_user() could page fault if the HTTP_BUFFER_SIZE overlap a page
#if defined(__aarch64__)
#pragma unroll
        for (int i = 0; i < HTTP_BUFFER_SIZE - 1; i++) {
            bpf_probe_read_user(&buffer[i], 1, &data[i]);
            if (buffer[i] == 0) {
                return;
            }
        }
#endif
    }
}

#endif
