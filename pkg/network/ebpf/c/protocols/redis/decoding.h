#ifndef __REDIS_DECODING_H
#define __REDIS_DECODING_H

#include "protocols/redis/decoding-maps.h"

#define REDIS_CMD_GET "GET"
#define REDIS_CMD_SET "SET"
#define MAX_KEY_LEN 128
#define RESP_ARRAY_PREFIX '*'
#define RESP_BULK_PREFIX '$'
#define RESP_TERMINATOR_1 '\r'
#define RESP_TERMINATOR_2 '\n'
#define METHOD_LEN 3
#define RESP_FIELD_TERMINATOR_LEN 2

// Read a CRLF terminator from the packet buffer. The terminator is expected to be in the format: \r\n.
// The function returns true if the terminator was successfully read, or false if the terminator could not be read.
static __always_inline bool read_crlf(pktbuf_t pkt) {
    char terminator[RESP_FIELD_TERMINATOR_LEN];
    if (pktbuf_load_bytes_from_current_offset(pkt, terminator, RESP_FIELD_TERMINATOR_LEN) < 0) {
        return false;
    }
    pktbuf_advance(pkt, RESP_FIELD_TERMINATOR_LEN);
    return terminator[0] == RESP_TERMINATOR_1 && terminator[1] == RESP_TERMINATOR_2;
}


// Read an array message from the packet buffer. The array message is expected to be in the format:
// *<param_count>\r\n<param1>\r\n<param2>\r\n...
// where <param_count> is the number of parameters in the array, and <param1>, <param2>, etc. are the parameters themselves.
// The function returns the number of parameters in the array, or 0 if the array message could not be read.
static __always_inline u32 read_array_message(pktbuf_t pkt) {
    // Verify RESP array prefix
    char first_byte;
    if (pktbuf_load_bytes_from_current_offset(pkt, &first_byte, sizeof(first_byte)) < 0 || first_byte != RESP_ARRAY_PREFIX) {
        return 0;
    }
    pktbuf_advance(pkt, sizeof(first_byte));

    // Read parameter count
    char param_count;
    // Assuming single-digit param count, as currently we don't need more.
    if (pktbuf_load_bytes_from_current_offset(pkt, &param_count, 1) < 0) {
        return 0;
    }
    pktbuf_advance(pkt, sizeof(param_count));

    if (param_count < '0' || param_count > '9') {
        return 0;
    }

    if (!read_crlf(pkt)) {
        return 0;
    }

    return param_count - '0';
}

// Read a bulk string from the packet buffer. The bulk string is expected to be in the format:
// $<key_len>\r\n<key>\r\n
// where <key_len> is the length of the key in bytes, and <key> is the key itself.
// The key is stored in the provided buffer, and the function returns true if the key was successfully read.
// The function returns false if the key could not be read, or if the key length is invalid.
// The function also returns false if the key length is greater than the provided buffer length.
static __always_inline bool read_bulk_string(pktbuf_t pkt, char *buf, u32 buf_len) {
    char bulk_prefix;
    if (pktbuf_load_bytes_from_current_offset(pkt, &bulk_prefix, sizeof(bulk_prefix)) < 0 || bulk_prefix != RESP_BULK_PREFIX) {
        return false;
    }
    pktbuf_advance(pkt, sizeof(bulk_prefix));

    // Read key length (up to 3 digits)
    s32 key_size = 0;
    char key_len;
    #pragma unroll (3)
    for (int i = 0; i < 3; i++) {
        if (pktbuf_load_bytes_from_current_offset(pkt, &key_len, sizeof(key_len)) < 0 || key_len == RESP_TERMINATOR_1) {
            break;
        }
        if (key_len < '0' || key_len > '9') {
            return false;
        }
        key_size = key_size * 10 + (key_len - '0');
        pktbuf_advance(pkt, sizeof(key_len));
    }
    // Ensure key_size is always positive and within a valid range
    // We support up to 999 characters in the key length, hence the mask is 2^10 - 1 = 1023 = 0x3FF.
    key_size &= 0x3FF;

    if (!read_crlf(pkt)) {
        return false;
    }

    if (key_size > MAX_KEY_LEN - 1) {
        key_size = MAX_KEY_LEN - 1;
    }

    if (key_size > buf_len) {
        key_size = buf_len;
    }

    if (key_size > 0) {
        long ret = pktbuf_load_bytes_from_current_offset(pkt, buf, key_size);
        if (ret < 0) {
            return false;
        }
        pktbuf_advance(pkt, key_size);
    } else {
        return false;
    }
    return read_crlf(pkt);
}

// Process a Redis request from the packet buffer. The function reads the request from the packet buffer,
// and returns the method (GET or SET) and the key(up to MAX_KEY_LEN bytes).
static __always_inline void process_redis_request(pktbuf_t pkt) {
    u32 param_count = read_array_message(pkt);
    if (param_count == 0) {
        return;
    }
    // GET message has 2 parameters, SET message has 3-5 parameters. Anything else is irrelevant for us.
    if (param_count < 2 || param_count > 5) {
        return;
    }

    char method[METHOD_LEN + 1] = {};
    if (!read_bulk_string(pkt, method, METHOD_LEN)) {
        return;
    }

    if (bpf_memcmp(method, REDIS_CMD_GET, METHOD_LEN) != 0 &&
        bpf_memcmp(method, REDIS_CMD_SET, METHOD_LEN) != 0) {
        return;
    }

    char key[MAX_KEY_LEN] = {};
    if (!read_bulk_string(pkt, key, MAX_KEY_LEN)) {
        return;
    }

    log_debug("Redis command: %s, Key: %s", method, key);
}

SEC("socket/redis_process")
int socket__redis_process(struct __sk_buff *skb) {
    dispatcher_arguments_t dispatcher_args_copy;
    bpf_memset(&dispatcher_args_copy, 0, sizeof(dispatcher_arguments_t));
    if (!fetch_dispatching_arguments(&dispatcher_args_copy.tup, &dispatcher_args_copy.skb_info)) {
        log_debug("redis_process failed to fetch arguments for tail call");
        return 0;
    }

    pktbuf_t pkt = pktbuf_from_skb(skb, &dispatcher_args_copy.skb_info);
    process_redis_request(pkt);

    return 0;
}

SEC("uprobe/redis_tls_process")
int uprobe__redis_tls_process(struct pt_regs *ctx) {
    return 0;
}

SEC("uprobe/redis_tls_termination")
int uprobe__redis_tls_termination(struct pt_regs *ctx) {
    return 0;
}

#endif /* __REDIS_DECODING_H */
