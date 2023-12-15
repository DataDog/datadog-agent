#ifndef _HELPERS_UTILS_H_
#define _HELPERS_UTILS_H_

#include "constants/custom.h"
#include "constants/macros.h"
#include "maps.h"

int __attribute__((always_inline)) ktime_get_sec() {
    return NS_TO_SEC(bpf_ktime_get_ns());
}

static __attribute__((always_inline)) u32 ord(u8 c) {
    if (c >= 49 && c <= 57) {
        return c - 48;
    }
    return 0;
}

static __attribute__((always_inline)) u32 atoi(char *buff) {
    u32 res = 0;
    u8 c = 0;
    char buffer[CHAR_TO_UINT32_BASE_10_MAX_LEN];

    int size = bpf_probe_read_str(&buffer, sizeof(buffer), buff);
    if (size <= 1) {
        return 0;
    }

#pragma unroll
    for (int i = 0; i < CHAR_TO_UINT32_BASE_10_MAX_LEN; i++)
    {
        bpf_probe_read(&c, sizeof(c), buffer + i);
        if (c == 0 || c == '\n') {
            break;
        }
        res = res * 10 + ord(c);
    }

    return res;
}

static __attribute__((always_inline)) int _isxdigit(unsigned char c) {
    return ((c >= '0' && c <= '9') ||
            (c >= 'a' && c <= 'f') ||
            (c >= 'A' && c <= 'F'));
}

int __attribute__((always_inline)) parse_buf_to_bool(const char *buf) {
    u32 key = 0;
    struct selinux_write_buffer_t *copy = bpf_map_lookup_elem(&selinux_write_buffer, &key);
    if (!copy) {
        return -1;
    }
    int read_status = bpf_probe_read_str(&copy->buffer, SELINUX_WRITE_BUFFER_LEN, (void *)buf);
    if (!read_status) {
        return -1;
    }

#pragma unroll
    for (size_t i = 0; i < SELINUX_WRITE_BUFFER_LEN; i++) {
        char curr = copy->buffer[i];
        if (curr == 0) {
            return 0;
        }
        if ('0' < curr && curr <= '9') {
            return 1;
        }
        if (curr != '0') {
            return 0;
        }
    }

    return 0;
}

u32 __attribute__((always_inline)) rand32() {
    return bpf_get_prandom_u32();
}

u64 __attribute__((always_inline)) rand64() {
    return (u64)rand32() << 32 | bpf_ktime_get_ns();
}

#endif
