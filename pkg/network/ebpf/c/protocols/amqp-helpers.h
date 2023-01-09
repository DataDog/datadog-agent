#ifndef __AMQP_HELPERS_H
#define __AMQP_HELPERS_H

#include "amqp-defs.h"
#include "protocol-classification-common.h"

// The method checks if the given buffer includes the protocol header which must be sent in the start of a new connection.
// Ref: https://www.rabbitmq.com/resources/specs/amqp0-9-1.pdf
static __always_inline bool is_amqp_protocol_header(const char* buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, AMQP_MIN_FRAME_LENGTH)

#define AMQP_PREFACE "AMQP"

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

    uint8_t frame_type = buf[0];
    // Check only for method frame type.
    if (frame_type != AMQP_FRAME_METHOD_TYPE) {
        return false;
    }

    // We extract the class id and method id by big indian from the buffer.
    // Ref https://www.rabbitmq.com/resources/specs/amqp0-9-1.pdf.
    __u16 class_id = buf[7] << 8 | buf[8];
    __u16 method_id = buf[9] << 8 | buf[10];

    // ConnectionStart, ConnectionStartOk, BasicPublish, BasicDeliver, BasicConsume are the most likely methods to
    // consider for the classification.
    if (class_id == AMQP_CONNECTION_CLASS) {
        return  method_id == AMQP_METHOD_CONNECTION_START || method_id == AMQP_METHOD_CONNECTION_START_OK;
    }

    if (class_id == AMQP_BASIC_CLASS) {
        return method_id == AMQP_METHOD_PUBLISH || method_id == AMQP_METHOD_DELIVER || method_id == AMQP_METHOD_CONSUME;
    }

    return false;
}

#endif
