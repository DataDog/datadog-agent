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
static __always_inline void handle_new_query(pktbuf_t pkt, conn_tuple_t *conn_tuple, __u32 query_len) {
    postgres_transaction_t new_transaction = {};
    new_transaction.request_started = bpf_ktime_get_ns();
    u32 data_off = pktbuf_data_offset(pkt);
    pktbuf_read_into_buffer_postgres_query((char *)new_transaction.request_fragment, pkt, data_off);
    new_transaction.original_query_size = query_len;
    bpf_map_update_elem(&postgres_in_flight, conn_tuple, &new_transaction, BPF_ANY);
}

// Handles a command complete message by enqueuing the transaction and deleting it from the in-flight map.
// The format of the command complete message is described here: https://www.postgresql.org/docs/current/protocol-message-formats.html#PROTOCOL-MESSAGE-FORMATS-COMMANDCOMPLETE
static __always_inline void handle_command_complete(conn_tuple_t *conn_tuple, postgres_transaction_t *transaction) {
    transaction->response_last_seen = bpf_ktime_get_ns();
    postgres_batch_enqueue_wrapper(conn_tuple, transaction);
    bpf_map_delete_elem(&postgres_in_flight, conn_tuple);
}

static void __always_inline postgres_tcp_termination(conn_tuple_t *tup) {
    bpf_map_delete_elem(&postgres_in_flight, tup);
    flip_tuple(tup);
    bpf_map_delete_elem(&postgres_in_flight, tup);
}

// Main processing logic for the Postgres protocol. It reads the first message header and decides what to do based on the
// message tag. If the message is a new query, it stores the query in the in-flight map. If the message is a command
// complete, it enqueues the transaction and deletes it from the in-flight map. If the message is not a command complete,
// it tries to read up to POSTGRES_MAX_MESSAGES messages, looking for a command complete message.
// If the message is not a new query or a command complete, it ignores the message.
static __always_inline void postgres_entrypoint(pktbuf_t pkt, conn_tuple_t *conn_tuple) {
    // Read first message header
    struct pg_message_header header;
    if (!read_message_header(pkt, &header)) {
        return;
    }
    // Advance the data offset to the end of the first message header.
    pktbuf_advance(pkt, sizeof(struct pg_message_header));

    // If the message is a new query, we store the query in the in-flight map.
    // If we had a transaction for the connection, we override it and drops the previous one.
    if (header.message_tag == POSTGRES_QUERY_MAGIC_BYTE) {
        // message_len includes size of the payload, 4 bytes of the message length itself, but not the message tag.
        // So if we want to know the size of the payload, we need to subtract the size of the message length.
        handle_new_query(pkt, conn_tuple, header.message_len - sizeof(__u32));
        return;
    }

    // We didn't find a new query, thus we assume we're in the middle of a transaction.
    // We look up the transaction in the in-flight map, and if it doesn't exist, we ignore the message.
    postgres_transaction_t *transaction = bpf_map_lookup_elem(&postgres_in_flight, conn_tuple);
    if (!transaction) {
        return;
    }

    // If the message is a command complete, we enqueue the transaction and delete it from the in-flight map.
    if (header.message_tag == POSTGRES_COMMAND_COMPLETE_MAGIC_BYTE) {
        handle_command_complete(conn_tuple, transaction);
        return;
    }

    // We're in the middle of a transaction, and the message is not a command complete, but it can be a chain of
    // messages. So we try to read up to POSTGRES_MAX_MESSAGES messages, looking for a command complete message.

    // Advance the data offset to the end of the first message (after the payload). The message length includes the size
    // of the payload, 4 bytes of the message length itself, but not the message tag. Since we already moved the data
    // offset to the end of the message header, we want to jump over the payload.
    pktbuf_advance(pkt, header.message_len - sizeof(__u32));

#pragma unroll(POSTGRES_MAX_MESSAGES)
    for (__u32 iteration = 0; iteration < POSTGRES_MAX_MESSAGES; ++iteration) {
        if (!read_message_header(pkt, &header)) {
            break;
        }
        if (header.message_tag == POSTGRES_COMMAND_COMPLETE_MAGIC_BYTE) {
            handle_command_complete(conn_tuple, transaction);
            break;
        }
        // We didn't find a command complete message, so we advance the data offset to the end of the message.
        // reminder, the message length includes the size of the payload, 4 bytes of the message length itself, but not
        // the message tag. So we need to add 1 to the message length to jump over the entire message.
        pktbuf_advance(pkt, header.message_len + 1);
    }
    return;
}

SEC("socket/postgres_process")
int socket__postgres_process(struct __sk_buff* skb) {
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
    postgres_entrypoint(pkt, &conn_tuple);
    return 0;
}

SEC("uprobe/postgres_tls_process")
int uprobe__postgres_tls_process(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return 0;
    }

    // Copying the tuple to the stack to handle verifier issues on kernel 4.14.
    conn_tuple_t tup = args->tup;

    pktbuf_t pkt = pktbuf_from_tls(args);
    postgres_entrypoint(pkt, &tup);
    return 0;
}

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

#endif
