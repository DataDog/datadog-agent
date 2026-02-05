#ifndef __REDIS_HELPERS_H
#define __REDIS_HELPERS_H

#include "protocols/classification/common.h"
#include "protocols/redis/defs.h"

// Checks the buffer represent a standard response (OK) or any of redis commands
// https://redis.io/commands/
static __always_inline __maybe_unused bool check_supported_ascii_and_crlf(const char* buf, __u32 buf_size, int index_to_start_from) {
    bool found_cr = false;
    char current_char;
    int i = index_to_start_from;
#pragma unroll(CLASSIFICATION_MAX_BUFFER)
    for (; i < CLASSIFICATION_MAX_BUFFER; i++) {
        current_char = buf[i];
        if (current_char == '\r') {
            found_cr = true;
            break;
        } else if ('A' <= current_char && current_char <= 'Z') {
            continue;
        } else if ('a' <= current_char && current_char <= 'z') {
            continue;
        } else if (current_char == '.' || current_char == ' ' || current_char == '-' || current_char == '_') {
            continue;
        }
        return false;
    }

    return found_cr && i + 1 < buf_size && i + 1 < CLASSIFICATION_MAX_BUFFER && buf[i + 1] == '\n';
}

static __always_inline __maybe_unused void convert_method_to_upper_case(char* method) {
    #pragma unroll (MAX_METHOD_LEN)
    for (int i = 0; i < MAX_METHOD_LEN; i++) {
        if ('a' <= method[i] && method[i] <= 'z') {
            method[i] = method[i] - 'a' + 'A';
        }
    }
}

// Checks the buffer represents an error according to https://redis.io/docs/reference/protocol-spec/#resp-errors
static __always_inline __maybe_unused bool check_err_prefix(const char* buf, __u32 buf_size) {
#define ERR "-ERR "
#define WRONGTYPE "-WRONGTYPE "

    // memcmp returns
    // 0 when s1 == s2,
    // !0 when s1 != s2.
    bool match = !(bpf_memcmp(buf, ERR, sizeof(ERR)-1)
        && bpf_memcmp(buf, WRONGTYPE, sizeof(WRONGTYPE)-1));

    return match;
}

static __always_inline __maybe_unused bool check_integer_and_crlf(const char* buf, __u32 buf_size, int index_to_start_from) {
    bool found_cr = false;
    char current_char;
    int i = index_to_start_from;
#pragma unroll(CLASSIFICATION_MAX_BUFFER)
    for (; i < CLASSIFICATION_MAX_BUFFER; i++) {
        current_char = buf[i];
        if (current_char == '\r') {
            found_cr = true;
            break;
        } else if ('0' <= current_char && current_char <= '9') {
            continue;
        }

        return false;
    }

    return found_cr && i + 1 < buf_size && i + 1 < CLASSIFICATION_MAX_BUFFER && buf[i + 1] == '\n';
}

static __always_inline bool is_redis(const char* buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, REDIS_MIN_FRAME_LENGTH);

    char first_char = buf[0];
    switch (first_char) {
    // RESP2 types
    case '+':  // Simple String
    case '-':  // Error
    case ':':  // Integer
    case '$':  // Bulk String
    case '*':  // Array
    // RESP3 types (Redis 6.0+)
    case '_':  // Null
    case '#':  // Boolean
    case ',':  // Double
    case '(':  // Big Number
    case '!':  // Bulk Error
    case '=':  // Verbatim String
    case '%':  // Map
    case '~':  // Set
    case '>':  // Push
        return true;
    default:
        return false;
    }
}

#endif
