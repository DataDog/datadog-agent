#ifndef __HTTP_BUFFER_H
#define __HTTP_BUFFER_H

#include "ktypes.h"
#if defined(COMPILE_PREBUILT) || defined(COMPILE_RUNTIME)
#include <linux/err.h>
#endif

#include "bpf_builtins.h"
#include "bpf_telemetry.h"

#include "protocols/http/types.h"
#include "protocols/read_into_buffer.h"

// This function reads a constant number of bytes into the fragment buffer of the http
// transaction object, and returns the number of bytes of the valid data. The number of
// bytes are used in userspace to zero out the garbage we may have read into the buffer.
//
// This function is used for the uprobe-based HTTPS monitoring (eg. OpenSSL, GnuTLS etc)
static __always_inline bool read_into_buffer(char *buffer, char *data, size_t data_size) {
    bpf_memset(buffer, 0, HTTP_BUFFER_SIZE);

    const size_t final_size = data_size < HTTP_BUFFER_SIZE ? data_size : HTTP_BUFFER_SIZE;
    if (final_size <= 0) {
        return false;
    }
    // Tricking the verifier
    const size_t final_size2 = final_size % (HTTP_BUFFER_SIZE + 1);
    bool ret = bpf_probe_read_user_with_telemetry(buffer, final_size2, data) >= 0;
    // In case of a success and we read more than HTTP_BUFFER_SIZE, zero the last byte.
    if (ret && final_size == HTTP_BUFFER_SIZE) {
        buffer[HTTP_BUFFER_SIZE-1] = 0;
    }
    return ret;
}

READ_INTO_BUFFER(skb, HTTP_BUFFER_SIZE, BLK_SIZE)

#endif
