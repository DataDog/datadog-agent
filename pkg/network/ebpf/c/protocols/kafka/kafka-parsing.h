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

    kafka->event.transaction.tags = NO_TAGS;
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

    pktbuf_t pkt = pktbuf_from_tls(ctx, args);
    kafka->event.transaction.tags = (__u8)args->tags;
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

struct read_with_remainder_config {
    u32 want_bytes;
    void (*convert)(void *dest, void *src);
};

static __always_inline enum parse_result __read_with_remainder(struct read_with_remainder_config config,
                                                               kafka_response_context_t *response, pktbuf_t pkt,
                                                               u32 *offset, u32 data_end, void *val, bool first)
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
    u32 want = config.want_bytes;

    extra_debug("avail %u want %u remainder %u", avail, want, remainder);

    // Statically optimize away code for non-first iteration of loop since there
    // can be no intra-packet remainder.
    if (!first) {
        remainder = 0;
    }

    if (avail < want) {
        // We have less than `want` bytes left in the packet.

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
        pktbuf_load_bytes(pkt, *offset, val, want);
        *offset += want;
        config.convert(val, val);
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

    pktbuf_load_bytes(pkt, *offset, &tail, want);

    switch (remainder) {
    case 1:
        reconstruct[1] = tail[0];
        if (want > 2) {
            reconstruct[2] = tail[1];
            reconstruct[3] = tail[2];
        }
        break;
    case 2:
        if (want > 2) {
            reconstruct[2] = tail[0];
            reconstruct[3] = tail[1];
        }
        break;
    case 3:
        if (want > 2) {
            reconstruct[3] = tail[0];
        }
        break;
    }

    *offset += want - remainder;
    config.convert(val, reconstruct);

    return RET_DONE;
}

static __always_inline void convert_u16(void *dest, void *src)
{
    u16 *dest16 = dest;
    u16 *src16 = src;

    *dest16 = bpf_ntohs(*src16);

    if (src == dest) {
        extra_debug("read without remainder: %u", *dest16);
    } else {
        extra_debug("read with remainder: %u", *dest16);
    }
}

static __always_inline enum parse_result read_with_remainder_s16(kafka_response_context_t *response, pktbuf_t pkt,
                                                             u32 *offset, u32 data_end, s16 *val, bool first)
{
    struct read_with_remainder_config config = {
        .want_bytes = sizeof(u16),
        .convert = convert_u16,
    };

    return __read_with_remainder(config, response, pkt, offset, data_end, val, first);
}

static __always_inline void convert_u32(void *dest, void *src)
{
    u32 *dest32 = dest;
    u32 *src32 = src;

    *dest32 = bpf_ntohl(*src32);

    if (src == dest) {
        extra_debug("read without remainder: %u", *dest32);
    } else {
        extra_debug("read with remainder: %u", *dest32);
    }
}

static __always_inline enum parse_result read_with_remainder(kafka_response_context_t *response, pktbuf_t pkt,
                                                             u32 *offset, u32 data_end, s32 *val, bool first)
{
    struct read_with_remainder_config config = {
        .want_bytes = sizeof(u32),
        .convert = convert_u32,
    };

    return __read_with_remainder(config, response, pkt, offset, data_end, val, first);
}

// Parses varints, based on:
// https://stackoverflow.com/questions/19758270/read-varint-from-linux-sockets
// The specification for Kafka Unsigned Varints can be found here:
// https://cwiki.apache.org/confluence/display/KAFKA/KIP-482%3A+The+Kafka+Protocol+should+Support+Optional+Tagged+Fields
//
// The varints can actually up to 10 bytes long but we only support up to
// max_bytes length due to code size limitations.
static __always_inline enum parse_result read_varint(kafka_response_context_t *response,
                                                    pktbuf_t pkt, u64 *out, u32 *offset,
                                                    u32 data_end,
                                                    bool first,
                                                    u32 max_bytes)
{
    uint32_t shift_amount = 0;
    uint64_t value = 0;
    uint32_t i = 0;
    uint32_t startpos = 0;

    if (response != NULL && first) {
        value = response->varint_value;
        startpos = response->varint_position;
        shift_amount = startpos * 7;

        extra_debug("varint continue pos %d value %lld", startpos, value);

        response->varint_value = 0;
        response->varint_position = 0;
    }

    u8 current_byte = 0;

    #pragma unroll
    for (; i < max_bytes; i++) {
        // This check works better than setting i = startpos initially which leads
        // to complaints from the verifier about too much complexity.
        if (i < startpos) {
            continue;
        }

        if (*offset >= data_end) {
            extra_debug("varint break pos %d value %lld", i, value);
            if (response != NULL) {
                response->varint_position = i;
                response->varint_value = value;
                response->carry_over_offset = *offset - data_end;
            }
            return RET_EOP;
        }

        pktbuf_load_bytes(pkt, *offset, &current_byte, sizeof(current_byte));
        *offset += sizeof(current_byte);

        value |= (uint64_t)(current_byte & 0x7F) << shift_amount;
        shift_amount += 7;

        if (!isMSBSet(current_byte)) {
            break;
        }
    }

    if ((i == max_bytes - 1) && isMSBSet(current_byte)) {
        // The last byte in the unsigned varint contains a continuation bit,
        // this shouldn't happen if MAX_VARINT_BYTES = 10, but if it is lesser,
        // then we could be hitting a number we don't support.
        return RET_ERR;
    }

    // When lengths are stored as varints in the protocol, they are always
    // stored as N + 1.
    *out = value - 1;
    return RET_DONE;
}

static __always_inline enum parse_result read_varint_or_s16(
                                                            bool flexible,
                                                            kafka_response_context_t *response,
                                                            pktbuf_t pkt,
                                                            u32 *offset,
                                                            u32 data_end,
                                                            s64 *val,
                                                            bool first,
                                                            u32 max_varint_bytes)
{
    enum parse_result ret;

    if (flexible) {
        u64 tmp = 0;
        ret = read_varint(response, pkt, &tmp, offset, data_end, first, max_varint_bytes);
        *val = tmp;
    } else {
        u16 tmp = 0;
        ret = read_with_remainder_s16(response, pkt, offset, data_end, &tmp, first);
        *val = tmp;
    }

    return ret;
}

static __always_inline enum parse_result read_varint_or_s32(
                                                            bool flexible,
                                                            kafka_response_context_t *response,
                                                            pktbuf_t pkt,
                                                            u32 *offset,
                                                            u32 data_end,
                                                            s64 *val,
                                                            bool first,
                                                            u32 max_varint_bytes)
{
    enum parse_result ret;

    if (flexible) {
        u64 tmp = 0;
        ret = read_varint(response, pkt, &tmp, offset, data_end, first, max_varint_bytes);
        *val = tmp;
    } else {
        s32 tmp = 0;
        ret = read_with_remainder(response, pkt, offset, data_end, &tmp, first);
        *val = tmp;
    }

    return ret;
}

static __always_inline enum parse_result skip_tagged_fields(kafka_response_context_t *response,
                                                            pktbuf_t pkt,
                                                            u32 *offset,
                                                            u32 data_end,
                                                            bool verify)
{
    if (*offset >= data_end) {
        response->carry_over_offset = *offset - data_end;
        return RET_EOP;
    }

    if (verify) {
        u8 num_tagged_fields = 0;

        pktbuf_load_bytes(pkt, *offset, &num_tagged_fields, 1);
        extra_debug("num_tagged_fields: %u", num_tagged_fields);

        if (num_tagged_fields != 0) {
            // We don't support parsing tagged fields for now.
            return RET_ERR;
        }
    }

    *offset += 1;

    return RET_DONE;
}

enum parser_level {
    PARSER_LEVEL_PARTITION,
    PARSER_LEVEL_RECORD_BATCH,
};

static enum parser_level parser_state_to_level(kafka_response_state state)
{
    switch (state) {
    case KAFKA_FETCH_RESPONSE_START:
    case KAFKA_FETCH_RESPONSE_NUM_TOPICS:
    case KAFKA_FETCH_RESPONSE_TOPIC_NAME_SIZE:
    case KAFKA_FETCH_RESPONSE_NUM_PARTITIONS:
    case KAFKA_FETCH_RESPONSE_PARTITION_START:
    case KAFKA_FETCH_RESPONSE_PARTITION_ERROR_CODE_START:
    case KAFKA_FETCH_RESPONSE_PARTITION_ABORTED_TRANSACTIONS:
    case KAFKA_FETCH_RESPONSE_RECORD_BATCHES_ARRAY_START:

    case KAFKA_PRODUCE_RESPONSE_START:
    case KAFKA_PRODUCE_RESPONSE_NUM_TOPICS:
    case KAFKA_PRODUCE_RESPONSE_TOPIC_NAME_SIZE:
    case KAFKA_PRODUCE_RESPONSE_NUM_PARTITIONS:
    case KAFKA_PRODUCE_RESPONSE_PARTITION_START:
    case KAFKA_PRODUCE_RESPONSE_PARTITION_ERROR_CODE_START:
        return PARSER_LEVEL_PARTITION;
    case KAFKA_FETCH_RESPONSE_RECORD_BATCH_START:
    case KAFKA_FETCH_RESPONSE_RECORD_BATCH_LENGTH:
    case KAFKA_FETCH_RESPONSE_RECORD_BATCH_MAGIC:
    case KAFKA_FETCH_RESPONSE_RECORD_BATCH_RECORDS_COUNT:
    case KAFKA_FETCH_RESPONSE_RECORD_BATCH_END:
    case KAFKA_FETCH_RESPONSE_RECORD_BATCHES_ARRAY_END:
        return PARSER_LEVEL_RECORD_BATCH;
    case KAFKA_FETCH_RESPONSE_PARTITION_TAGGED_FIELDS:
    case KAFKA_FETCH_RESPONSE_PARTITION_END:
        return PARSER_LEVEL_PARTITION;
    }
}

static __always_inline enum parse_result kafka_continue_parse_response_partition_loop_fetch(kafka_info_t *kafka,
                                                                            conn_tuple_t *tup,
                                                                            kafka_response_context_t *response,
                                                                            pktbuf_t pkt, u32 offset,
                                                                            u32 data_end,
                                                                            u32 api_version)
{
    extra_debug("Parsing fetch response");
    u32 orig_offset = offset;
    bool flexible = api_version >= 12;
    enum parse_result ret;

    extra_debug("carry_over_offset %d", response->carry_over_offset);

    if (response->carry_over_offset < 0) {
        return RET_ERR;
    }

    offset += response->carry_over_offset;
    response->carry_over_offset = 0;

    switch (response->state) {
    case KAFKA_FETCH_RESPONSE_START:
        if (flexible) {
            ret = skip_tagged_fields(response, pkt, &offset, data_end, true);
            if (ret != RET_DONE) {
                return ret;
            }
        }

        if (api_version >= 1) {
            offset += sizeof(s32); // Skip throttle_time_ms
        }
        if (api_version >= 7) {
            offset += sizeof(s16); // Skip error_code
            offset += sizeof(s32); // Skip session_id
        }
        response->state = KAFKA_FETCH_RESPONSE_NUM_TOPICS;
        // fallthrough

    case KAFKA_FETCH_RESPONSE_NUM_TOPICS:
        {
            s64 num_topics = 0;
            ret = read_varint_or_s32(flexible, response, pkt, &offset, data_end, &num_topics, true,
                                     VARINT_BYTES_NUM_TOPICS);
            extra_debug("num_topics: %lld", num_topics);
            if (ret != RET_DONE) {
                return ret;
            }
            if (num_topics <= 0) {
                return RET_ERR;
            }
        }
        response->state = KAFKA_FETCH_RESPONSE_TOPIC_NAME_SIZE;
        // fallthrough

    case KAFKA_FETCH_RESPONSE_TOPIC_NAME_SIZE:
        {
            s64 topic_name_size = 0;
            ret = read_varint_or_s16(flexible, response, pkt, &offset, data_end, &topic_name_size, true,
                                     VARINT_BYTES_TOPIC_NAME_SIZE);
            extra_debug("topic_name_size: %lld", topic_name_size);
            if (ret != RET_DONE) {
                return ret;
            }
            if (topic_name_size <= 0 || topic_name_size > TOPIC_NAME_MAX_ALLOWED_SIZE) {
                return RET_ERR;
            }

            // Should we check that topic name matches the topic we expect?
            offset += topic_name_size;
        }
        response->state = KAFKA_FETCH_RESPONSE_NUM_PARTITIONS;
        // fallthrough

    case KAFKA_FETCH_RESPONSE_NUM_PARTITIONS:
        {
            s64 number_of_partitions = 0;
            ret = read_varint_or_s32(flexible, response, pkt, &offset, data_end, &number_of_partitions, true,
                                     VARINT_BYTES_NUM_PARTITIONS);
            extra_debug("number_of_partitions: %lld", number_of_partitions);
            if (ret != RET_DONE) {
                return ret;
            }
            if (number_of_partitions <= 0) {
                return RET_ERR;
            }

            response->partitions_count = number_of_partitions;
            response->state = KAFKA_FETCH_RESPONSE_PARTITION_START;
            response->record_batches_num_bytes = 0;
            response->record_batch_length = 0;
        }
        break;
    default:
        break;
    }

#pragma unroll(KAFKA_RESPONSE_PARSER_MAX_ITERATIONS)
    for (int i = 0; i < KAFKA_RESPONSE_PARSER_MAX_ITERATIONS; i++) {
        bool first = i == 0;

        extra_debug("partition state: %d", response->state);
        switch (response->state) {
        case KAFKA_FETCH_RESPONSE_START:
        case KAFKA_FETCH_RESPONSE_NUM_TOPICS:
        case KAFKA_FETCH_RESPONSE_TOPIC_NAME_SIZE:
        case KAFKA_FETCH_RESPONSE_NUM_PARTITIONS:
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
                extra_debug("invalid error code: %d", error_code);
                return RET_ERR;
            }
            extra_debug("got error code: %d", error_code);
            response->partition_error_code = error_code;

            offset += sizeof(s64); // Skip high_watermark

            if (api_version >= 4) {
                offset += sizeof(s64); // Skip last_stable_offset

                if (api_version >= 5) {
                    offset += sizeof(s64); // log_start_offset
                }
            }

            response->state = KAFKA_FETCH_RESPONSE_PARTITION_ABORTED_TRANSACTIONS;
            // fallthrough
            }

        case KAFKA_FETCH_RESPONSE_PARTITION_ABORTED_TRANSACTIONS:
            if (api_version >= 4) {
                s64 aborted_transactions = 0;
                ret = read_varint_or_s32(flexible, response, pkt, &offset, data_end, &aborted_transactions, first,
                                         VARINT_BYTES_NUM_ABORTED_TRANSACTIONS);
                if (ret != RET_DONE) {
                    return ret;
                }

                extra_debug("aborted_transactions: %lld", aborted_transactions);

                // Note that -1 is a valid value which means that the list is empty.
                if (aborted_transactions < -1) {
                    return RET_ERR;
                }
                // If we interpret some junk data as a packet with a huge aborted_transactions,
                // we could end up missing up a lot of future response processing since we
                // would wait for the end of the aborted_transactions list. So add a limit
                // as a heuristic.
                if (aborted_transactions >= KAFKA_MAX_ABORTED_TRANSACTIONS) {
                    extra_debug("Possibly invalid aborted_transactions %lld", aborted_transactions);
                    return RET_ERR;
                }
                if (aborted_transactions >= 0) {
                    // producer_id and first_offset in each aborted transaction
                    u32 transaction_size = sizeof(s64) * 2;

                    if (flexible) {
                        // Assume zero tagged fields.  It's a bit involved to verify that they are
                        // zero here so we don't do it for now.
                        transaction_size += sizeof(u8);
                    }

                    offset += transaction_size * aborted_transactions;
                }

                if (api_version >= 11) {
                    offset += sizeof(s32); // preferred_read_replica
                }
            }

            response->state = KAFKA_FETCH_RESPONSE_RECORD_BATCHES_ARRAY_START;
            // fallthrough

        case KAFKA_FETCH_RESPONSE_RECORD_BATCHES_ARRAY_START:
            if (response->record_batches_arrays_count >= KAFKA_MAX_RECORD_BATCHES_ARRAYS) {
                extra_debug("exit due to record_batches_array full");
                goto exit;
            }

            s64 tmp = 0;
            ret = read_varint_or_s32(flexible, response, pkt, &offset, data_end, &tmp, first,
                                     VARINT_BYTES_RECORD_BATCHES_NUM_BYTES);
            if (ret != RET_DONE) {
                return ret;
            }

            response->record_batches_num_bytes = tmp;

            extra_debug("record_batches_num_bytes: %d", response->record_batches_num_bytes);

            if (response->record_batches_num_bytes != 0) {
                u32 idx = response->record_batches_arrays_count;

                if (idx >= KAFKA_MAX_RECORD_BATCHES_ARRAYS) {
                    extra_debug("out of space in record_batches_array");
                    return RET_ERR;
                }

                extra_debug("setting record_batches_arrays in index %d with error code %d", idx, response->partition_error_code);
                kafka->record_batches_arrays[idx].partition_error_code = response->partition_error_code;
                kafka->record_batches_arrays[idx].num_bytes = response->record_batches_num_bytes;
                kafka->record_batches_arrays[idx].offset = offset - orig_offset;
                response->record_batches_arrays_count++;
            }

            offset += response->record_batches_num_bytes;
            response->state = KAFKA_FETCH_RESPONSE_PARTITION_TAGGED_FIELDS;
            // fallthrough

        case KAFKA_FETCH_RESPONSE_PARTITION_TAGGED_FIELDS:
            if (flexible) {
                // Verification disabled due to code size limitations.
                ret = skip_tagged_fields(response, pkt, &offset, data_end, false);
                if (ret != RET_DONE) {
                    return ret;
                }
            }
            response->state = KAFKA_FETCH_RESPONSE_PARTITION_END;
            // fallthrough

        case KAFKA_FETCH_RESPONSE_PARTITION_END:
            if (offset > data_end) {
                response->carry_over_offset = offset - data_end;
                return RET_EOP;
            }

            response->partitions_count--;
            if (response->partitions_count == 0) {
                return RET_DONE;
            }

            response->state = KAFKA_FETCH_RESPONSE_PARTITION_START;
            break;

        default:

            extra_debug("invalid state %d in partition parser", response->state);
            return RET_ERR;
            break;
        }
    }

exit:
    // We should have exited at KAFKA_FETCH_RESPONSE_PARTITION_END if we
    // managed to parse the entire packet, so if we get here we still have
    // more to go. Remove the skb_info.data_off so that this function can
    // be called again on the same packet with the same arguments in a tail
    // call.
    response->carry_over_offset = offset - orig_offset;
    return RET_LOOP_END;
}

static __always_inline enum parse_result kafka_continue_parse_response_partition_loop_produce(kafka_info_t *kafka,
                                                                            conn_tuple_t *tup,
                                                                            kafka_response_context_t *response,
                                                                            pktbuf_t pkt, u32 offset,
                                                                            u32 data_end,
                                                                            u32 api_version)
{
    extra_debug("Parsing produce response");
    u32 orig_offset = offset;
    bool flexible = api_version >= 9;
    enum parse_result ret;

    extra_debug("carry_over_offset %d", response->carry_over_offset);

    if (response->carry_over_offset < 0) {
        return RET_ERR;
    }

    offset += response->carry_over_offset;
    response->carry_over_offset = 0;

    switch (response->state) {
    case KAFKA_PRODUCE_RESPONSE_START:
        extra_debug("KAFKA_PRODUCE_RESPONSE_START");
        if (flexible) {
            ret = skip_tagged_fields(response, pkt, &offset, data_end, true);
            if (ret != RET_DONE) {
                return ret;
            }
        }

        response->state = KAFKA_PRODUCE_RESPONSE_NUM_TOPICS;
        // fallthrough

    case KAFKA_PRODUCE_RESPONSE_NUM_TOPICS:
    {
        extra_debug("KAFKA_PRODUCE_RESPONSE_NUM_TOPICS");
        s64 num_topics = 0;
        ret = read_varint_or_s32(flexible, response, pkt, &offset, data_end, &num_topics, true,
                                 VARINT_BYTES_NUM_TOPICS);
        extra_debug("num_topics: %lld", num_topics);
        if (ret != RET_DONE) {
            return ret;
        }
        if (num_topics <= 0) {
            return RET_ERR;
        }
    }
    response->state = KAFKA_PRODUCE_RESPONSE_TOPIC_NAME_SIZE;
    // fallthrough

    case KAFKA_PRODUCE_RESPONSE_TOPIC_NAME_SIZE:
    {
        extra_debug("KAFKA_PRODUCE_RESPONSE_TOPIC_NAME_SIZE");
        s64 topic_name_size = 0;
        ret = read_varint_or_s16(flexible, response, pkt, &offset, data_end, &topic_name_size, true,
                                 VARINT_BYTES_TOPIC_NAME_SIZE);
        extra_debug("topic_name_size: %lld", topic_name_size);
        if (ret != RET_DONE) {
            return ret;
        }
        if (topic_name_size <= 0 || topic_name_size > TOPIC_NAME_MAX_ALLOWED_SIZE) {
            return RET_ERR;
        }
        offset += topic_name_size;
    }
    response->state = KAFKA_PRODUCE_RESPONSE_NUM_PARTITIONS;
    // fallthrough

    case KAFKA_PRODUCE_RESPONSE_NUM_PARTITIONS:
    {
        extra_debug("KAFKA_PRODUCE_RESPONSE_NUM_PARTITIONS");
        s64 number_of_partitions = 0;
        ret = read_varint_or_s32(flexible, response, pkt, &offset, data_end, &number_of_partitions, true,
                              VARINT_BYTES_NUM_PARTITIONS);
        extra_debug("number_of_partitions: %lld", number_of_partitions);
        if (ret != RET_DONE) {
            return ret;
        }
        if (number_of_partitions <= 0 || number_of_partitions >= 2) {
            // We only support a single partition for produce requests at the moment
            return RET_ERR;
        }
        response->partitions_count = number_of_partitions;
        response->state = KAFKA_PRODUCE_RESPONSE_PARTITION_START;

    }
        break;

    default:
        break;
    }

    switch (response->state) {
    case KAFKA_PRODUCE_RESPONSE_PARTITION_START:
        offset += sizeof(s32); // Skip partition_index
        response->state = KAFKA_PRODUCE_RESPONSE_PARTITION_ERROR_CODE_START;
        // fallthrough

    case KAFKA_PRODUCE_RESPONSE_PARTITION_ERROR_CODE_START:
    {
        // Error codes range from -1 to 119 as per the Kafka protocol specification.
        // For details, refer to: https://kafka.apache.org/protocol.html#protocol_error_codes
        s16 error_code = 0;
        ret = read_with_remainder_s16(response, pkt, &offset, data_end, &error_code, true);
        if (ret != RET_DONE) {
            return ret;
        }
        if (error_code < -1 || error_code > 119) {
            extra_debug("invalid error code: %d", error_code);
            return RET_ERR;
        }
        extra_debug("got error code: %d", error_code);
        response->partition_error_code = error_code;
        response->transaction.error_code = error_code;

        // No need to continue parsing the produce response, as we got the error now
        return RET_DONE;
    }
    default:
        break;
    }

    response->carry_over_offset = offset - orig_offset;
    return RET_LOOP_END;
}

static __always_inline enum parse_result kafka_continue_parse_response_record_batches_loop(kafka_info_t *kafka,
                                                                            conn_tuple_t *tup,
                                                                            kafka_response_context_t *response,
                                                                            pktbuf_t pkt, u32 offset,
                                                                            u32 data_end,
                                                                            u32 api_version)
{
    u32 orig_offset = offset;
    enum parse_result ret;

    extra_debug("carry_over_offset %d", response->carry_over_offset);

    if (response->carry_over_offset < 0) {
        return RET_ERR;
    }

    offset += response->carry_over_offset;
    response->carry_over_offset = 0;

    extra_debug("record batches array num_bytes %u offset %u", response->record_batches_num_bytes, offset);

#pragma unroll(KAFKA_RESPONSE_PARSER_MAX_ITERATIONS)
    for (int i = 0; i < KAFKA_RESPONSE_PARSER_MAX_ITERATIONS; i++) {
        bool first = i == 0;

        extra_debug("record batches state: %d", response->state);
        switch (response->state) {
        case KAFKA_FETCH_RESPONSE_RECORD_BATCH_START:
                extra_debug("KAFKA_FETCH_RESPONSE_RECORD_BATCH_START: response->error_code %u, transaction.error_code %u, transaction.records_count: %d \n", response->partition_error_code,
                response->partition_error_code,
                response->transaction.records_count);
            // If the next record batch has an error code that the ones we've
            // been seeing so far in the accumulated transaction, we should emit
            // the transaction event first and then continue parsing.  We can't
            // emit the event from inside this loop due to instruction count
            // restrictions, so force an exit and let the caller do it.
            if (response->transaction.records_count > 0 && response->partition_error_code != response->transaction.error_code) {
                goto exit;
            }

            extra_debug("KAFKA_FETCH_RESPONSE_RECORD_BATCH_START: setting transaction error code to %d",  response->partition_error_code);
            response->transaction.error_code = response->partition_error_code;

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
                if (response->transaction.records_count > 0 && response->partitions_count <= 1 &&
                        response->record_batches_arrays_count - response->record_batches_arrays_idx == 1) {
                    extra_debug("assuming truncated data due to maxsize");
                    response->record_batch_length = 0;
                    response->record_batches_num_bytes = 0;
                    response->state = KAFKA_FETCH_RESPONSE_RECORD_BATCHES_ARRAY_END;
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
            extra_debug("new record_batches_num_bytes %u", response->record_batches_num_bytes);
            response->record_batch_length = 0;

            if (response->record_batches_num_bytes > 0) {
                response->state = KAFKA_FETCH_RESPONSE_RECORD_BATCH_START;
                break;
            }

        case KAFKA_FETCH_RESPONSE_RECORD_BATCHES_ARRAY_END:
        {
            // The u64 type is used here to avoid some verifier errors if the
            // compiler performs the index bounds check on a different register
            // than the one used for the final access operation.
            u64 idx = response->record_batches_arrays_idx + 1;
            if (idx >= response->record_batches_arrays_count) {
                response->record_batches_arrays_idx = idx;
                response->carry_over_offset = offset - orig_offset;
                return RET_DONE;
            }

            if (idx >= KAFKA_MAX_RECORD_BATCHES_ARRAYS) {
                return RET_ERR;
            }

            response->partition_error_code = kafka->record_batches_arrays[idx].partition_error_code;
            response->record_batches_num_bytes = kafka->record_batches_arrays[idx].num_bytes;
            offset = kafka->record_batches_arrays[idx].offset + orig_offset;
            response->state = KAFKA_FETCH_RESPONSE_RECORD_BATCH_START;
            response->record_batches_arrays_idx = idx;
            extra_debug("next idx %llu num_bytes %u offset %u", idx, response->record_batches_num_bytes, offset);
            extra_debug("next idx %llu error_code %u\n", idx, response->partition_error_code);
        }
            break;

        default:
            extra_debug("invalid state %d in record batches array parser", response->state);
            break;
        }
    }

exit:
    // We should have exited at KAFKA_FETCH_RESPONSE_PARTITION_END if we
    // managed to parse the entire packet, so if we get here we still have
    // more to go. Remove the skb_info.data_off so that this function can
    // be called again on the same packet with the same arguments in a tail
    // call.
    response->carry_over_offset = offset - orig_offset;
    return RET_LOOP_END;
}

static __always_inline void kafka_call_response_parser(void *ctx, conn_tuple_t *tup, pktbuf_t pkt, kafka_response_state state, u32 api_version, u32 api_key)
{
    enum parser_level level = parser_state_to_level(state);
    // Leave uninitialzed to get a compile-time warning if we miss setting it in
    // some code path.
    u32 index;

    switch (level) {
    case PARSER_LEVEL_RECORD_BATCH: // Can only be fetch
        if (api_version >= 12) {
            index = PROG_KAFKA_FETCH_RESPONSE_RECORD_BATCH_PARSER_V12;
        } else {
            index = PROG_KAFKA_FETCH_RESPONSE_RECORD_BATCH_PARSER_V0;
        }
        break;
    case PARSER_LEVEL_PARTITION: // Can be fetch or produce
    default:
        switch (api_key) {
        case KAFKA_FETCH:
            if (api_version >= 12) {
                index = PROG_KAFKA_FETCH_RESPONSE_PARTITION_PARSER_V12;
            } else {
                index = PROG_KAFKA_FETCH_RESPONSE_PARTITION_PARSER_V0;
            }
            break;
        case KAFKA_PRODUCE:
            if (api_version >= 9) {
                index = PROG_KAFKA_PRODUCE_RESPONSE_PARTITION_PARSER_V9;
            } else {
                index = PROG_KAFKA_PRODUCE_RESPONSE_PARTITION_PARSER_V0;
            }
            break;
        default:
            // Shouldn't happen
            return;
        }
    }

    switch (pkt.type) {
    case PKTBUF_SKB:
        bpf_tail_call_compat(ctx, &protocols_progs, index);
        break;
    case PKTBUF_TLS:
        bpf_tail_call_compat(ctx, &tls_process_progs, index);
        break;
    }

    // The only reason we would get here if the tail call failed due to too
    // many tail calls.
    extra_debug("failed to call response parser");
    bpf_map_delete_elem(&kafka_response, tup);
}

static __always_inline enum parse_result kafka_continue_parse_response(void *ctx, kafka_info_t *kafka,
                                                                       conn_tuple_t *tup,
                                                                       kafka_response_context_t *response,
                                                                       pktbuf_t pkt, u32 offset,
                                                                       u32 data_end,
                                                                       enum parser_level level,
                                                                       u32 api_version,
                                                                       u32 api_key)
{
    enum parse_result ret = 0;

    if (level == PARSER_LEVEL_PARTITION) {
        response->record_batches_arrays_count = 0;
        response->record_batches_arrays_idx = 0;

        if (api_key == KAFKA_PRODUCE) {
            ret = kafka_continue_parse_response_partition_loop_produce(kafka, tup, response, pkt, offset, data_end, api_version);
        } else if (api_key == KAFKA_FETCH) {
            ret = kafka_continue_parse_response_partition_loop_fetch(kafka, tup, response, pkt, offset, data_end, api_version);
        }
        extra_debug("partition loop ret %d record_batches_array_count %u partitions_count %u", ret, response->record_batches_arrays_count, response->partitions_count);

        // If we have parsed any record batches arrays (message sets), then
        // start processing them with the record batches parser. We will resume
        // the partition parser (either at the end of the current partition, if
        // it was incomplete, or at the start of the next partition) after that
        // is done. In this way, we don't have to save the state (varint
        // position, remainder, etc.) separately for each parser.
        if (ret != RET_ERR && response->record_batches_arrays_count) {
            response->varint_value = 0;
            response->varint_position = 0;
            response->partition_state = response->state;
            response->state = KAFKA_FETCH_RESPONSE_RECORD_BATCH_START;
            response->partition_error_code = kafka->record_batches_arrays[0].partition_error_code;
            response->record_batches_num_bytes = kafka->record_batches_arrays[0].num_bytes;
            response->carry_over_offset = kafka->record_batches_arrays[0].offset;
            // Caller will do tail call
            return RET_LOOP_END;
        }

        if (ret == RET_DONE) {
            extra_debug("enqueue, records_count %d, error_code %d",  response->transaction.records_count, response->transaction.error_code);
            kafka_batch_enqueue_wrapper(kafka, tup, &response->transaction);
            return ret;
        }
    } else {
        extra_debug("record batches before loop idx %u count %u\n", response->record_batches_arrays_idx, response->record_batches_arrays_count);

        ret = kafka_continue_parse_response_record_batches_loop(kafka, tup, response, pkt, offset, data_end, api_version);
        extra_debug("record batches loop ret %d carry_over_offset %d", ret, response->carry_over_offset);
        extra_debug("record batches after loop idx %u count %u\n", response->record_batches_arrays_idx, response->record_batches_arrays_count);

        // If we've exited due to having to queue up the existing event before
        // parsing a new error code, handle that now.  See the corresponding
        // comment in kafka_continue_parse_response_record_batches_loop().
        if (ret == RET_LOOP_END && response->transaction.records_count > 0 && response->partition_error_code != response->transaction.error_code) {
                extra_debug("enqueue from new condition, records_count %d, error_code %d",
                    response->transaction.records_count,
                    response->partition_error_code);
                kafka_batch_enqueue_wrapper(kafka, tup, &response->transaction);
                response->transaction.records_count = 0;
                response->transaction.error_code = 0;
                return ret;
        }

        // When we're done with parsing the record batch arrays, we either need
        // to return to the partition parser (if there are partitions left to
        // parse), or exit.
        if (ret == RET_DONE) {
            if (response->partitions_count == 0) {
                extra_debug("enqueue, records_count %d",  response->transaction.records_count);
                kafka_batch_enqueue_wrapper(kafka, tup, &response->transaction);
                return ret;
            }

            // We resume the partition parsing at the end of the record batches
            // array, since that's the offset we have in `carry_over_offset`.  However,
            // on the previous run of the partition parser, the parser may have have
            // stopped somewhere before the start of the record batches array in
            // the _next_ partition, and thus already have reduced the
            // `partitions_count` to account for the current partition.  In that
            // case, we need to adjust `partitions_count` since we will be
            // re-running the end states for the current partition.
            if (response->partition_state <= KAFKA_FETCH_RESPONSE_RECORD_BATCHES_ARRAY_START) {
                response->partitions_count++;
            }
            response->state = KAFKA_FETCH_RESPONSE_PARTITION_TAGGED_FIELDS;

            // Caller will do tail call
            return RET_LOOP_END;
        }

        // If we seen an end-of-packet, it must mean that we're on the last
        // record_batches_arrays, since if there are more it means that the
        // partition parser was able to parse to the next one.
        //
        // The num_bytes and offset of the last record batches array is already
        // spilled into the fields in the kafka_response_context_t.  We just
        // need to ensure that it looks like we have only one element in the
        // array of record batches arrays so that we end processing correctly
        // after we fully parse the next paket.
        if (ret == RET_EOP) {
            u32 idx = response->record_batches_arrays_idx;
            u32 size = response->record_batches_arrays_count;

            if (idx != size - 1) {
                extra_debug("EOP in record batch loop on non-last record batch array %u %u", idx, size);
                return RET_ERR;
            }

            response->record_batches_arrays_idx = 0;
            response->record_batches_arrays_count = 1;
            extra_debug("reset idx 0 count 1");
        }
    }

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

static __always_inline void kafka_response_parser(kafka_info_t *kafka, void *ctx, conn_tuple_t *tup, pktbuf_t pkt,
enum parser_level level, u32 min_api_version, u32 max_api_version, u32 target_api_key) {
    kafka_response_context_t *response = bpf_map_lookup_elem(&kafka_response, tup);
    if (!response) {
        return;
    }

    u32 api_version = response->transaction.request_api_version;
    u32 api_key = response->transaction.request_api_key;

    if (api_version < min_api_version || api_version > max_api_version) {
        // Should never happen.  This check is there to inform the compiler about
        // the bounds of api_version so that it can optimize away branches for versions
        // outside the range at compile time.
        return;
    }
    if (api_key != target_api_key) {
        // Should never happen.  This check is there to inform the compiler about
        // the target_api_key so that it can optimize away branches for other keys
        return;
    }

    u32 data_off = pktbuf_data_offset(pkt);
    u32 data_end = pktbuf_data_end(pkt);

    enum parse_result result = kafka_continue_parse_response(ctx, kafka, tup, response, pkt,
                                                             data_off, data_end, level,
                                                             api_version, target_api_key);
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
        kafka_call_response_parser(ctx, tup, pkt, response->state, response->transaction.request_api_version, response->transaction.request_api_key);

        // If we failed (due to exceeding tail calls), at least flush what
        // we have.
        if (response->transaction.records_count) {
            extra_debug("enqueue (loop exceeded), records_count %d", response->transaction.records_count);
            kafka_batch_enqueue_wrapper(kafka, tup, &response->transaction);
        }
        break;
    }
}

static __always_inline int __socket__kafka_response_parser(struct __sk_buff *skb, enum parser_level level, u32 min_api_version, u32 max_api_version, u32 target_api_key) {
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

    kafka_response_parser(kafka, skb, &tup, pktbuf_from_skb(skb, &skb_info), level, min_api_version, max_api_version, target_api_key);

    return 0;
}

SEC("socket/kafka_fetch_response_partition_parser_v0")
int socket__kafka_fetch_response_partition_parser_v0(struct __sk_buff *skb) {
    return __socket__kafka_response_parser(skb, PARSER_LEVEL_PARTITION, 0, 11, KAFKA_FETCH);
}

SEC("socket/kafka_fetch_response_partition_parser_v12")
int socket__kafka_fetch_response_partition_parser_v12(struct __sk_buff *skb) {
    return __socket__kafka_response_parser(skb, PARSER_LEVEL_PARTITION, 12, KAFKA_DECODING_MAX_SUPPORTED_FETCH_REQUEST_API_VERSION, KAFKA_FETCH);
}

SEC("socket/kafka_fetch_response_record_batch_parser_v0")
int socket__kafka_fetch_response_record_batch_parser_v0(struct __sk_buff *skb) {
    return __socket__kafka_response_parser(skb, PARSER_LEVEL_RECORD_BATCH, 0, 11, KAFKA_FETCH);
}

SEC("socket/kafka_fetch_response_record_batch_parser_v12")
int socket__kafka_fetch_response_record_batch_parser_v12(struct __sk_buff *skb) {
    return __socket__kafka_response_parser(skb, PARSER_LEVEL_RECORD_BATCH, 12, KAFKA_DECODING_MAX_SUPPORTED_FETCH_REQUEST_API_VERSION, KAFKA_FETCH);
}

SEC("socket/kafka_produce_response_partition_parser_v0")
int socket__kafka_produce_response_partition_parser_v0(struct __sk_buff *skb) {
    return __socket__kafka_response_parser(skb, PARSER_LEVEL_PARTITION, 0, 8, KAFKA_PRODUCE);
}

SEC("socket/kafka_produce_response_partition_parser_v9")
int socket__kafka_produce_response_partition_parser_v9(struct __sk_buff *skb) {
    return __socket__kafka_response_parser(skb, PARSER_LEVEL_PARTITION, 9, KAFKA_DECODING_MAX_SUPPORTED_PRODUCE_REQUEST_API_VERSION, KAFKA_PRODUCE);
}


static __always_inline int __uprobe__kafka_tls_response_parser(struct pt_regs *ctx, enum parser_level level, u32 min_api_version, u32 max_api_version, u32 target_api_key) {
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
    kafka_response_parser(kafka, ctx, &tup, pktbuf_from_tls(ctx, args), level, min_api_version, max_api_version, target_api_key);

    return 0;
}

SEC("uprobe/kafka_tls_fetch_response_partition_parser_v0")
int uprobe__kafka_tls_fetch_response_partition_parser_v0(struct pt_regs *ctx) {
    return __uprobe__kafka_tls_response_parser(ctx, PARSER_LEVEL_PARTITION, 0, 11, KAFKA_FETCH);
}

SEC("uprobe/kafka_tls_fetch_response_partition_parser_v12")
int uprobe__kafka_tls_fetch_response_partition_parser_v12(struct pt_regs *ctx) {
    return __uprobe__kafka_tls_response_parser(ctx, PARSER_LEVEL_PARTITION, 12, KAFKA_DECODING_MAX_SUPPORTED_FETCH_REQUEST_API_VERSION, KAFKA_FETCH);
}

SEC("uprobe/kafka_tls_fetch_response_record_batch_parser_v0")
int uprobe__kafka_tls_fetch_response_record_batch_parser_v0(struct pt_regs *ctx) {
    return __uprobe__kafka_tls_response_parser(ctx, PARSER_LEVEL_RECORD_BATCH, 0, 11, KAFKA_FETCH);
}

SEC("uprobe/kafka_tls_fetch_response_record_batch_parser_v12")
int uprobe__kafka_tls_fetch_response_record_batch_parser_v12(struct pt_regs *ctx) {
    return __uprobe__kafka_tls_response_parser(ctx, PARSER_LEVEL_RECORD_BATCH, 12, KAFKA_DECODING_MAX_SUPPORTED_FETCH_REQUEST_API_VERSION, KAFKA_FETCH);
}

SEC("uprobe/kafka_tls_produce_response_partition_parser_v0")
int uprobe__kafka_tls_produce_response_partition_parser_v0(struct pt_regs *ctx) {
    return __uprobe__kafka_tls_response_parser(ctx, PARSER_LEVEL_PARTITION, 0, 8, KAFKA_PRODUCE);
}

SEC("uprobe/kafka_tls_produce_response_partition_parser_v9")
int uprobe__kafka_tls_produce_response_partition_parser_v9(struct pt_regs *ctx) {
    return __uprobe__kafka_tls_response_parser(ctx, PARSER_LEVEL_PARTITION, 9, KAFKA_DECODING_MAX_SUPPORTED_PRODUCE_REQUEST_API_VERSION, KAFKA_PRODUCE);
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

    if (request->request_api_key == KAFKA_FETCH) {
        kafka->response.state = KAFKA_FETCH_RESPONSE_START;
    } else if (request->request_api_key == KAFKA_PRODUCE) {
        kafka->response.state = KAFKA_PRODUCE_RESPONSE_START;
    } else {
        return false;
    }
    kafka->response.carry_over_offset = offset - orig_offset;
    kafka->response.expected_tcp_seq = kafka_get_next_tcp_seq(skb_info);
    kafka->response.transaction.response_last_seen = bpf_ktime_get_ns();

    // Copy it to the stack since the verifier on 4.14 complains otherwise.
    kafka_response_context_t response_ctx;
    bpf_memcpy(&response_ctx, &kafka->response, sizeof(response_ctx));

    bpf_map_update_elem(&kafka_response, tup, &response_ctx, BPF_ANY);

    kafka_call_response_parser(ctx, tup, pkt, KAFKA_FETCH_RESPONSE_START, kafka->response.transaction.request_api_version, kafka->response.transaction.request_api_key);
    return true;
}

static __always_inline bool kafka_process_response(void *ctx, conn_tuple_t *tup, kafka_info_t *kafka, pktbuf_t pkt, skb_info_t *skb_info) {
    kafka_response_context_t *response = bpf_map_lookup_elem(&kafka_response, tup);
    if (response) {
        response->transaction.response_last_seen = bpf_ktime_get_ns();
        if (!skb_info || skb_info->tcp_seq == response->expected_tcp_seq) {
            response->expected_tcp_seq = kafka_get_next_tcp_seq(skb_info);
            kafka_call_response_parser(ctx, tup, pkt, response->state, response->transaction.request_api_version, response->transaction.request_api_key);
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
    u32 pktlen = pktbuf_data_end(pkt) - offset;

    if (pktlen < sizeof(kafka_header_t)) {
        return false;
    }

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

    // Check if the api key and version are supported
    if(!is_supported_api_version_for_classification(kafka_header.api_key, kafka_header.api_version)) {
        return false;
    }

    // Check if the api key and version are supported
    switch (kafka_header.api_key) {
        case KAFKA_PRODUCE:
            if (kafka_header.api_version > KAFKA_DECODING_MAX_SUPPORTED_PRODUCE_REQUEST_API_VERSION) {
                return false;
            }
            break;
        case KAFKA_FETCH:
            if (kafka_header.api_version > KAFKA_DECODING_MAX_SUPPORTED_FETCH_REQUEST_API_VERSION) {
                return false;
            }
            break;
    }

    kafka_transaction->request_started = bpf_ktime_get_ns();
    kafka_transaction->response_last_seen = 0;
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

    bool flexible = false;

    s16 produce_required_acks = 0;
    switch (kafka_header.api_key) {
    case KAFKA_PRODUCE:
        if (!get_topic_offset_from_produce_request(&kafka_header, pkt, &offset, &produce_required_acks)) {
            return false;
        }
        if (produce_required_acks == 0) {
            __sync_fetch_and_add(&kafka_tel->produce_no_required_acks, 1);
        }
        flexible = kafka_header.api_version >= 9;
        break;
    case KAFKA_FETCH:
        if (!get_topic_offset_from_fetch_request(&kafka_header, pkt, &offset)) {
            return false;
        }
        flexible = kafka_header.api_version >= 12;
        break;
    default:
        return false;
    }

    // Skipping number of entries for now
    if (flexible) {
        if (!skip_varint_number_of_topics(pkt, &offset)) {
            return false;
        }
    } else {
        offset += sizeof(s32);
    }

    s16 topic_name_size = read_nullable_string_size(pkt, flexible, &offset);
    if (topic_name_size <= 0 || topic_name_size > TOPIC_NAME_MAX_ALLOWED_SIZE) {
        return false;
    }

    extra_debug("topic_name_size: %u", topic_name_size);
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
        if (flexible) {
            PKTBUF_READ_BIG_ENDIAN_WRAPPER(s8, partition_count_varint, pkt, offset);

            // Varints are stored as N+1 so this means 1 partition.
            if (partition_count_varint != 2) {
                return false;
            }
        } else {
            PKTBUF_READ_BIG_ENDIAN_WRAPPER(s32, number_of_partitions, pkt, offset);
            if (number_of_partitions <= 0) {
                return false;
            }

            if (number_of_partitions > 1) {
                log_debug("Multiple partitions detected in produce request, current support limited to requests with a single partition");
                return false;
            }
        }
        offset += sizeof(s32); // Skipping Partition ID

        // Parsing the message set of the partition.
        // It's assumed that the message set is not in the old message format, as the format differs:
        // https://kafka.apache.org/documentation/#messageset
        // The old message format predates Kafka 0.11, released on September 27, 2017.
        // It's unlikely for us to encounter these older versions in practice.

        // Skipping record batch (message set in wireshark) size in bytes
        if (flexible) {
            if (!skip_varint(pkt, &offset, VARINT_BYTES_RECORD_BATCHES_NUM_BYTES)) {
                return false;
            }
        } else {
            offset += sizeof(s32);
        }

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
        // We now know the record count, but we'll have to wait for the response to obtain the error code and latency
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

    if (kafka_header.api_key == KAFKA_PRODUCE && produce_required_acks == 0) {
        // If we have a produce request with required acks set to 0, we can enqueue it immediately, as there will be no produce response.
        kafka_batch_enqueue_wrapper(kafka, tup, kafka_transaction);
        return true;
    }

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
    // We have 10 buckets in the ranges of: 1 - 10, 11 - 20, ... , 71 - 80, 81 - 90, 91 - 255
    __u8 bucket_idx = (size - 1) / KAFKA_TELEMETRY_TOPIC_NAME_BUCKET_SIZE;

    // Ensure that the bucket index falls within the valid range.
    bucket_idx = bucket_idx < 0 ? 0 : bucket_idx;
    bucket_idx = bucket_idx > (KAFKA_TELEMETRY_TOPIC_NAME_NUM_OF_BUCKETS - 1) ? (KAFKA_TELEMETRY_TOPIC_NAME_NUM_OF_BUCKETS - 1) : bucket_idx;

    __sync_fetch_and_add(&kafka_tel->topic_name_size_buckets[bucket_idx], 1);
}

#endif
