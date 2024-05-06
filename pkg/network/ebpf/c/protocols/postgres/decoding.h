#ifndef __POSTGRES_H
#define __POSTGRES_H

#include "bpf_builtins.h"
#include "bpf_telemetry.h"

#include "protocols/sockfd.h"

#include "protocols/classification/common.h"
#include "protocols/postgres/defs.h"
#include "protocols/postgres/maps.h"
#include "protocols/postgres/types.h"
#include "protocols/postgres/usm-events.h"
#include "protocols/read_into_buffer.h"

READ_INTO_BUFFER(postgres_query, POSTGRES_BUFFER_SIZE, BLK_SIZE)

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

// Reads a message header from the given context.
// Return true if the header was read successfully, false otherwise.
static __always_inline bool read_message_header(struct __sk_buff *skb, skb_info_t *skb_info, struct pg_message_header* header) {
    if (bpf_skb_load_bytes(skb, skb_info->data_off, header, sizeof(struct pg_message_header)) < 0) {
        return false;
    }
    header->message_len = bpf_ntohl(header->message_len);
    return true;
}

// Handles a new query by creating a new transaction and storing it in the map.
// If a transaction already exists for the given connection, it is aborted.
static __always_inline void handle_new_query(struct __sk_buff *skb, skb_info_t *skb_info, conn_tuple_t *conn_tuple, __u32 query_len) {
    postgres_transaction_t new_transaction = {};
    new_transaction.request_started = bpf_ktime_get_ns();
    read_into_buffer_postgres_query((char *)new_transaction.request_fragment, skb, skb_info->data_off);
    new_transaction.frag_size = query_len > POSTGRES_BUFFER_SIZE ? POSTGRES_BUFFER_SIZE : query_len;
    bpf_map_update_elem(&postgres_in_flight, conn_tuple, &new_transaction, BPF_ANY);
}

static __always_inline void handle_command_complete(conn_tuple_t *conn_tuple, postgres_transaction_t *transaction) {
    transaction->response_last_seen = bpf_ktime_get_ns();
    postgres_batch_enqueue_wrapper(conn_tuple, transaction);
    bpf_map_delete_elem(&postgres_in_flight, conn_tuple);
}

SEC("socket/postgres_process")
int socket__postgres_process(struct __sk_buff* skb) {
    skb_info_t skb_info = {};
    conn_tuple_t conn_tuple = {};

    if (!fetch_dispatching_arguments(&conn_tuple, &skb_info)) {
        return 0;
    }

    normalize_tuple(&conn_tuple);

    // Read first message header
    struct pg_message_header header;
    if (!read_message_header(skb, &skb_info, &header)) {
        return 0;
    }
    skb_info.data_off += sizeof(struct pg_message_header);

    if (header.message_tag == POSTGRES_QUERY_MAGIC_BYTE) { // new query.
        handle_new_query(skb, &skb_info, &conn_tuple, header.message_len - 4);
        // Currently, for query events, we only process the first message.
        // TODO: guy - is it possible to have more than a single query message?
        return 0;
    }

    postgres_transaction_t *transaction = bpf_map_lookup_elem(&postgres_in_flight, &conn_tuple);
    if (!transaction) {
        return 0;
    }

    if (header.message_tag == POSTGRES_COMMAND_COMPLETE_MAGIC_BYTE) {
        // command complete
        handle_command_complete(&conn_tuple, transaction);
        return 0;
    }

    skb_info.data_off += header.message_len + 1 - sizeof(struct pg_message_header);
    // Command complete might arrive in a chained message.
#pragma unroll(POSTGRES_MAX_MESSAGES)
    for (__u32 iteration = 0; iteration < POSTGRES_MAX_MESSAGES; ++iteration) {
        if (!read_message_header(skb, &skb_info, &header)) {
            break;
        }
        skb_info.data_off += header.message_len + 1;
        if (header.message_tag == POSTGRES_COMMAND_COMPLETE_MAGIC_BYTE) {
            handle_command_complete(&conn_tuple, transaction);
            break;
        }
    }
    return 0;
}

#endif
