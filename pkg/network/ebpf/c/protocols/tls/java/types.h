#ifndef __JAVA_TLS_TYPES_H
#define __JAVA_TLS_TYPES_H

#include "ktypes.h"

#define MAX_DOMAIN_NAME_LENGTH 64


enum erpc_message_type {
    SYNCHRONOUS_PAYLOAD,
    CLOSE_CONNECTION,
    CONNECTION_BY_PEER,
    ASYNC_PAYLOAD,
};

typedef struct{
    __u16 port;
    char domain[MAX_DOMAIN_NAME_LENGTH]; //__attribute__ ((aligned (8)));
} peer_t;

typedef struct{
    __u32 pid;
    peer_t peer;
} connection_by_peer_key_t;


#endif //__JAVA_TLS_TYPES_H
