#ifndef __TLS_TYPES_H
#define __TLS_TYPES_H

#include "tracer.h"
#include "classifier.h"

typedef struct __attribute__((packed)) {
    __u8 app;
    __u16 version;
    __u16 length;
} tls_record_t;

// handshake types
#define CLIENT_HELLO 1
#define SERVER_HELLO 2
#define CERTIFICATE 11

typedef struct __attribute__((packed)) {
    __u8 handshake_type;
    __u8 length[3];
} tls_handshake_t;

#define TLS_HEADER_SIZE 5

#define SSL_VERSION20 0x0200
#define SSL_VERSION30 0x0300
#define TLS_VERSION10 0x0301
#define TLS_VERSION11 0x0302
#define TLS_VERSION12 0x0303
#define TLS_VERSION13 0x0304

#define TLS_CHANGE_CIPHER 0x14
#define TLS_ALERT 0x15
#define TLS_HANDSHAKE 0x16
#define TLS_APPLICATION_DATA 0x17
// For tls 1.0, 1.1 and 1.3 maximum allowed size of the TLS fragment
// is 2^14. However, for tls 1.2 maximum size is (2^14)+1024.
#define MAX_TLS_FRAGMENT_LENGTH ((1<<14)+1024)

#define TLS_MAX_PACKET_CLASSIFIER 10

// The different states of the tls connection we observe
#define STATE_HELLO_CLIENT (1)
#define STATE_HELLO_SERVER (1<<1)
#define STATE_SHARE_CERTIFICATE (1<<2)
#define STATE_APPLICATION_DATA (1<<3)

/* packets here is used as guard for miss classification */
typedef struct {
    cnx_info_t info;
    __u8 packets;
    __u8 state;
    __u16 version;
    __u16 cipher_suite;
} tls_session_t __attribute__((aligned(8)));

typedef union {
	tls_session_t tls;
} session_t;

// TLS protocol structure
#define EXTENSION_DATA_LEN (1<<15)
#define NUM_OF_EXTENSIONS EXTENSION_DATA_LEN
struct Extension {
    __u16 extension_type;
    __u8 extension_data[EXTENSION_DATA_LEN];
};

struct ServerHello {
    tls_record_t record;
    tls_handshake_t handshake;
    __u8 major;
    __u8 minor;
    __u32 gmt_unix_time;
    __u8 random_bytes[28];
    __u8 session_id_length;
    __u8 session_id[32];
    __u8 cipher_suite[2];
    __u8 compression_method;
    struct Extension extensions[NUM_OF_EXTENSIONS];
} __attribute__((packed));

#endif
