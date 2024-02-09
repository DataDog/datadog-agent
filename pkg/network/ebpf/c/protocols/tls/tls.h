#ifndef __TLS_H
#define __TLS_H

#include "ktypes.h"
#include "bpf_builtins.h"

#define SSL_VERSION20 0x0200
#define SSL_VERSION30 0x0300
#define TLS_VERSION10 0x0301
#define TLS_VERSION11 0x0302
#define TLS_VERSION12 0x0303
#define TLS_VERSION13 0x0304

#define TLS_HANDSHAKE 0x16

/* https://www.rfc-editor.org/rfc/rfc5246#page-19 6.2. Record Layer */

// TLS record layer header structure
typedef struct {
    __u8 content_type;
    __u16 version;
    __u16 length;
} __attribute__((packed)) tls_record_header_t;

typedef struct {
    __u8 handshake_type;
    __u32 length : 24;
    __u16 version;
} __attribute__((packed)) tls_hello_message_t;

#define TLS_HANDSHAKE_CLIENT_HELLO 0x01
#define TLS_HANDSHAKE_SERVER_HELLO 0x02

static __always_inline bool is_tls(const char* buf, __u32 buf_size) {
    if (buf_size < (sizeof(tls_record_header_t)+sizeof(tls_hello_message_t))) {
        return false;
    }

    tls_record_header_t *tls_record_header = (tls_record_header_t *)buf;
    if (tls_record_header->content_type != TLS_HANDSHAKE) {
        return false;
    }

    tls_hello_message_t *tls_hello_message = (tls_hello_message_t *)(buf + sizeof(tls_record_header_t));
    if (tls_hello_message->handshake_type != TLS_HANDSHAKE_CLIENT_HELLO && tls_hello_message->handshake_type != TLS_HANDSHAKE_SERVER_HELLO) {
        return false;
    }

    switch (tls_hello_message->version) {
        case SSL_VERSION20:
        case SSL_VERSION30:
        case TLS_VERSION10:
        case TLS_VERSION11:
        case TLS_VERSION12:
        case TLS_VERSION13:
            return true;
    }
    return false;
}

#endif
