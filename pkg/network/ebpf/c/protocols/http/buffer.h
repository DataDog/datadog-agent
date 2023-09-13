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

#define PAGESIZE 4096

#define READ_INTO_USER_BUFFER(name, total_size)                                                                         \
    static __always_inline void read_into_user_buffer_##name(char *dst, char *src) {                                    \
        bpf_memset(dst, 0, total_size);                                                                                 \
        long ret = bpf_probe_read_user_with_telemetry(dst, total_size, src);                                            \
        if (ret >= 0) {                                                                                                 \
            return;                                                                                                     \
        }                                                                                                               \
        const __u64 read_size_until_end_of_page = PAGESIZE - ((__u64)src % PAGESIZE);                                   \
        const __u64 size_to_read = read_size_until_end_of_page < total_size ? read_size_until_end_of_page : total_size; \
        bpf_probe_read_user_with_telemetry(dst, size_to_read, src);                                                     \
        return;                                                                                                         \
    }                                                                                                                   \

READ_INTO_USER_BUFFER(http, HTTP_BUFFER_SIZE)

static __always_inline void read_into_buffer_classification(char *buffer, char *data, size_t data_size) {
    bpf_memset(buffer, 0, CLASSIFICATION_MAX_BUFFER);

    // we read CLASSIFICATION_MAX_BUFFER-1 bytes to ensure that the string is always null terminated
    if (bpf_probe_read_user_with_telemetry(buffer, CLASSIFICATION_MAX_BUFFER - 1, data) < 0) {
// note: arm64 bpf_probe_read_user() could page fault if the CLASSIFICATION_MAX_BUFFER overlap a page
#pragma unroll(CLASSIFICATION_MAX_BUFFER - 1)
        for (int i = 0; i < CLASSIFICATION_MAX_BUFFER - 1; i++) {
            bpf_probe_read_user(&buffer[i], 1, &data[i]);
            if (buffer[i] == 0) {
                return;
            }
        }
    }
}

READ_INTO_BUFFER(skb, HTTP_BUFFER_SIZE, BLK_SIZE)

#endif
