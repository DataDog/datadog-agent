#ifndef __KAFKA_PARSING
#define __KAFKA_PARSING

#include "bpf_builtins.h"
#include "bpf_telemetry.h"
#include "protocols/kafka/types.h"
#include "protocols/kafka/parsing-maps.h"
#include "protocols/kafka/usm-events.h"

// forward declaration
static __always_inline bool kafka_allow_packet(skb_info_t *skb_info);
static __always_inline bool kafka_process(conn_tuple_t *tup, kafka_info_t *kafka, pktbuf_t pkt, kafka_telemetry_t *kafka_tel);
static __always_inline bool kafka_process_response(void *ctx, conn_tuple_t *tup, kafka_info_t *kafka, pktbuf_t pkt, skb_info_t *skb_info);
static __always_inline void update_topic_name_size_telemetry(kafka_telemetry_t *kafka_tel, __u64 size);

// A template for verifying a given buffer is composed of the characters [a-z], [A-Z], [0-9], ".", "_", or "-".
// The iterations reads up to MIN(max_buffer_size, real_size).
// Has to be a template and not a function, as we have pragma unroll.
#define CHECK_STRING_COMPOSED_OF_ASCII_FOR_PARSING(max_buffer_size, real_size, buffer)                                                      \
    char ch = 0;                                                                                                                            \
_Pragma( STRINGIFY(unroll(max_buffer_size)) )                                                                                               \
    for (int j = 0; j < max_buffer_size; j++) {                                                                                             \
        /* Verifies we are not exceeding the real client_id_size, and if we do, we finish the iteration as we reached */                    \
        /* to the end of the buffer and all checks have been successful. */                                                                 \
        if (j + 1 <= real_size) {                                                                                                           \
            ch = buffer[j];                                                                                                                 \
            if (('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ('0' <= ch && ch <= '9') || ch == '.' || ch == '_' || ch == '-') {  \
                continue;                                                                                                                   \
            }                                                                                                                               \
            return false;                                                                                                                   \
        }                                                                                                                                   \
    }                                                                                                                                       \

#ifdef EXTRA_DEBUG
#define extra_debug(fmt, ...) log_debug("kafka: " fmt, ##__VA_ARGS__)
#else
#define extra_debug(fmt, ...)
#endif

static void __always_inline kafka_tcp_termination(conn_tuple_t *tup)
{
    bpf_map_delete_elem(&kafka_response, tup);
    // Delete the opposite direction also like HTTP/2 does since the termination
    // for the other direction may not be reached in some cases (localhost).
    flip_tuple(tup);
    bpf_map_delete_elem(&kafka_response, tup);
}

SEC("socket/kafka_filter")
int socket__kafka_filter(struct __sk_buff* skb) {
    const u32 zero = 0;
    skb_info_t skb_info;
    kafka_info_t *kafka = bpf_map_lookup_elem(&kafka_heap, &zero);
    if (kafka == NULL) {
        log_debug("socket__kafka_filter: kafka_transaction state is NULL");
        return 0;
    }
    bpf_memset(&kafka->event.transaction, 0, sizeof(kafka_transaction_t));

    // Put this on the stack instead of using the one in in kafka_info_t.event
    // since it's used for map lookups in a few different places and 4.14 complains
    // if it's not on the stack.
    conn_tuple_t tup;

    if (!fetch_dispatching_arguments(&tup, &skb_info)) {
        log_debug("socket__kafka_filter failed to fetch arguments for tail call");
        return 0;
    }

    if (!kafka_allow_packet(&skb_info)) {
        return 0;
    }

    kafka_telemetry_t *kafka_tel = bpf_map_lookup_elem(&kafka_telemetry, &zero);
    if (kafka_tel == NULL) {
        return 0;
    }

    if (is_tcp_termination(&skb_info)) {
        kafka_tcp_termination(&tup);
        return 0;
    }

    pktbuf_t pkt = pktbuf_from_skb(skb, &skb_info);

    if (kafka_process_response(skb, &tup, kafka, pkt, &skb_info)) {
        return 0;
    }

    (void)kafka_process(&tup, kafka, pkt, kafka_tel);
    return 0;
}

SEC("uprobe/kafka_tls_filter")
int uprobe__kafka_tls_filter(struct pt_regs *ctx) {
    const __u32 zero = 0;

    kafka_info_t *kafka = bpf_map_lookup_elem(&kafka_heap, &zero);
    if (kafka == NULL) {
        return 0;
    }

    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return 0;
    }

    kafka_telemetry_t *kafka_tel = bpf_map_lookup_elem(&kafka_telemetry, &zero);
    if (kafka_tel == NULL) {
        return 0;
    }    

    // On stack for 4.14
    conn_tuple_t tup = args->tup;

    pktbuf_t pkt = pktbuf_from_tls(args);

    if (kafka_process_response(ctx, &tup, kafka, pkt, NULL)) {
        return 0;
    }

    kafka_process(&tup, kafka, pkt, kafka_tel);
    return 0;
}

SEC("uprobe/kafka_tls_termination")
int uprobe__kafka_tls_termination(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return 0;
    }

    // On stack for 4.14
    conn_tuple_t tup = args->tup;
    kafka_tcp_termination(&tup);

    return 0;
}

PKTBUF_READ_INTO_BUFFER(topic_name_parser, TOPIC_NAME_MAX_STRING_SIZE, BLK_SIZE)

static __always_inline void kafka_batch_enqueue_wrapper(kafka_info_t *kafka, conn_tuple_t *tup, kafka_transaction_t *transaction) {
    kafka_event_t *event = &kafka->event;

    bpf_memcpy(&event->tup, tup, sizeof(conn_tuple_t));
    normalize_tuple(&event->tup);

    if (transaction != &event->transaction) {
        bpf_memcpy(&event->transaction, transaction, sizeof(kafka_transaction_t));
    }

    kafka_batch_enqueue(event);
}

enum parse_result {
    // End of packet. This packet parsed successfully, but more data is needed
    // for the response to be completed.
    RET_EOP = 0,
    // Response parsed fully.
    RET_DONE = 1,
    // Error during processing response.
    RET_ERR = -1,
    // Ran out of iterations in the packet processing loop.
    RET_LOOP_END = -2,
};

// TCP segments splits can happen at any point in the response. If a
// field happens to straddles the segment boundary, we need to read
// some bytes from the old packet and the rest from the new packet.
static __always_inline enum parse_result read_with_remainder(kafka_response_context_t *response, pktbuf_t pkt,
                                                             u32 *offset, u32 data_end, s32 *val, bool first)
{
    if (*offset >= data_end) {
        // The offset we want to read is completely outside of the current
        // packet. No remainder to save, we just need to save the offset
        // at which we need to start reading the next packet from.
        response->carry_over_offset = *offset - data_end;
        return RET_EOP;
    }

    u32 avail = data_end - *offset;
    u32 remainder = response->remainder;
    u32 want = sizeof(s32);

    extra_debug("avail %u want %u remainder %u", avail, want, remainder);

    // Statically optimize away code for non-first iteration of loop since there
    // can be no intra-packet remainder.
    if (!first) {
        remainder = 0;
    }

    if (avail < want) {
        // We have less than 4 bytes left in the packet.

        if (remainder) {
            // We don't handle the case where we already have a remainder saved
            // and the new packet is so small that it doesn't allow us to fully
            // read the value we want to read. Actually we don't need to check
            // for 4 bytes but just enough bytes to fill the value, but in reality
            // packet sizes so small are highly unlikely so just check for 4 bytes.
            extra_debug("Continuation packet less than 4 bytes?");
            return RET_ERR;
        }

        // This is negative and so kafka_continue_parse_response() will save
        // remainder.
        response->carry_over_offset = *offset - data_end;
        return RET_EOP;
    }

    if (!remainder) {
        // No remainder, and 4 or more bytes more in the packet, so just
        // do a normal read.
        pktbuf_load_bytes(pkt, *offset, val, sizeof(*val));
        *offset += sizeof(*val);
        *val = bpf_ntohl(*val);
        extra_debug("read without remainder: %d", *val);
        return RET_DONE;
    }

    // We'll be using up the remainder so clear it.
    response->remainder = 0;

    // The remainder_buf contains up to 3 head bytes of the value we
    // need to read, saved from the previous packet. Read the tail
    // bytes of the value from the current packet and reconstruct
    // the value to be read.
    u8 *reconstruct = response->remainder_buf;
    u8 tail[4] = {0};

    pktbuf_load_bytes(pkt, *offset, &tail, 4);

    switch (remainder) {
    case 1:
        reconstruct[1] = tail[0];
        reconstruct[2] = tail[1];
        reconstruct[3] = tail[2];
        break;
    case 2:
        reconstruct[2] = tail[0];
        reconstruct[3] = tail[1];
        break;
    case 3:
        reconstruct[3] = tail[0];
        break;
    }

    *offset += want - remainder;
    *val = bpf_ntohl(*(u32 *)reconstruct);
    extra_debug("read with remainder: %d", *val);

    return RET_DONE;
}

static __always_inline enum parse_result read_with_remainder_s16(kafka_response_context_t *response, pktbuf_t pkt,
                                                             u32 *offset, u32 data_end, s16 *val, bool first)
{
    if (*offset >= data_end) {
        // The offset we want to read is completely outside of the current
        // packet. No remainder to save, we just need to save the offset
        // at which we need to start reading the next packet from.
        response->carry_over_offset = *offset - data_end;
        return RET_EOP;
    }

    u32 avail = data_end - *offset;
    u32 remainder = response->remainder;
    u32 want = sizeof(*val);

    extra_debug("avail %u want %u remainder %u", avail, want, remainder);

    // Statically optimize away code for non-first iteration of loop since there
    // can be no intra-packet remainder.
    if (!first) {
        remainder = 0;
    }

    if (avail < want) {
        // We have less than 2 bytes left in the packet.

        if (remainder) {
            // We don't handle the case where we already have a remainder saved
            // and the new packet is so small that it doesn't allow us to fully
            // read the value we want to read. Actually we don't need to check
            // for 2 bytes but just enough bytes to fill the value, but in reality
            // packet sizes so small are highly unlikely so just check for 2 bytes.
            extra_debug("Continuation packet less than 2 bytes?");
            return RET_ERR;
        }

        // This is negative and so kafka_continue_parse_response() will save
        // remainder.
        response->carry_over_offset = *offset - data_end;
        return RET_EOP;
    }

    if (!remainder) {
        // No remainder, and 2 or more bytes more in the packet, so just
        // do a normal read.
        pktbuf_load_bytes(pkt, *offset, val, sizeof(*val));
        *offset += sizeof(*val);
        *val = bpf_ntohs(*val);
        extra_debug("read without remainder: %d", *val);
        return RET_DONE;
    }

    // We'll be using up the remainder so clear it.
    response->remainder = 0;

    // The remainder_buf contains up to 1 head bytes of the value we
    // need to read, saved from the previous packet. Read the tail
    // bytes of the value from the current packet and reconstruct
    // the value to be read.
//    u8 *reconstruct = response->remainder_buf_s16;
    u8 *reconstruct = response->remainder_buf;
    u8 tail[2] = {0};

    pktbuf_load_bytes(pkt, *offset, &tail, 1);

    switch (remainder) {
    case 1:
        reconstruct[1] = tail[0];
        break;
    }

    *offset += want - remainder;
    *val = bpf_ntohs(*(u16 *)reconstruct);
    extra_debug("read with remainder: %d", *val);

    return RET_DONE;
}

static __always_inline enum parse_result kafka_continue_parse_response_loop(kafka_info_t *kafka,
                                                                            conn_tuple_t *tup,
                                                                            kafka_response_context_t *response,
                                                                            pktbuf_t pkt, u32 offset,
                                                                            u32 data_end)
{
    u32 orig_offset = offset;
    kafka_transaction_t *request = &response->transaction;
    enum parse_result ret;

    extra_debug("carry_over_offset %d", response->carry_over_offset);

    if (response->carry_over_offset < 0) {
        return RET_ERR;
    }

    offset += response->carry_over_offset;
    response->carry_over_offset = 0;

    if (response->state == KAFKA_FETCH_RESPONSE_START) {
         if (request->request_api_version >= 1) {
            offset += sizeof(s32); // Skip throttle_time_ms
        }
        if (request->request_api_version >= 7) {
            offset += sizeof(s16); // Skip error_code
            offset += sizeof(s32); // Skip session_id
        }

        PKTBUF_READ_BIG_ENDIAN_WRAPPER(s32, num_topics, pkt, offset);
        if (num_topics <= 0) {
            return RET_ERR;
        }

        PKTBUF_READ_BIG_ENDIAN_WRAPPER(s16, topic_name_size, pkt, offset);
        if (topic_name_size <= 0 || topic_name_size > TOPIC_NAME_MAX_ALLOWED_SIZE) {
            return RET_ERR;
        }

        // Should we check that topic name matches the topic we expect?
        offset += topic_name_size;

        PKTBUF_READ_BIG_ENDIAN_WRAPPER(s32, number_of_partitions, pkt, offset);
        if (number_of_partitions <= 0) {
            return RET_ERR;
        }

        response->partitions_count = number_of_partitions;
        response->state = KAFKA_FETCH_RESPONSE_PARTITION_START;
        response->record_batches_num_bytes = 0;
        response->record_batch_length = 0;
    }

#pragma unroll(KAFKA_RESPONSE_PARSER_MAX_ITERATIONS)
    for (int i = 0; i < KAFKA_RESPONSE_PARSER_MAX_ITERATIONS; i++) {
        bool first = i == 0;

        extra_debug("state: %d", response->state);
        switch (response->state) {
        case KAFKA_FETCH_RESPONSE_START:
            // Never happens. Only present to supress a compiler warning.
            break;
        case KAFKA_FETCH_RESPONSE_PARTITION_START:
            offset += sizeof(s32); // Skip partition_index
            response->state = KAFKA_FETCH_RESPONSE_PARTITION_ERROR_CODE_START;
            // fallthrough

         case KAFKA_FETCH_RESPONSE_PARTITION_ERROR_CODE_START:
         {
            // Error codes range from -1 to 119 as per the Kafka protocol specification.
            // For details, refer to: https://kafka.apache.org/protocol.html#protocol_error_codes
            s16 error_code = 0;
            ret = read_with_remainder_s16(response, pkt, &offset, data_end, &error_code, first);
            if (ret != RET_DONE) {
                return ret;
            }
            if (error_code < -1 || error_code > 119) {
                log_debug("kafka: invalid error code: %d", error_code);
                return RET_ERR;
            }
            log_debug("kafka: Got error code: %d", error_code);
            response->transaction.error_code = (s8)error_code;

            offset += sizeof(s64); // Skip high_watermark

            if (request->request_api_version >= 4) {
                offset += sizeof(s64); // Skip last_stable_offset

                if (request->request_api_version >= 5) {
                    offset += sizeof(s64); // log_start_offset
                }
            }

            response->state = KAFKA_FETCH_RESPONSE_PARTITION_ABORTED_TRANSACTIONS;
            // fallthrough
            }

        case KAFKA_FETCH_RESPONSE_PARTITION_ABORTED_TRANSACTIONS:
            if (request->request_api_version >= 4) {
                s32 aborted_transactions = 0;
                ret = read_with_remainder(response, pkt, &offset, data_end, &aborted_transactions, first);
                if (ret != RET_DONE) {
                    return ret;
                }

                // Note that -1 is a valid value which means that the list is empty.
                if (aborted_transactions < -1) {
                    return RET_ERR;
                }
                // If we interpret some junk data as a packet with a huge aborted_transactions,
                // we could end up missing up a lot of future response processing since we
                // would wait for the end of the aborted_transactions list. So add a limit
                // as a heuristic.
                if (aborted_transactions >= KAFKA_MAX_ABORTED_TRANSACTIONS) {
                    extra_debug("Possibly invalid aborted_transactions %d", aborted_transactions);
                    return RET_ERR;
                }
                if (aborted_transactions >= 0) {
                    // producer_id and first_offset in each aborted transaction
                    offset += sizeof(s64) * 2 * aborted_transactions;
                }

                if (request->request_api_version >= 11) {
                    offset += sizeof(s32); // preferred_read_replica
                }
            }

            response->state = KAFKA_FETCH_RESPONSE_RECORD_BATCHES_ARRAY_START;
            // fallthrough

        case KAFKA_FETCH_RESPONSE_RECORD_BATCHES_ARRAY_START:
            ret = read_with_remainder(response, pkt, &offset, data_end, &response->record_batches_num_bytes, first);
            if (ret != RET_DONE) {
                return ret;
            }

            extra_debug("record_batches_num_bytes: %d", response->record_batches_num_bytes);

            if (response->record_batches_num_bytes == 0) {
                response->state = KAFKA_FETCH_RESPONSE_PARTITION_END;
                break;
            }

            response->state = KAFKA_FETCH_RESPONSE_RECORD_BATCH_START;
            // fallthrough

        case KAFKA_FETCH_RESPONSE_RECORD_BATCH_START:
            offset += sizeof(s64); // baseOffset
            response->state = KAFKA_FETCH_RESPONSE_RECORD_BATCH_LENGTH;
            // fallthrough

        case KAFKA_FETCH_RESPONSE_RECORD_BATCH_LENGTH:
            ret = read_with_remainder(response, pkt, &offset, data_end, &response->record_batch_length, first);
            if (ret != RET_DONE) {
                return ret;
            }

            extra_debug("batchLength %d", response->record_batch_length);
            if (response->record_batch_length <= 0) {
                extra_debug("batchLength too small %d", response->record_batch_length);
                return RET_ERR;
            }
            // The batchLength excludes the baseOffset (u64) and the batchLength (s32) itself,
            // so those need to be be added separately.
            if (response->record_batch_length + sizeof(s32) + sizeof(u64) > response->record_batches_num_bytes) {
                extra_debug("batchLength too large %d (record_batches_num_bytes: %d)", response->record_batch_length,
                            response->record_batches_num_bytes);

                // Kafka fetch responses can have some partial, unparseable records in the record
                // batch block which are truncated due to the maximum response size specified in
                // the request.  If there are no more partitions left, assume we've reached such
                // a block and report what we have.
                if (response->transaction.records_count > 0 && response->partitions_count == 1) {
                    extra_debug("assuming truncated data due to maxsize");
                    response->record_batch_length = 0;
                    response->record_batches_num_bytes = 0;
                    response->state = KAFKA_FETCH_RESPONSE_PARTITION_END;
                    continue;
                }

                extra_debug("assuming corrupt packet");
                return RET_ERR;
            }

            offset += sizeof(s32); // Skip partitionLeaderEpoch
            response->state = KAFKA_FETCH_RESPONSE_RECORD_BATCH_MAGIC;
            // fallthrough

        case KAFKA_FETCH_RESPONSE_RECORD_BATCH_MAGIC:
            if (offset + sizeof(s8) > data_end) {
                response->carry_over_offset = offset - data_end;
                return RET_EOP;
            }

            PKTBUF_READ_BIG_ENDIAN_WRAPPER(s8, magic, pkt, offset);
            if (magic != 2) {
                extra_debug("Invalid magic byte");
                return RET_ERR;
            }

            offset += sizeof(u32); // Skipping crc
            offset += sizeof(s16); // Skipping attributes
            offset += sizeof(s32); // Skipping last offset delta
            offset += sizeof(s64); // Skipping base timestamp
            offset += sizeof(s64); // Skipping max timestamp
            offset += sizeof(s64); // Skipping producer id
            offset += sizeof(s16); // Skipping producer epoch
            offset += sizeof(s32); // Skipping base sequence
            response->state = KAFKA_FETCH_RESPONSE_RECORD_BATCH_RECORDS_COUNT;
            // fallthrough

        case KAFKA_FETCH_RESPONSE_RECORD_BATCH_RECORDS_COUNT:
            {
                s32 records_count = 0;
                ret = read_with_remainder(response, pkt, &offset, data_end, &records_count, first);
                if (ret != RET_DONE) {
                    return ret;
                }

                extra_debug("records_count: %d", records_count);
                if (records_count <= 0) {
                    extra_debug("Invalid records count: %d", records_count);
                    return RET_ERR;
                }

                // All the records have to fit inside the record batch, so guard against
                // unreasonable values in corrupt packets.
                if (records_count >= response->record_batch_length) {
                    extra_debug("Bogus records count %d (batch_length %d)",
                                records_count, response->record_batch_length);
                    return RET_ERR;
                }

                response->transaction.records_count += records_count;
            }

            offset += response->record_batch_length
            - sizeof(s32) // Skip partitionLeaderEpoch
            - sizeof(s8) // Skipping magic
            - sizeof(u32) // Skipping crc
            - sizeof(s16) // Skipping attributes
            - sizeof(s32) // Skipping last offset delta
            - sizeof(s64) // Skipping base timestamp
            - sizeof(s64) // Skipping max timestamp
            - sizeof(s64) // Skipping producer id
            - sizeof(s16) // Skipping producer epoch
            - sizeof(s32) // Skipping base sequence
            - sizeof(s32); // Skipping records count
            response->state = KAFKA_FETCH_RESPONSE_RECORD_BATCH_END;
            // fallthrough

        case KAFKA_FETCH_RESPONSE_RECORD_BATCH_END:
            if (offset > data_end) {
                response->carry_over_offset = offset - data_end;
                return RET_EOP;
            }

            // Record batch batchLength does not include batchOffset and batchLength.
            response->record_batches_num_bytes -= response->record_batch_length + sizeof(u32) + sizeof(u64);
            response->record_batch_length = 0;

            if (response->record_batches_num_bytes > 0) {
                response->state = KAFKA_FETCH_RESPONSE_RECORD_BATCH_START;
                break;
            }

            response->state = KAFKA_FETCH_RESPONSE_PARTITION_END;
            // fallthrough

        case KAFKA_FETCH_RESPONSE_PARTITION_END:
            response->partitions_count--;
            if (response->partitions_count == 0) {
                extra_debug("enqueue final partition, records_count %d",  response->transaction.records_count);
                kafka_batch_enqueue_wrapper(kafka, tup, &response->transaction);
                return RET_DONE;
            } else {
                extra_debug("enqueue partition, #partitions left %d, records_count %d",
                    response->partitions_count,
                    response->transaction.records_count);
                kafka_batch_enqueue_wrapper(kafka, tup, &response->transaction);
                // Reset records count for the next partition, so we won't be double counting.
                response->transaction.records_count = 0;
            }

            response->state = KAFKA_FETCH_RESPONSE_PARTITION_START;
            break;
        }
    }

    // We should have exited at KAFKA_FETCH_RESPONSE_PARTITION_END if we
    // managed to parse the entire packet, so if we get here we still have
    // more to go. Remove the skb_info.data_off so that this function can
    // be called again on the same packet with the same arguments in a tail
    // call.
    response->carry_over_offset = offset - orig_offset;
    return RET_LOOP_END;
}

static __always_inline enum parse_result kafka_continue_parse_response(kafka_info_t *kafka,
                                                                       conn_tuple_t *tup,
                                                                       kafka_response_context_t *response,
                                                                       pktbuf_t pkt, u32 offset,
                                                                       u32 data_end)
{
    enum parse_result ret;

    ret = kafka_continue_parse_response_loop(kafka, tup, response, pkt, offset, data_end);
    if (ret != RET_EOP) {
        return ret;
    }

    // carry_over_offset is negative when the paket ended in the middle of a
    // 4 byte value that we wanted to read, so we we need to save the appropriate
    // number of byte to be able to reconstruct the value when we receive the
    // rest of the bytes in the next packet. See read_with_remainder().
    if (response->carry_over_offset < 0) {
        extra_debug("Saving remainder %d", response->carry_over_offset);

        switch (response->carry_over_offset) {
        case -1:
            pktbuf_load_bytes(pkt, data_end - 1, &response->remainder_buf, 1);
            break;
        case -2:
            pktbuf_load_bytes(pkt, data_end - 2, &response->remainder_buf, 2);
            break;
        case -3:
            pktbuf_load_bytes(pkt, data_end - 3, &response->remainder_buf, 3);
            break;
        default:
            // read_with_remainder() only reads 4 byte values, so the remainder
            // can never be more than 3.
            return RET_ERR;
        }

        response->remainder = -1 * response->carry_over_offset;
        // We shouldn't be skipping any part of the new packet.
        response->carry_over_offset = 0;
    }

    return ret;
}

static __always_inline void kafka_call_response_parser(void *ctx, conn_tuple_t *tup, pktbuf_t pkt)
{
    switch (pkt.type) {
    case PKTBUF_SKB:
        bpf_tail_call_compat(ctx, &protocols_progs, PROG_KAFKA_RESPONSE_PARSER);
        break;
    case PKTBUF_TLS:
        bpf_tail_call_compat(ctx, &tls_process_progs, TLS_KAFKA_RESPONSE_PARSER);
        break;
    }

    // The only reason we would get here if the tail call failed due to too
    // many tail calls.
    extra_debug("failed to call response parser");
    bpf_map_delete_elem(&kafka_response, tup);
}

static __always_inline void kafka_response_parser(kafka_info_t *kafka, void *ctx, conn_tuple_t *tup, pktbuf_t pkt) {
    kafka_response_context_t *response = bpf_map_lookup_elem(&kafka_response, tup);
    if (!response) {
        return;
    }

    u32 data_off = pktbuf_data_offset(pkt);
    u32 data_end = pktbuf_data_end(pkt);

    enum parse_result result = kafka_continue_parse_response(kafka, tup, response, pkt,
                                                             data_off, data_end);
    switch (result) {
    case RET_EOP:
        // This packet parsed successfully but more data needed, nothing
        // more to do for now.
        break;
    case RET_ERR:
        // Error during processing response continuation, abandon this
        // response.
        bpf_map_delete_elem(&kafka_response, tup);
        break;
    case RET_DONE:
        // Response parsed fully.
        bpf_map_delete_elem(&kafka_response, tup);
        break;
    case RET_LOOP_END:
        // We ran out of iterations in the loop, but we're not done
        // processing this packet, so continue in a self tail call.
        kafka_call_response_parser(ctx, tup, pkt);

        // If we failed (due to exceeding tail calls), at least flush what
        // we have.
        if (response->transaction.records_count) {
            extra_debug("enqueue (loop exceeded), records_count %d", response->transaction.records_count);
            kafka_batch_enqueue_wrapper(kafka, tup, &response->transaction);
        }
        break;
    }
}

SEC("socket/kafka_response_parser")
int socket__kafka_response_parser(struct __sk_buff *skb) {
    const __u32 zero = 0;
    kafka_info_t *kafka = bpf_map_lookup_elem(&kafka_heap, &zero);
    if (kafka == NULL) {
        return 0;
    }

    skb_info_t skb_info;
    conn_tuple_t tup;
    if (!fetch_dispatching_arguments(&tup, &skb_info)) {
        return 0;
    }

    kafka_response_parser(kafka, skb, &tup, pktbuf_from_skb(skb, &skb_info));

    return 0;
}

SEC("uprobe/kafka_tls_response_parser")
int uprobe__kafka_tls_response_parser(struct pt_regs *ctx) {
    const __u32 zero = 0;
    kafka_info_t *kafka = bpf_map_lookup_elem(&kafka_heap, &zero);
    if (kafka == NULL) {
        return 0;
    }

    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return 0;
    }

    // Put tuple on stack for 4.14.
    conn_tuple_t tup = args->tup;
    kafka_response_parser(kafka, ctx, &tup, pktbuf_from_tls(args));

    return 0;
}

// Gets the next expected TCP sequence in the stream, assuming
// no retransmits and out-of-order segments.
static __always_inline u32 kafka_get_next_tcp_seq(skb_info_t *skb_info) {
    if (!skb_info) {
        return 0;
    }

    u32 data_len = skb_info->data_end - skb_info->data_off;
    u32 next_seq = skb_info->tcp_seq + data_len;

    return next_seq;
}

static __always_inline bool kafka_process_new_response(void *ctx, conn_tuple_t *tup, kafka_info_t *kafka, pktbuf_t pkt, skb_info_t *skb_info) {
    u32 pktlen = pktbuf_data_end(pkt) - pktbuf_data_offset(pkt);
    u32 offset = pktbuf_data_offset(pkt);
    u32 orig_offset = offset;

    // Usually the first packet containts the message size, correlation ID, and the first
    // fields of the headers up to the partirtion start. However, with TLS, each read from
    // user space will arrive as a separate "packet".
    //
    // In theory the program can read even one byte at a time, but since supporting arbitrary
    // sizes costs instructions we assume some common cases:
    //
    // (a) message size (4 bytes) read first, then rest of the packet read (eg. franz-go)
    // (b) message size and correlation id (8 byte) read first, then rest of the packet (eg. librdkafka)
    // (c) message size read first (4), then correlation id (4), then rest of the packet (eg. kafka-go)
    // (d) message size, correlation ID, and first headers read together (eg. non-TLS)
    //
    // There could be some false positives due to this, if the message size happens to match
    // a valid in-flight correlation ID.

    if (pkt.type != PKTBUF_TLS || pktlen >= 8) {
        offset += sizeof(__s32); // Skip message size
    }

    PKTBUF_READ_BIG_ENDIAN_WRAPPER(s32, correlation_id, pkt, offset);

    extra_debug("pktlen %u correlation_id: %d", pktlen, correlation_id);

    kafka_transaction_key_t key = {};
    key.correlation_id = correlation_id;
    bpf_memcpy(&key.tuple, tup, sizeof(key.tuple));
    kafka_transaction_t *request = bpf_map_lookup_elem(&kafka_in_flight, &key);
    if (!request) {
        if (pkt.type == PKTBUF_TLS && pktlen >= 8) {
            // Try reading the first value, in case it's case (a) or (c)
            offset = orig_offset;
            PKTBUF_READ_BIG_ENDIAN_WRAPPER(s32, correlation_id2, pkt, offset);
            key.correlation_id = correlation_id2;

            extra_debug("correlation_id2: %d", correlation_id2);
            request = bpf_map_lookup_elem(&kafka_in_flight, &key);
        }

        if (!request) {
            return false;
        }
    }

    extra_debug("Received response for request with correlation id %d", correlation_id);

    kafka->response.transaction = *request;
    bpf_map_delete_elem(&kafka_in_flight, &key);

    kafka->response.state = KAFKA_FETCH_RESPONSE_START;
    kafka->response.carry_over_offset = offset - orig_offset;
    kafka->response.expected_tcp_seq = kafka_get_next_tcp_seq(skb_info);

    // Copy it to the stack since the verifier on 4.14 complains otherwise.
    kafka_response_context_t response_ctx;
    bpf_memcpy(&response_ctx, &kafka->response, sizeof(response_ctx));

    bpf_map_update_elem(&kafka_response, tup, &response_ctx, BPF_ANY);

    kafka_call_response_parser(ctx, tup, pkt);
    return true;
}

static __always_inline bool kafka_process_response(void *ctx, conn_tuple_t *tup, kafka_info_t *kafka, pktbuf_t pkt, skb_info_t *skb_info) {
    kafka_response_context_t *response = bpf_map_lookup_elem(&kafka_response, tup);
    if (response) {
        if (!skb_info || skb_info->tcp_seq == response->expected_tcp_seq) {
            response->expected_tcp_seq = kafka_get_next_tcp_seq(skb_info);
            kafka_call_response_parser(ctx, tup, pkt);
            // It's on the response path, so no need to parser as a request.
            return true;
        }

        // When the sequence number is greater than the end of the earlier
        // segment, we don't know for sure if we saw all the older data since
        // segments in between the previous and the current one could have
        // been lost. But since we anyway don't do reassembly we can't
        // handle such out-of-order segments properly if they arrive later.
        // So just drop all older segments here since it helps on systems
        // where groups of packets (a couple of TCP segments) are seen to
        // often be duplicated.
        //
        // The comparison is done this way to handle wraparound of sequence numbers.
        s32 diff = skb_info->tcp_seq - response->expected_tcp_seq;
        if (diff < 0) {
            extra_debug("skip old TCP segment");
            // It's on the response path, so no need to parser as a request.
            return true;
        }

        // The segment is not old, but it is not the next one we were expecting.
        // No point in parsing this as a response continuation since it may
        // yield bogus values. Flush what we have and forget about this current
        // response.
        extra_debug("lost response TCP segments, expected %u got %u",
                    response->expected_tcp_seq,
                    skb_info->tcp_seq);

        if (response->transaction.records_count) {
            extra_debug("enqueue (broken stream), records_count %d", response->transaction.records_count);
            kafka_batch_enqueue_wrapper(kafka, tup, &response->transaction);
        }

        bpf_map_delete_elem(&kafka_response, tup);
        // Try to parse it as a new response.
    }

    return kafka_process_new_response(ctx, tup, kafka, pkt, skb_info);
}

static __always_inline bool kafka_process(conn_tuple_t *tup, kafka_info_t *kafka, pktbuf_t pkt, kafka_telemetry_t *kafka_tel) {
    /*
        We perform Kafka request validation as we can get kafka traffic that is not relevant for parsing (unsupported requests, responses, etc)
    */

    u32 offset = pktbuf_data_offset(pkt);
    kafka_transaction_t *kafka_transaction = &kafka->event.transaction;
    kafka_header_t kafka_header;
    bpf_memset(&kafka_header, 0, sizeof(kafka_header));
    pktbuf_load_bytes_with_telemetry(pkt, offset, (char *)&kafka_header, sizeof(kafka_header));
    kafka_header.message_size = bpf_ntohl(kafka_header.message_size);
    kafka_header.api_key = bpf_ntohs(kafka_header.api_key);
    kafka_header.api_version = bpf_ntohs(kafka_header.api_version);
    kafka_header.correlation_id = bpf_ntohl(kafka_header.correlation_id);
    kafka_header.client_id_size = bpf_ntohs(kafka_header.client_id_size);

    log_debug("kafka: kafka_header.api_key: %d api_version: %d", kafka_header.api_key, kafka_header.api_version);

    if (!is_valid_kafka_request_header(&kafka_header)) {
        return false;
    }

    kafka_transaction->request_started = bpf_ktime_get_ns();
    kafka_transaction->request_api_key = kafka_header.api_key;
    kafka_transaction->request_api_version = kafka_header.api_version;

    offset += sizeof(kafka_header_t);

    // Validate client ID
    // Client ID size can be equal to '-1' if the client id is null.
    if (kafka_header.client_id_size > 0) {
        if (!is_valid_client_id(pkt, offset, kafka_header.client_id_size)) {
            return false;
        }
        offset += kafka_header.client_id_size;
    } else if (kafka_header.client_id_size < -1) {
        return false;
    }

    switch (kafka_header.api_key) {
    case KAFKA_PRODUCE:
        if (!get_topic_offset_from_produce_request(&kafka_header, pkt, &offset)) {
            return false;
        }
        break;
    case KAFKA_FETCH:
        offset += get_topic_offset_from_fetch_request(&kafka_header);
        break;
    default:
        return false;
    }

    // Skipping number of entries for now
    offset += sizeof(s32);
    PKTBUF_READ_BIG_ENDIAN_WRAPPER(s16, topic_name_size, pkt, offset);
    if (topic_name_size <= 0 || topic_name_size > TOPIC_NAME_MAX_ALLOWED_SIZE) {
        // Since topic_name_size doesn't adhere to the protocol, there's no need to update our telemetry, as it's not a valid Kafka field
        return false;
    }
    update_topic_name_size_telemetry(kafka_tel, topic_name_size);
    bpf_memset(kafka_transaction->topic_name, 0, TOPIC_NAME_MAX_STRING_SIZE);
    pktbuf_read_into_buffer_topic_name_parser((char *)kafka_transaction->topic_name, pkt, offset);
    offset += topic_name_size;
    kafka_transaction->topic_name_size = topic_name_size;

    CHECK_STRING_COMPOSED_OF_ASCII_FOR_PARSING(TOPIC_NAME_MAX_STRING_SIZE_TO_VALIDATE, topic_name_size, kafka_transaction->topic_name);

    log_debug("kafka: topic name is %s", kafka_transaction->topic_name);

    switch (kafka_header.api_key) {
    case KAFKA_PRODUCE:
    {
        PKTBUF_READ_BIG_ENDIAN_WRAPPER(s32, number_of_partitions, pkt, offset);
        if (number_of_partitions <= 0) {
            return false;
        }
        if (number_of_partitions > 1) {
            log_debug("Multiple partitions detected in produce request, current support limited to requests with a single partition");
            return false;
        }
        offset += sizeof(s32); // Skipping Partition ID

        // Parsing the message set of the partition.
        // It's assumed that the message set is not in the old message format, as the format differs:
        // https://kafka.apache.org/documentation/#messageset
        // The old message format predates Kafka 0.11, released on September 27, 2017.
        // It's unlikely for us to encounter these older versions in practice.

        offset += sizeof(s32); // Skipping record batch (message set in wireshark) size in bytes
        offset += sizeof(s64); // Skipping record batch baseOffset
        offset += sizeof(s32); // Skipping record batch batchLength
        offset += sizeof(s32); // Skipping record batch partitionLeaderEpoch
        PKTBUF_READ_BIG_ENDIAN_WRAPPER(s8, magic_byte, pkt, offset);
        if (magic_byte != 2) {
            log_debug("Got magic byte != 2, the protocol state it should be 2");
            return false;
        }
        offset += sizeof(u32); // Skipping crc
        offset += sizeof(s16); // Skipping attributes
        offset += sizeof(s32); // Skipping last offset delta
        offset += sizeof(s64); // Skipping base timestamp
        offset += sizeof(s64); // Skipping max timestamp
        offset += sizeof(s64); // Skipping producer id
        offset += sizeof(s16); // Skipping producer epoch
        offset += sizeof(s32); // Skipping base sequence
        PKTBUF_READ_BIG_ENDIAN_WRAPPER(s32, records_count, pkt, offset);
        if (records_count <= 0) {
            log_debug("Got number of Kafka produce records <= 0");
            return false;
        }
        kafka_transaction->records_count = records_count;
        break;
    }
    case KAFKA_FETCH:
        // Filled in when the response is parsed.
        kafka_transaction->records_count = 0;
        break;
    default:
        return false;
     }

    if (kafka_header.api_key == KAFKA_FETCH) {
        // Copy to stack required by 4.14 verifier.
        kafka_transaction_t transaction;
        kafka_transaction_key_t key;
        bpf_memset(&key, 0, sizeof(key));
        bpf_memcpy(&transaction, kafka_transaction, sizeof(transaction));
        key.correlation_id = kafka_header.correlation_id;
        bpf_memcpy(&key.tuple, tup, sizeof(key.tuple));
        // Flip the tuple for the response path.
        flip_tuple(&key.tuple);
        bpf_map_update_elem(&kafka_in_flight, &key, &transaction, BPF_NOEXIST);
        return true;
    }

    kafka_batch_enqueue_wrapper(kafka, tup, kafka_transaction);
    return true;
}

// this function is called by the socket-filter program to decide whether or not we should inspect
// the contents of a certain packet, in order to avoid the cost of processing packets that are not
// of interest such as empty ACKs.
static __always_inline bool kafka_allow_packet(skb_info_t *skb_info) {
    // if payload data is empty, we only process it if the packet represents a TCP termination
    if (is_payload_empty(skb_info)) {
        return skb_info->tcp_flags&(TCPHDR_FIN|TCPHDR_RST);
    }

    return true;
}

// update_path_size_telemetry updates the topic name size telemetry.
static __always_inline void update_topic_name_size_telemetry(kafka_telemetry_t *kafka_tel, __u64 size) {
    // We have 10 buckets in the ranges of: 1 - 10, 11 - 20, ... , 71 - 80, 81 - 90, 91 - 100, 101 - 255
    __u8 bucket_idx = (size - 1) / KAFKA_TELEMETRY_TOPIC_NAME_BUCKET_SIZE;

    // Ensure that the bucket index falls within the valid range.
    bucket_idx = bucket_idx < 0 ? 0 : bucket_idx;
    bucket_idx = bucket_idx > (KAFKA_TELEMETRY_TOPIC_NAME_NUM_OF_BUCKETS - 1) ? (KAFKA_TELEMETRY_TOPIC_NAME_NUM_OF_BUCKETS - 1) : bucket_idx;

    __sync_fetch_and_add(&kafka_tel->topic_name_size_buckets[bucket_idx], 1);
}

#endif
