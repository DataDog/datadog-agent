#ifndef __JAVA_TLS_TYPES_H
#define __JAVA_TLS_TYPES_H

#include "ktypes.h"\

#define MAX_DOMAIN_NAME_LENGTH 64


enum erpc_message_type {
    REQUEST,
    CLOSE_CONNECTION,
    HOSTNAME,
    PLAIN,
};


typedef struct{
    __u32 pid;
    __u16 port;
    char domain_name[MAX_DOMAIN_NAME_LENGTH] __attribute__ ((aligned (8)));
} peer_domain_port_t;


#endif //__JAVA_TLS_TYPES_H
