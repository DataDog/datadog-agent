#ifndef __TLS_H
#define __TLS_H

#include "ktypes.h"
#include "bpf_builtins.h"

/* https://www.rfc-editor.org/rfc/rfc5246#page-19 6.2. Record Layer */
typedef struct __attribute__((packed)) {
    __u8 app;
    __u16 version;
    __u16 length;
} tls_record_t;

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

static __always_inline bool is_valid_tls_app(u8 app) {
    return (app == TLS_CHANGE_CIPHER) || (app == TLS_ALERT) || (app == TLS_HANDSHAKE) || (app == TLS_APPLICATION_DATA);
}

static __always_inline bool is_valid_tls_version(__u16 version) {
    return (version == SSL_VERSION20) || (version == SSL_VERSION30) || (version == TLS_VERSION10) || (version == TLS_VERSION11) || (version == TLS_VERSION12) || (version == TLS_VERSION13);
}

static __always_inline bool is_payload_length_valid(__u8 app, __u16 tls_len, __u32 buf_size) {
    /* check only for application data layer */
    if (app != TLS_APPLICATION_DATA) {
        return true;
    }

    if (buf_size < (sizeof(tls_record_t)+tls_len)) {
        return false;
    }

    return true;
}

static __always_inline bool is_tls(const char* buf, __u32 buf_size) {
    if (buf_size < TLS_HEADER_SIZE) {
        return false;
    }

    tls_record_t *tls_record = (tls_record_t *)buf;
    if (!is_valid_tls_app(tls_record->app)) {
        return false;
    }

    if (!is_valid_tls_version(tls_record->version)) {
        return false;
    }

    if (tls_record->length > MAX_TLS_FRAGMENT_LENGTH) {
        return false;
    }

    if (!is_payload_length_valid(tls_record->app, tls_record->length, buf_size)) {
        return false;
    }

    return true;
}

#endif
