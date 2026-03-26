#ifndef __REDIS_DECODING_H
#define __REDIS_DECODING_H

#include "protocols/redis/decoding-maps.h"
#include "protocols/helpers/pktbuf.h"

PKTBUF_READ_INTO_BUFFER(redis_bulk, MAX_KEY_LEN, READ_KEY_CHUNK_SIZE)

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
static __always_inline u32 read_array_message_param_count(pktbuf_t pkt) {
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

// Extracts and returns the length of a Redis key from a RESP bulk string.
// Validates the format and returns 0 if invalid or exceeds maximum length.
static __always_inline u16 get_key_len(pktbuf_t pkt) {
    u32 current_offset = pktbuf_data_offset(pkt);
    const u32 data_end = pktbuf_data_end(pkt);

    char bulk_prefix;
    // Verify we can read the RESP bulk prefix.
    if (current_offset + sizeof(bulk_prefix) > data_end) {
        return 0;
    }
    if (pktbuf_load_bytes(pkt, current_offset, &bulk_prefix, sizeof(bulk_prefix)) < 0 || bulk_prefix != RESP_BULK_PREFIX) {
        return 0;
    }
    current_offset++;

    // Read key length (up to MAX_DIGITS_KEY_LEN_PREFIX digits)
    char key_size_bytes[MAX_DIGITS_KEY_LEN_PREFIX] = {};
    if (current_offset + sizeof(key_size_bytes) > data_end) {
        return 0;
    }
    if (pktbuf_load_bytes(pkt, current_offset, key_size_bytes, sizeof(key_size_bytes)) < 0) {
        return 0;
    }

    u16 key_size = 0;
    u32 digits_read = 0;
    // The key length is a decimal number, so we need to convert it from ASCII to an integer.
    #pragma unroll (MAX_DIGITS_KEY_LEN_PREFIX)
    for (int i = 0; i < MAX_DIGITS_KEY_LEN_PREFIX; i++) {
        if (key_size_bytes[i] == RESP_TERMINATOR_1) {
            break;
        }
        if (key_size_bytes[i] < '0' || key_size_bytes[i] > '9') {
            return 0;
        }
        key_size = key_size * 10 + (key_size_bytes[i] - '0');
        digits_read++;
    }

    // Advance past the digits we read
    current_offset += digits_read;
    pktbuf_set_offset(pkt, current_offset);

    if (!read_crlf(pkt)) {
        return 0;
    }

    if (key_size <= 0 || key_size > MAX_READABLE_KEY_LEN) {
        return 0;
    }

    return key_size;
}

// Reads a Redis key name into the provided buffer with length validation.
// Sets truncated flag if key was too long for buffer, and out_key_len as the key size after clamping.
static __always_inline bool read_key_name(pktbuf_t pkt, char *buf, u8 buf_len, u16 *out_key_len, bool *truncated) {
    const u32 key_size = *out_key_len > MAX_KEY_LEN - 1 ? MAX_KEY_LEN - 1 : *out_key_len;
    const u32 final_key_size = key_size > buf_len ? buf_len : key_size;
    if (final_key_size == 0) {
        return false;
    }

    // Read possibly more than out_key_len
    pktbuf_read_into_buffer_redis_bulk(buf, pkt, pktbuf_data_offset(pkt));
    // Advance by out_key_len since the remainder is not part of the key name
    pktbuf_advance(pkt, *out_key_len);

    // Read and skip past the CRLF after the key data
    if (!read_crlf(pkt)) {
        return false;
    }

    *truncated = final_key_size < *out_key_len;
    *out_key_len = final_key_size;
    return true;
}


// Processes incoming Redis requests (GET or SET commands).
// Extracts command type and key (up to MAX_KEY_LEN bytes), stores transaction info in redis_in_flight map.
static __always_inline void process_redis_request(pktbuf_t pkt, conn_tuple_t *conn_tuple, __u8 tags) {
    u32 param_count = read_array_message_param_count(pkt);
    if (param_count == 0) {
        return;
    }
    // PING has 1 parameter, GET has 2 parameters, SET has 3-5 parameters.
    if (param_count < MIN_PARAM_COUNT || param_count > MAX_PARAM_COUNT) {
        return;
    }

    // Read method length
    const u16 method_len = get_key_len(pkt);

    if (method_len < MIN_METHOD_LEN || method_len > MAX_METHOD_LEN) {
        return;
    }

    char method[MAX_METHOD_LEN + 1] = {};
    if (pktbuf_load_bytes_from_current_offset(pkt, method, MAX_METHOD_LEN + 1) < 0) {
        return;
    }
    pktbuf_advance(pkt, method_len);

    // Read CRLF after method
    if (!read_crlf(pkt)) {
        return;
    }
    convert_method_to_upper_case(method);

    // Declare transaction at function scope to help verifier
    redis_transaction_t transaction = {};
    transaction.tags = tags;
    transaction.request_started = bpf_ktime_get_ns();
    bool should_extract_key = false;
    if (bpf_memcmp(method, REDIS_CMD_SET, sizeof(REDIS_CMD_SET)-1) == 0) {
        transaction.command = REDIS_SET;
        should_extract_key = true;
    } else if (bpf_memcmp(method, REDIS_CMD_GET, sizeof(REDIS_CMD_GET)-1) == 0) {
        transaction.command = REDIS_GET;
        should_extract_key = true;
    } else if (bpf_memcmp(method, REDIS_CMD_PING, sizeof(REDIS_CMD_PING)-1) == 0) {
        transaction.command = REDIS_PING;
    } else {
        return;
    }

    if (should_extract_key && is_redis_with_key_monitoring_enabled()) {
        // Read key name (only for GET/SET, not PING)
        __u16 len = get_key_len(pkt);
        if (len == 0) {
            return;
        }
        redis_key_data_t key = {
            .len = len,
        };
        if (!read_key_name(pkt, key.buf, sizeof(key.buf), &key.len, &key.truncated)) {
            return;
        }
        bpf_map_update_with_telemetry(redis_key_in_flight, conn_tuple, &key, BPF_ANY);
    }

    bpf_map_update_with_telemetry(redis_in_flight, conn_tuple, &transaction, BPF_ANY);
}


// Handles TCP connection termination by cleaning up in-flight transactions.
// Removes entries from redis_in_flight map for both directions.
static void __always_inline redis_tcp_termination(conn_tuple_t *tup) {
    bpf_map_delete_elem(&redis_in_flight, tup);
    if (is_redis_with_key_monitoring_enabled()) {
        bpf_map_delete_elem(&redis_key_in_flight, tup);
    }
    flip_tuple(tup);
    bpf_map_delete_elem(&redis_in_flight, tup);
    if (is_redis_with_key_monitoring_enabled()) {
        bpf_map_delete_elem(&redis_key_in_flight, tup);
    }
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

// Enqueues a batch of events to the user-space. To spare stack size, we take a scratch buffer from the map, copy
// the connection tuple and the transaction to it, and then enqueue the event.
static __always_inline void redis_with_key_batch_enqueue_wrapper(conn_tuple_t *tuple, redis_transaction_t *tx, redis_key_data_t *key) {
    u32 zero = 0;
    redis_with_key_event_t *event = bpf_map_lookup_elem(&redis_with_key_scratch_buffer, &zero);
    if (!event) {
        return;
    }

    bpf_memcpy(&event->header.tuple, tuple, sizeof(conn_tuple_t));
    bpf_memcpy(&event->header.tx, tx, sizeof(redis_transaction_t));
    bpf_memcpy(&event->key, key, sizeof(redis_key_data_t));
    redis_with_key_batch_enqueue(event);
}

// Checks if a byte represents a valid RESP (Redis Serialization Protocol) response type prefix.
// Supports both RESP2 (Redis 1.2+) and RESP3 (Redis 6.0+) type prefixes.
// Returns true if the byte is a valid RESP response type, false otherwise.
static __always_inline bool is_valid_resp_response(char first_byte) {
    switch (first_byte) {
        // RESP2 types
        case RESP_SIMPLE_STRING_PREFIX:  // '+' - Simple string (e.g., "+OK\r\n")
        case RESP_ERROR_PREFIX:          // '-' - Error message (e.g., "-ERR unknown\r\n")
        case RESP_INTEGER_PREFIX:        // ':' - Integer (e.g., ":1000\r\n")
        case RESP_BULK_PREFIX:           // '$' - Bulk string (e.g., "$5\r\nhello\r\n")
        case RESP_ARRAY_PREFIX:          // '*' - Array (e.g., "*2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n")

        // RESP3 types (Redis 6.0+)
        case RESP3_NULL_PREFIX:          // '_' - Null value
        case RESP3_BOOLEAN_PREFIX:       // '#' - Boolean (e.g., "#t\r\n", "#f\r\n")
        case RESP3_DOUBLE_PREFIX:        // ',' - Double precision float
        case RESP3_BIG_NUMBER_PREFIX:    // '(' - Arbitrary precision integer
        case RESP3_BULK_ERROR_PREFIX:    // '!' - Bulk error (e.g., "!21\r\nSYNTAX error\r\n")
        case RESP3_VERBATIM_STRING_PREFIX: // '=' - Verbatim string with encoding
        case RESP3_MAP_PREFIX:           // '%' - Map/Dictionary
        case RESP3_SET_PREFIX:           // '~' - Set
        case RESP3_PUSH_PREFIX:          // '>' - Push data (pub/sub)
            return true;
        default:
            return false;
    }
}

// Checks if a response byte indicates an error condition.
// Supports both RESP2 error prefix ('-') and RESP3 bulk error prefix ('!').
static __always_inline bool is_resp_error(char first_byte) {
    return first_byte == RESP_ERROR_PREFIX || first_byte == RESP3_BULK_ERROR_PREFIX;
}

// Processes Redis response messages and validates their format.
// Handles all RESP2 and RESP3 response types for comprehensive monitoring coverage.
static void __always_inline process_redis_response(pktbuf_t pkt, conn_tuple_t *tup, redis_transaction_t *transaction) {
    redis_key_data_t *key = NULL;
    redis_key_data_t empty_key = {};  // For PING commands when resource tracking is enabled
    if (is_redis_with_key_monitoring_enabled()) {
        key = bpf_map_lookup_elem(&redis_key_in_flight, tup);
        // When resource tracking is enabled:
        // - PING doesn't have a key, so key can be NULL for PING commands
        // - GET/SET must have a key, so if key is NULL, it's an error
        if (key == NULL) {
            if (transaction->command == REDIS_PING) {
                // For PING commands, use an empty key when sending to keyed stream
                key = &empty_key;
            } else {
                // For GET/SET commands, key is required when resource tracking is enabled
                goto cleanup;
            }
        }
    }

    char first_byte;
    if (pktbuf_load_bytes_from_current_offset(pkt, &first_byte, sizeof(first_byte)) < 0) {
        return;
    }

    // Check if this is a valid RESP response type (RESP2 or RESP3)
    if (!is_valid_resp_response(first_byte)) {
        goto cleanup;  // Not a valid RESP response
    }

    // Mark error responses (both RESP2 '-' and RESP3 '!' types)
    if (is_resp_error(first_byte)) {
        transaction->is_error = true;
    }

    // All valid RESP responses are accepted regardless of command type
    goto enqueue;

enqueue:
    transaction->response_last_seen = bpf_ktime_get_ns();
    // When resource tracking is enabled, ALL commands (including PING) go to the keyed stream
    // PING commands use an empty key, GET/SET commands use their actual keys
    if (is_redis_with_key_monitoring_enabled()) {
        redis_with_key_batch_enqueue_wrapper(tup, transaction, key);
    } else {
        redis_batch_enqueue_wrapper(tup, transaction);
    }
cleanup:
    bpf_map_delete_elem(&redis_in_flight, tup);
    if (is_redis_with_key_monitoring_enabled()) {
        bpf_map_delete_elem(&redis_key_in_flight, tup);
    }
}

// Main socket processing function for Redis traffic.
// Handles both requests and responses based on connection state.
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
        process_redis_request(pkt, &conn_tuple, NO_TAGS);
    } else {
        process_redis_response(pkt, &conn_tuple, transaction);
    }

    return 0;
}

// Processes Redis messages over TLS connections.
// Similar to socket__redis_process but handles TLS-encrypted traffic.
SEC("uprobe/redis_tls_process")
int uprobe__redis_tls_process(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return 0;
    }

    // Copying the tuple to the stack to handle verifier issues on kernel 4.14.
    conn_tuple_t tup = args->tup;
    normalize_tuple(&tup);

    pktbuf_t pkt = pktbuf_from_tls(ctx, args);
    redis_transaction_t *transaction = bpf_map_lookup_elem(&redis_in_flight, &tup);
    if (transaction == NULL) {
        process_redis_request(pkt, &tup, (__u8)args->tags);
    } else {
        process_redis_response(pkt, &tup, transaction);
    }
    return 0;
}

// Handles termination of TLS Redis connections.
// Cleans up connection state for TLS connections.
SEC("uprobe/redis_tls_termination")
int uprobe__redis_tls_termination(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return 0;
    }

    // Copying the tuple to the stack to handle verifier issues on kernel 4.14.
    conn_tuple_t tup = args->tup;
    normalize_tuple(&tup);
    redis_tcp_termination(&tup);

    return 0;
}

#endif /* __REDIS_DECODING_H */
