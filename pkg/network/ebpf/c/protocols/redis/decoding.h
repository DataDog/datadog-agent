#ifndef __REDIS_DECODING_H
#define __REDIS_DECODING_H

#include "protocols/redis/decoding-maps.h"

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
static __always_inline bool read_bulk_string(pktbuf_t pkt, char *buf, u32 buf_len, u16 *out_key_len, bool *truncated) {
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

    const s32 original_key_size = key_size;
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
        pktbuf_advance(pkt, original_key_size);
        *out_key_len = key_size;
        *truncated = key_size < original_key_size;
    } else {
        return false;
    }
    return read_crlf(pkt);
}

// Process a Redis request from the packet buffer. The function reads the request from the packet buffer,
// and returns the method (GET or SET) and the key(up to MAX_KEY_LEN bytes).
static __always_inline void process_redis_request(pktbuf_t pkt, conn_tuple_t *conn_tuple) {
    u32 param_count = read_array_message(pkt);
    if (param_count == 0) {
        return;
    }
    // GET message has 2 parameters, SET message has 3-5 parameters. Anything else is irrelevant for us.
    if (param_count < 2 || param_count > 5) {
        return;
    }

    char method[METHOD_LEN + 1] = {};
    __u16 key_len = 0;
    bool truncated = false;
    if (!read_bulk_string(pkt, method, METHOD_LEN, &key_len, &truncated)) {
        return;
    }

    redis_transaction_t transaction = {};
    transaction.request_started = bpf_ktime_get_ns();
    if (bpf_memcmp(method, REDIS_CMD_SET, METHOD_LEN) == 0) {
        transaction.command = REDIS_SET;
    } else if (bpf_memcmp(method, REDIS_CMD_GET, METHOD_LEN) == 0) {
        transaction.command = REDIS_GET;
    } else {
        return;
    }

    if (!read_bulk_string(pkt, transaction.buf, sizeof(transaction.buf), &transaction.buf_len, &transaction.truncated)) {
        return;
    }

    bpf_map_update_elem(&redis_in_flight, conn_tuple, &transaction, BPF_ANY);
}

// Handles a TCP termination event by deleting the connection tuple from the in-flight map.
static void __always_inline redis_tcp_termination(conn_tuple_t *tup) {
    bpf_map_delete_elem(&redis_in_flight, tup);
    flip_tuple(tup);
    bpf_map_delete_elem(&redis_in_flight, tup);
}

// Enqueues a batch of events to the user-space. To spare stack size, we take a scratch buffer from the map, copy
// the connection tuple and the transaction to it, and then enqueue the event.
static __always_inline void redis_batch_enqueue_wrapper(conn_tuple_t *tuple, redis_transaction_t *tx) {
    u32 zero = 0;
    redis_event_t *event = bpf_map_lookup_elem(&redis_scratch_buffer, &zero);
    if (!event) {
        return;
    }

    bpf_memcpy(&event->tuple, tuple, sizeof(conn_tuple_t));
    bpf_memcpy(&event->tx, tx, sizeof(redis_transaction_t));
    redis_batch_enqueue(event);
}

static void __always_inline process_redis_response(pktbuf_t pkt, conn_tuple_t *tup, redis_transaction_t *transaction) {
    char first_byte;
    if (pktbuf_load_bytes_from_current_offset(pkt, &first_byte, sizeof(first_byte)) < 0) {
        return;
    }
    if (first_byte == RESP_ERROR_PREFIX) {
        transaction->is_error = true;
    }
    if (transaction->command == REDIS_GET && first_byte != RESP_BULK_PREFIX) {
        goto cleanup;
    } else if (first_byte != RESP_SIMPLE_STRING_PREFIX) {
        goto cleanup;
    }
    transaction->response_last_seen = bpf_ktime_get_ns();
    goto enqueue;

enqueue:
    redis_batch_enqueue_wrapper(tup, transaction);
cleanup:
    bpf_map_delete_elem(&redis_in_flight, tup);
}

SEC("socket/redis_process")
int socket__redis_process(struct __sk_buff *skb) {
    skb_info_t skb_info = {};
    conn_tuple_t conn_tuple = {};
    if (!fetch_dispatching_arguments(&conn_tuple, &skb_info)) {
        return 0;
    }

    if (is_tcp_termination(&skb_info)) {
        redis_tcp_termination(&conn_tuple);
        return 0;
    }
    normalize_tuple(&conn_tuple);
    pktbuf_t pkt = pktbuf_from_skb(skb, &skb_info);

    redis_transaction_t *transaction = bpf_map_lookup_elem(&redis_in_flight, &conn_tuple);
    if (transaction == NULL) {
        process_redis_request(pkt, &conn_tuple);
    } else {
        process_redis_response(pkt, &conn_tuple, transaction);
    }

    return 0;
}

SEC("uprobe/redis_tls_process")
int uprobe__redis_tls_process(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return 0;
    }

    // Copying the tuple to the stack to handle verifier issues on kernel 4.14.
    conn_tuple_t tup = args->tup;

    pktbuf_t pkt = pktbuf_from_tls(ctx, args);
    redis_transaction_t *transaction = bpf_map_lookup_elem(&redis_in_flight, &tup);
    if (transaction == NULL) {
        process_redis_request(pkt, &tup);
    } else {
        process_redis_response(pkt, &tup, transaction);
    }
    return 0;
}

SEC("uprobe/redis_tls_termination")
int uprobe__redis_tls_termination(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return 0;
    }

    // Copying the tuple to the stack to handle verifier issues on kernel 4.14.
    conn_tuple_t tup = args->tup;
    redis_tcp_termination(&tup);

    return 0;
}

#endif /* __REDIS_DECODING_H */
