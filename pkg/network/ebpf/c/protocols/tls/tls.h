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
#define TLS_APPLICATION_DATA 0x17

/* https://www.rfc-editor.org/rfc/rfc5246#page-19 6.2. Record Layer */

#define TLS_MAX_PAYLOAD_LENGTH (1 << 14)

// TLS record layer header structure
typedef struct {
    __u8 content_type;
    __u16 version;
    __u16 length;
} __attribute__((packed)) tls_record_header_t;

typedef struct {
    __u8 handshake_type;
    __u8 length[3];
    __u16 version;
} __attribute__((packed)) tls_hello_message_t;

#define TLS_HANDSHAKE_CLIENT_HELLO 0x01
#define TLS_HANDSHAKE_SERVER_HELLO 0x02
// The size of the handshake type and the length.
#define TLS_HELLO_MESSAGE_HEADER_SIZE 4

// is_valid_tls_version checks if the given version is a valid TLS version as
// defined in the TLS specification.
static __always_inline bool is_valid_tls_version(__u16 version) {
    switch (version) {
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

// is_valid_tls_app_data checks if the buffer is a valid TLS Application Data
// record header. The record header is considered valid if:
// - the TLS version field is a known SSL/TLS version
// - the payload length is below the maximum payload length defined in the
//   standard.
// - the payload length + the size of the record header is less than the size
//   of the skb
static __always_inline bool is_valid_tls_app_data(tls_record_header_t *hdr, __u32 buf_size, __u32 skb_len) {
    return sizeof(*hdr) + hdr->length <= skb_len;
}

// is_tls_handshake checks if the given TLS message header is a valid TLS
// handshake message. The message is considered valid if:
// - The type matches CLIENT_HELLO or SERVER_HELLO
// - The version is a known SSL/TLS version
static __always_inline bool is_tls_handshake(tls_record_header_t *hdr, const char *buf, __u32 buf_size) {
    // Checking the buffer size contains at least the size of the tls record header and the tls hello message header.
    if (sizeof(tls_record_header_t) + sizeof(tls_hello_message_t) > buf_size) {
        return false;
    }
    // Checking the tls record header length is greater than the tls hello message header length.
    if (hdr->length < sizeof(tls_hello_message_t)) {
        return false;
    }

    // Getting the tls hello message header.
    tls_hello_message_t msg = *(tls_hello_message_t *)(buf + sizeof(tls_record_header_t));
    // If the message is not a CLIENT_HELLO or SERVER_HELLO, we don't attempt to classify.
    if (msg.handshake_type != TLS_HANDSHAKE_CLIENT_HELLO && msg.handshake_type != TLS_HANDSHAKE_SERVER_HELLO) {
        return false;
    }
    // Converting the fields to host byte order.
    __u32 length = msg.length[0] << 16 | msg.length[1] << 8 | msg.length[2];
    // TLS handshake message length should be equal to the record header length minus the size of the hello message
    // header.
    if (length + TLS_HELLO_MESSAGE_HEADER_SIZE != hdr->length) {
        return false;
    }

    msg.version = bpf_ntohs(msg.version);
    return is_valid_tls_version(msg.version) && msg.version >= hdr->version;
}

// is_tls checks if the given buffer is a valid TLS record header. We are
// currently checking for two types of record headers:
// - TLS Handshake record headers
// - TLS Application Data record headers
static __always_inline bool is_tls(const char *buf, __u32 buf_size, __u32 skb_len) {
    if (buf_size < sizeof(tls_record_header_t)) {
        return false;
    }

    // Copying struct to the stack, to avoid modifying the original buffer that will be used for other classifiers.
    tls_record_header_t tls_record_header = *(tls_record_header_t *)buf;
    // Converting the fields to host byte order.
    tls_record_header.version = bpf_ntohs(tls_record_header.version);
    tls_record_header.length = bpf_ntohs(tls_record_header.length);

    // Checking the version in the record header.
    if (!is_valid_tls_version(tls_record_header.version)) {
        return false;
    }

    // Checking the length in the record header is not greater than the maximum payload length.
    if (tls_record_header.length > TLS_MAX_PAYLOAD_LENGTH) {
        return false;
    }
    switch (tls_record_header.content_type) {
    case TLS_HANDSHAKE:
        return is_tls_handshake(&tls_record_header, buf, buf_size);
    case TLS_APPLICATION_DATA:
        return is_valid_tls_app_data(&tls_record_header, buf_size, skb_len);
    }

    return false;
}

#endif
