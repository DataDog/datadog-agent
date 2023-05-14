#ifndef __AMQP_HELPERS_H
#define __AMQP_HELPERS_H

#include "bpf_endian.h"

#include "protocols/amqp/defs.h"
#include "protocols/classification/common.h"

// The method checks if the given buffer includes the protocol header which must be sent in the start of a new connection.
// Ref: https://www.rabbitmq.com/resources/specs/amqp0-9-1.pdf
static __always_inline bool is_amqp_protocol_header(const char* buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, AMQP_MIN_FRAME_LENGTH);

    bool match = !bpf_memcmp(buf, AMQP_PREFACE, sizeof(AMQP_PREFACE)-1);

    return match;
}

// The method checks if the given buffer is an AMQP message.
// Ref: https://www.rabbitmq.com/resources/specs/amqp0-9-1.pdf
static __always_inline bool is_amqp(const char* buf, __u32 buf_size) {
    // New connection should start with protocol header of AMQP.
    // Ref https://www.rabbitmq.com/resources/specs/amqp0-9-1.pdf.
    if (is_amqp_protocol_header(buf, buf_size)) {
        return true;
    }

    // Validate that we will be able to get from the buffer the class and method ids.
    if (buf_size < AMQP_MIN_PAYLOAD_LENGTH) {
       return false;
    }

    __u8 frame_type = buf[0];
    // Check only for method frame type.
    if (frame_type != AMQP_FRAME_METHOD_TYPE) {
        return false;
    }

    // We extract the class id and method id by big endian from the buffer.
    // Ref https://www.rabbitmq.com/resources/specs/amqp0-9-1.pdf.
    amqp_header *hdr = (amqp_header *)(buf+7);
    __u16 class_id = bpf_ntohs(hdr->class_id);
    __u16 method_id = bpf_ntohs(hdr->method_id);

    switch (class_id) {
    case AMQP_CONNECTION_CLASS:
        switch (method_id) {
        case AMQP_METHOD_CONNECTION_START:
        case AMQP_METHOD_CONNECTION_START_OK:
            return true;
        default:
            return false;
        }
    case AMQP_BASIC_CLASS:
        switch (method_id) {
        case AMQP_METHOD_PUBLISH:
        case AMQP_METHOD_DELIVER:
        case AMQP_METHOD_CONSUME:
            return true;
        default:
            return false;
        }
    case AMQP_CHANNEL_CLASS:
        switch (method_id) {
        case AMQP_METHOD_CLOSE_OK:
        case AMQP_METHOD_CLOSE:
            return true;
        default:
            return false;
        }
    default:
        return false;
    }
}

#endif
