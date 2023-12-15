#ifndef __PROTOCOL_CLASSIFICATION_STRUCTS_H
#define __PROTOCOL_CLASSIFICATION_STRUCTS_H

#include "ktypes.h"

#include "conn_tuple.h"

typedef struct {
    __s32   message_length; // total message size, including this
    __s32   request_id;     // identifier for this message
    __s32   response_to;    // requestID from the original request (used in responses from db)
    __s32   op_code;        // request type - see table below for details
} mongo_msg_header;

// The key used in mongo_request_id set.
typedef struct {
    conn_tuple_t tup;
    __s32 req_id;
} mongo_key;

typedef struct {
    conn_tuple_t tup;
    skb_info_t skb_info;
} dispatcher_arguments_t;

typedef struct {
    conn_tuple_t tup;
    __u64 tags;
    char *buffer_ptr;
} tls_dispatcher_arguments_t;

#endif
