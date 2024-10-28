#ifndef __PROTOCOL_CLASSIFICATION_COMMON_H
#define __PROTOCOL_CLASSIFICATION_COMMON_H

#include "ktypes.h"

#include "defs.h"
#include "bpf_builtins.h"
#include "bpf_telemetry.h"

#include "protocols/read_into_buffer.h"

#define CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, min_buff_size) \
    do {                                                                  \
        if (buf_size < min_buff_size) {                                   \
            return false;                                                 \
        }                                                                 \
                                                                          \
        if (buf == NULL) {                                                \
            return false;                                                 \
        }                                                                 \
    } while (0)

// Returns true if the packet is TCP.
static __always_inline bool is_tcp(conn_tuple_t *tup) {
    return tup->metadata & CONN_TYPE_TCP;
}

// Returns true if the payload is empty.
static __always_inline bool is_payload_empty(skb_info_t *skb_info) {
    return skb_info->data_off == skb_info->data_end;
}

READ_INTO_BUFFER(for_classification, CLASSIFICATION_MAX_BUFFER, BLK_SIZE)

#endif
