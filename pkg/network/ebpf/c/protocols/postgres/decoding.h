#ifndef __POSTGRES_DECODING_H
#define __POSTGRES_DECODING_H

#include "bpf_builtins.h"
#include "bpf_telemetry.h"

#include "protocols/sockfd.h"

#include "protocols/helpers/pktbuf.h"
#include "protocols/postgres/decoding-maps.h"
#include "protocols/postgres/defs.h"
#include "protocols/postgres/types.h"
#include "protocols/postgres/usm-events.h"
#include "protocols/read_into_buffer.h"

PKTBUF_READ_INTO_BUFFER(postgres_query, POSTGRES_BUFFER_SIZE, BLK_SIZE)

// Enqueues a batch of events to the user-space. To spare stack size, we take a scratch buffer from the map, copy
// the connection tuple and the transaction to it, and then enqueue the event.
static __always_inline void postgres_batch_enqueue_wrapper(conn_tuple_t *tuple, postgres_transaction_t *tx) {
    u32 zero = 0;
    postgres_event_t *event = bpf_map_lookup_elem(&postgres_scratch_buffer, &zero);
    if (!event) {
        return;
    }

    bpf_memcpy(&event->tuple, tuple, sizeof(conn_tuple_t));
    bpf_memcpy(&event->tx, tx, sizeof(postgres_transaction_t));
    postgres_batch_enqueue(event);
}

// Reads a message header from the given context. Returns true if the header was read successfully, false otherwise.
static __always_inline bool read_message_header(pktbuf_t pkt, struct pg_message_header* header) {
    u32 data_off = pktbuf_data_offset(pkt);
    u32 data_end = pktbuf_data_end(pkt);
    // Ensuring that the header is in the buffer.
    if (data_off + sizeof(struct pg_message_header) > data_end) {
        return false;
    }
    pktbuf_load_bytes(pkt, data_off, header, sizeof(struct pg_message_header));
    // Converting the header to host byte order.
    header->message_len = bpf_ntohl(header->message_len);
    return true;
}

// Handles a new query by creating a new transaction and storing it in the map.
// If a transaction already exists for the given connection, it is aborted.
// Query message format - https://www.postgresql.org/docs/current/protocol-message-formats.html#PROTOCOL-MESSAGE-FORMATS-QUERY
// the first 5 bytes are the message header, and the query is the rest of the payload.
static __always_inline void handle_new_query(pktbuf_t pkt, conn_tuple_t *conn_tuple, __u32 query_len, __u8 tags) {
    postgres_transaction_t new_transaction = {};
    new_transaction.request_started = bpf_ktime_get_ns();
    u32 data_off = pktbuf_data_offset(pkt);
    pktbuf_read_into_buffer_postgres_query((char *)new_transaction.request_fragment, pkt, data_off);
    new_transaction.original_query_size = query_len;
    new_transaction.tags = tags;
    bpf_map_update_elem(&postgres_in_flight, conn_tuple, &new_transaction, BPF_ANY);
}

// Handles a command complete message by enqueuing the transaction and deleting it from the in-flight map.
// The format of the command complete message is described here: https://www.postgresql.org/docs/current/protocol-message-formats.html#PROTOCOL-MESSAGE-FORMATS-COMMANDCOMPLETE
static __always_inline void handle_command_complete(conn_tuple_t *conn_tuple, postgres_transaction_t *transaction) {
    transaction->response_last_seen = bpf_ktime_get_ns();
    postgres_batch_enqueue_wrapper(conn_tuple, transaction);
    bpf_map_delete_elem(&postgres_in_flight, conn_tuple);
}

// Handles a TCP termination event by deleting the connection tuple from the in-flight map.
static void __always_inline postgres_tcp_termination(conn_tuple_t *tup) {
    bpf_map_delete_elem(&postgres_in_flight, tup);
    flip_tuple(tup);
    bpf_map_delete_elem(&postgres_in_flight, tup);
}

// Tries to skip the next null-terminated string. Returns the number of bytes to skip, or 0 if the null terminator was
// not found within the first 128 (POSTGRES_SKIP_STRING_ITERATIONS * POSTGRES_SKIP_STRING_READ_SIZE) bytes.
static int __always_inline skip_string(pktbuf_t pkt, int message_len) {
    const __u32 original_data_off = pktbuf_data_offset(pkt);
    __u32 data_off = original_data_off;
    __u32 data_end = pktbuf_data_end(pkt);
    // If the message is larger than the buffer, we limit the data_end to the end of the message.
    if (data_off + message_len < data_end) {
        data_end = data_off + message_len;
    }

    char temp_buffer[POSTGRES_SKIP_STRING_READ_SIZE] = {0};
    __u8 size_to_read = 0;

    #pragma unroll(POSTGRES_SKIP_STRING_ITERATIONS)
    for (int iter = 0; iter < POSTGRES_SKIP_STRING_ITERATIONS; iter++) {
        // We read the next block of data into the temp buffer. We read the minimum between the size of the temp buffer
        // and the remaining data in the message.
        size_to_read = data_end - data_off > sizeof(temp_buffer) ? sizeof(temp_buffer) : data_end - data_off;
        pktbuf_load_bytes(pkt, data_off, temp_buffer, sizeof(temp_buffer));

        #pragma unroll(POSTGRES_SKIP_STRING_READ_SIZE)
        for (int i = 0; i < POSTGRES_SKIP_STRING_READ_SIZE; i++) {
            if (i >= size_to_read) {
                return SKIP_STRING_FAILED;
            }
            if (temp_buffer[i] == NULL_TERMINATOR) {
                return data_off + i + 1 - original_data_off;
            }
        }

        data_off += size_to_read;
    }
    return SKIP_STRING_FAILED;
}

// Return a pointer to the postgres telemetry record in the corresponding map.
static __always_inline void* get_pg_msg_counts_map(pktbuf_t pkt) {
    const __u32 plain_key = 0;
    const __u32 tls_key = 1;

    pktbuf_map_lookup_option_t pg_telemetry_lookup_opt[] = {
        [PKTBUF_SKB] = {
            .map = &postgres_telemetry,
            .key = (void*)&plain_key,
        },
        [PKTBUF_TLS] = {
            .map = &postgres_telemetry,
            .key = (void*)&tls_key,
        },
    };
    return pktbuf_map_lookup(pkt, pg_telemetry_lookup_opt);
}

// update_msg_count_telemetry increases the corresponding counter of the telemetry bucket.
static __always_inline void update_msg_count_telemetry(postgres_kernel_msg_count_t* pg_msg_counts, __u8 count) {
    // This line can be interpreted as a step function of the difference, multiplied by the difference itself.
    // The step function of the difference returns 0 if the difference is negative and 1 if it is positive.
    // As a result, if the difference is negative, the output will be 0; if the difference is positive,
    // the output will equal the difference.
    count = count < PG_KERNEL_MSG_COUNT_FIRST_BUCKET ? 0 : count - PG_KERNEL_MSG_COUNT_FIRST_BUCKET;

    // This line functions as a ceiling operation, ensuring that if the count is not a multiple of the bucket size,
    // it is rounded up to the next bucket. Since eBPF does not support floating-point numbers, the implementation
    // adds (bucket size - 1) to the count and then divides the result by the bucket size.
    // This effectively simulates the ceiling function.
    __u8 bucket_idx = (count + PG_KERNEL_MSG_COUNT_BUCKET_SIZE - 1) / PG_KERNEL_MSG_COUNT_BUCKET_SIZE;

    // This line ensures that the bucket index stays within the range of 0 to PG_KERNEL_MSG_COUNT_NUM_BUCKETS.
    // While not strictly necessary, we include this check to satisfy the verifier and to explicitly define a lower bound.
    bucket_idx = bucket_idx < 0 ? 0 : bucket_idx;

    // This line ensures that the bucket index remains within the range of 0 to PG_KERNEL_MSG_COUNT_NUM_BUCKETS,
    // preventing any possibility of exceeding the upper bound.
    bucket_idx = bucket_idx >= PG_KERNEL_MSG_COUNT_NUM_BUCKETS ? PG_KERNEL_MSG_COUNT_NUM_BUCKETS-1 : bucket_idx;
    __sync_fetch_and_add(&pg_msg_counts->msg_count_buckets[bucket_idx], 1);
}

// Reads the first message header and decides what to do based on the
// message tag. If the message is a new query, it stores the query in the in-flight map.
// If the message is a parse message, we tail call to the dedicated process_parse_message program.
// If the message is a command complete, it calls the handle_command_complete program.
static __always_inline void postgres_handle_message(pktbuf_t pkt, conn_tuple_t *conn_tuple, struct pg_message_header *header, __u8 tags) {
    // If the message is a parse message, we tail call to the dedicated function to handle it as it is too large to be
    // inlined in the main function.
    if (header->message_tag == POSTGRES_PARSE_MAGIC_BYTE) {
        pktbuf_tail_call_option_t process_parse_tail_call_array[] = {
            [PKTBUF_SKB] = {
                .prog_array_map = &protocols_progs,
                .index = PROG_POSTGRES_PROCESS_PARSE_MESSAGE,
            },
            [PKTBUF_TLS] = {
                .prog_array_map = &tls_process_progs,
                .index = PROG_POSTGRES_PROCESS_PARSE_MESSAGE,
            },
        };
        pktbuf_tail_call_compact(pkt, process_parse_tail_call_array);
        return;
    }

    // If the message is a new query, we store the query in the in-flight map.
    // If we had a transaction for the connection, we override it and drops the previous one.
    if (header->message_tag == POSTGRES_QUERY_MAGIC_BYTE) {
        // Read first message header
        // Advance the data offset to the end of the first message header.
        pktbuf_advance(pkt, sizeof(struct pg_message_header));
        // message_len includes size of the payload, 4 bytes of the message length itself, but not the message tag.
        // So if we want to know the size of the payload, we need to subtract the size of the message length.
        handle_new_query(pkt, conn_tuple, header->message_len - sizeof(__u32), tags);
        return;
    }

    const __u32 zero = 0;
    postgres_tail_call_state_t *iteration_value = bpf_map_lookup_elem(&postgres_iterations, &zero);
    if (iteration_value == NULL) {
        return;
    }

    iteration_value->total_msg_count = 0;
    iteration_value->data_off = 0;
    pktbuf_tail_call_option_t handle_response_tail_call_array[] = {
        [PKTBUF_SKB] = {
            .prog_array_map = &protocols_progs,
            .index = PROG_POSTGRES_HANDLE_RESPONSE,
        },
        [PKTBUF_TLS] = {
            .prog_array_map = &tls_process_progs,
            .index = PROG_POSTGRES_HANDLE_RESPONSE,
        },
    };
    pktbuf_tail_call_compact(pkt, handle_response_tail_call_array);
    return;
}

// A dedicated function to handle the parse message. This function is called from a tail call from the main entrypoint.
// The reason for this is that the main entrypoint is too large to be inlined, and the verifier has issues with it.
static __always_inline void postgres_handle_parse_message(pktbuf_t pkt, conn_tuple_t *conn_tuple, __u8 tags) {
    // Read first message header
    struct pg_message_header header;
    if (!read_message_header(pkt, &header)) {
        return;
    }
    // Advance the data offset to the end of the first message header.
    pktbuf_advance(pkt, sizeof(struct pg_message_header));

    // message_len includes size of the payload, 4 bytes of the message length itself, but not the message tag.
    // So if we want to know the size of the payload, we need to subtract the size of the message length.
    __u32 payload_data_length = header.message_len - sizeof(__u32);
    int length = skip_string(pkt, payload_data_length);
    if (length <= 0 || length >= payload_data_length) {
        // We failed to find the null terminator within the first 128 bytes of the message, so we cannot read the
        // query string. We ignore the message. If length is 0, we failed to find the null terminator, and if it's
        // greater than or equal to the payload length, we reached to the end of the payload and we don't have the query
        // string after the first string.
        return;
    }
    pktbuf_advance(pkt, length);
    header.message_len -= length;

    // message_len includes size of the payload, 4 bytes of the message length itself, but not the message tag.
    // So if we want to know the size of the payload, we need to subtract the size of the message length.
    handle_new_query(pkt, conn_tuple, header.message_len - sizeof(__u32), tags);
    return;
}

// Handles Postgres command complete messages by examining packet data for both plaintext and TLS traffic.
// This function handles multiple messages within a single packet, processing up to POSTGRES_MAX_MESSAGES_PER_TAIL_CALL
// messages per call. When more messages exist beyond this limit, it uses tail call chaining to continue processing.
static __always_inline bool handle_response(pktbuf_t pkt, conn_tuple_t conn_tuple, postgres_kernel_msg_count_t* pg_msg_counts) {
    const __u32 zero = 0;
    bool read_result = false;
    bool found_command_complete = false;
    struct pg_message_header header;

    postgres_tail_call_state_t *iteration_value = bpf_map_lookup_elem(&postgres_iterations, &zero);
    if (iteration_value == NULL) {
        bpf_map_delete_elem(&postgres_in_flight, &conn_tuple);
        return 0;
    }

    if (iteration_value->total_msg_count >= (POSTGRES_MAX_TOTAL_MESSAGES - 1)) {
        return 0;
    }

    if (iteration_value->data_off != 0) {
        pktbuf_set_offset(pkt, iteration_value->data_off);
    }

    // We didn't find a new query, thus we assume we're in the middle of a transaction.
    // We look up the transaction in the in-flight map, and if it doesn't exist, we ignore the message.
    postgres_transaction_t *transaction = bpf_map_lookup_elem(&postgres_in_flight, &conn_tuple);
    if (!transaction) {
        return 0;
    }

    __u8 messages_count = 0;
#pragma unroll(POSTGRES_MAX_MESSAGES_PER_TAIL_CALL)
    for (; messages_count < POSTGRES_MAX_MESSAGES_PER_TAIL_CALL; ++messages_count) {
        read_result = read_message_header(pkt, &header);
        if (read_result != true) {
            break;
        }
        if (header.message_tag == POSTGRES_COMMAND_COMPLETE_MAGIC_BYTE) {
            found_command_complete = true;
            break;
        }
        // We didn't find a command complete message, so we advance the data offset to the end of the message.
        // reminder, the message length includes the size of the payload, 4 bytes of the message length itself, but not
        // the message tag. So we need to add 1 to the message length to jump over the entire message.
        pktbuf_advance(pkt, header.message_len + 1);
    }
    iteration_value->total_msg_count += messages_count;

    if (found_command_complete) {
        handle_command_complete(&conn_tuple, transaction);
        update_msg_count_telemetry(pg_msg_counts, iteration_value->total_msg_count);

        return 0;
    }

    if (iteration_value->total_msg_count >= (POSTGRES_MAX_TOTAL_MESSAGES - 1)) {
        // reached max messages, add counter and stop iterating.
        __sync_fetch_and_add(&pg_msg_counts->reached_max_messages, 1);
        return 0;
    }
    if (pktbuf_data_offset(pkt) == pktbuf_data_end(pkt)) {
        // stop the iterator if the end of the TCP packet is reached.
        update_msg_count_telemetry(pg_msg_counts, iteration_value->total_msg_count);
        return 0;
    }
    if (read_result == false) {
        // the packet was fragmented, add counter stop iterating.
        __sync_fetch_and_add(&pg_msg_counts->fragmented_packets, 1);
        return 0;
    }

    // We didn't find a command complete message, so we need to continue processing the packet.
    // We save the current data offset.
    iteration_value->data_off = pktbuf_data_offset(pkt);

    pktbuf_tail_call_option_t handle_response_tail_call_array[] = {
        [PKTBUF_SKB] = {
            .prog_array_map = &protocols_progs,
            .index = PROG_POSTGRES_HANDLE_RESPONSE,
        },
        [PKTBUF_TLS] = {
            .prog_array_map = &tls_process_progs,
            .index = PROG_POSTGRES_HANDLE_RESPONSE,
        },
    };
    pktbuf_tail_call_compact(pkt, handle_response_tail_call_array);
    return 0;
}

// Entrypoint to process plaintext Postgres traffic. Pulls the connection tuple and the packet buffer from the map and
// calls the main processing function. If the packet is a TCP termination, it calls the termination function.
SEC("socket/postgres_handle")
int socket__postgres_handle(struct __sk_buff* skb) {
    skb_info_t skb_info = {};
    conn_tuple_t conn_tuple = {};

    if (!fetch_dispatching_arguments(&conn_tuple, &skb_info)) {
        return 0;
    }

    if (is_tcp_termination(&skb_info)) {
        postgres_tcp_termination(&conn_tuple);
        return 0;
    }

    normalize_tuple(&conn_tuple);

    pktbuf_t pkt = pktbuf_from_skb(skb, &skb_info);
    struct pg_message_header header;
    if (!read_message_header(pkt, &header)) {
        return 0;
    }

    postgres_handle_message(pkt, &conn_tuple, &header, NO_TAGS);
    return 0;
}

// Handles plain text command complete messages for plaintext Postgres traffic. Pulls the connection tuple and the
// packet buffer from the map and calls the dedicated function to handle the message.
SEC("socket/postgres_handle_response")
int socket__postgres_handle_response(struct __sk_buff* skb) {
    skb_info_t skb_info = {};
    conn_tuple_t conn_tuple = {};

    if (!fetch_dispatching_arguments(&conn_tuple, &skb_info)) {
        return 0;
    }

    if (is_tcp_termination(&skb_info)) {
        postgres_tcp_termination(&conn_tuple);
        return 0;
    }

    normalize_tuple(&conn_tuple);

    pktbuf_t pkt = pktbuf_from_skb(skb, &skb_info);
    postgres_kernel_msg_count_t* pg_msg_counts = get_pg_msg_counts_map(pkt);
    if (pg_msg_counts == NULL) {
        return 0;
    }
    handle_response(pkt, conn_tuple, pg_msg_counts);
    return 0;
}

// Handles plaintext Postgres Parse messages. Pulls the connection tuple and the packet buffer from the map and calls the
// dedicated function to handle the message.
SEC("socket/postgres_process_parse_message")
int socket__postgres_process_parse_message(struct __sk_buff* skb) {
    skb_info_t skb_info = {};
    conn_tuple_t conn_tuple = {};

    if (!fetch_dispatching_arguments(&conn_tuple, &skb_info)) {
        return 0;
    }

    normalize_tuple(&conn_tuple);

    pktbuf_t pkt = pktbuf_from_skb(skb, &skb_info);
    postgres_handle_parse_message(pkt, &conn_tuple, NO_TAGS);
    return 0;
}

// Entrypoint to process TLS Postgres traffic. Pulls the connection tuple and the packet buffer from the map and calls
// the main processing function.
SEC("uprobe/postgres_tls_handle")
int uprobe__postgres_tls_handle(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return 0;
    }

    // Copying the tuple to the stack to handle verifier issues on kernel 4.14.
    conn_tuple_t tup = args->tup;

    pktbuf_t pkt = pktbuf_from_tls(ctx, args);
    struct pg_message_header header;
    if (!read_message_header(pkt, &header)) {
        return 0;
    }

    postgres_handle_message(pkt, &tup, &header, (__u8)args->tags);
    return 0;
}

// Handles TLS Postgres Parse messages. Pulls the connection tuple and the packet buffer from the map and calls the
// dedicated function to handle the message.
SEC("uprobe/postgres_tls_process_parse_message")
int uprobe__postgres_tls_process_parse_message(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return 0;
    }

    // Copying the tuple to the stack to handle verifier issues on kernel 4.14.
    conn_tuple_t tup = args->tup;

    pktbuf_t pkt = pktbuf_from_tls(ctx, args);
    postgres_handle_parse_message(pkt, &tup, (__u8)args->tags);
    return 0;
}

// Handles connection termination for a TLS Postgres connection.
SEC("uprobe/postgres_tls_termination")
int uprobe__postgres_tls_termination(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return 0;
    }

    // Copying the tuple to the stack to handle verifier issues on kernel 4.14.
    conn_tuple_t tup = args->tup;
    postgres_tcp_termination(&tup);
    return 0;
}

// Handles message parsing for a TLS Postgres traffic.
SEC("uprobe/postgres_tls_handle_response")
int uprobe__postgres_tls_handle_response(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return 0;
    }

    pktbuf_t pkt = pktbuf_from_tls(ctx, args);
    postgres_kernel_msg_count_t* pg_msg_counts = get_pg_msg_counts_map(pkt);
    if (pg_msg_counts == NULL) {
        return 0;
    }

    // Copying the tuple to the stack to handle verifier issues on kernel 4.14.
    conn_tuple_t tup = args->tup;
    handle_response(pkt, tup, pg_msg_counts);
    return 0;
}

#endif
