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
READ_INTO_USER_BUFFER(classification, CLASSIFICATION_MAX_BUFFER)

READ_INTO_BUFFER(skb, HTTP_BUFFER_SIZE, BLK_SIZE)

#endif
