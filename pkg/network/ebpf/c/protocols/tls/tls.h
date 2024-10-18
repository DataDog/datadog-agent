#ifndef __TLS_H
#define __TLS_H

#include "ktypes.h"
#include "bpf_builtins.h"

#include "ip.h"

#define ETH_HLEN 14  // Ethernet header length

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

typedef struct {
    __u16 offered_versions[6];
} tls_client_tags_t;

typedef struct {
    __u16 version;
    __u16 cipher_suite;
    __u8 compression_method;
} tls_server_tags_t;

typedef struct {
    tls_client_tags_t client_tags;
    tls_server_tags_t server_tags;
} tls_enhanced_tags_t;

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
static __always_inline bool is_valid_tls_app_data(tls_record_header_t *hdr, __u32 skb_len) {
    return hdr->length + sizeof(tls_record_header_t) <= skb_len;
}

static __always_inline bool parse_supported_versions_extension(struct __sk_buff *skb, __u64 offset, __u16 extension_length) {
    __u64 skb_len = skb->len;

    // Ensure we have at least 1 byte for the list length
    if (offset + 1 > skb_len) {
        return false;
    }

    // Read Supported Versions Length (1 byte)
    __u8 sv_list_length;
    if (bpf_skb_load_bytes(skb, offset, &sv_list_length, sizeof(sv_list_length)) < 0) {
        return false;
    }
    offset += 1;

    // Ensure the list length is consistent with the extension length
    if (sv_list_length + 1 > extension_length) {
        return false;
    }

    // Ensure we don't read beyond the packet
    if (offset + sv_list_length > skb_len) {
        return false;
    }

    // Set an upper bound for the loop to satisfy the eBPF verifier
    #define MAX_SUPPORTED_VERSIONS 8
    __u8 versions_parsed = 0;

    // Read the list of supported versions (2 bytes each)
    for (__u8 i = 0; i + 1 < sv_list_length && versions_parsed < MAX_SUPPORTED_VERSIONS; i += 2, versions_parsed++) {
        __u16 sv_version;
        if (bpf_skb_load_bytes(skb, offset, &sv_version, sizeof(sv_version)) < 0) {
            return false;
        }
        sv_version = bpf_ntohs(sv_version);
        offset += 2;

        if (sv_version == TLS_VERSION13) {
            log_debug("adamk supported version: TLS 1.3");
        }

        // TODO: add supported version to the map

    }
    log_debug("adamk supported versions parsed: %d", versions_parsed);
    return true;
}

static __always_inline bool parse_client_hello_extensions(struct __sk_buff *skb, __u64 offset, __u16 extensions_length) {
    __u64 skb_len = skb->len;
    __u64 extensions_end = offset + extensions_length;

    // Set an upper bound for the loop to satisfy the eBPF verifier
    #define MAX_EXTENSIONS 16
    __u8 extensions_parsed = 0;

    while (offset + 4 <= extensions_end && extensions_parsed < MAX_EXTENSIONS) {
        // Read Extension Type (2 bytes)
        __u16 extension_type;
        if (bpf_skb_load_bytes(skb, offset, &extension_type, sizeof(extension_type)) < 0) {
            return false;
        }
        extension_type = bpf_ntohs(extension_type);
        offset += 2;

        // Read Extension Length (2 bytes)
        __u16 extension_length;
        if (bpf_skb_load_bytes(skb, offset, &extension_length, sizeof(extension_length)) < 0) {
            return false;
        }
        extension_length = bpf_ntohs(extension_length);
        offset += 2;

        // Ensure we don't read beyond the packet
        if (offset + extension_length > skb_len || offset + extension_length > extensions_end) {
            return false;
        }

        // Check for supported_versions extension (type 0x002B)
        if (extension_type == 0x002B) {
            if (!parse_supported_versions_extension(skb, offset, extension_length)) {
                return false;
            }
        }

        // Skip to the next extension
        offset += extension_length;
        extensions_parsed++;
    }

    return true;
}

static __always_inline bool parse_client_hello(tls_record_header_t *hdr, struct __sk_buff *skb, __u64 offset) {
    __u32 skb_len = skb->len;

    // Move offset past handshake type (1 byte)
    offset += 1;

    // Read handshake length (3 bytes)
    __u8 handshake_length_bytes[3];
    if (bpf_skb_load_bytes(skb, offset, handshake_length_bytes, 3) < 0)
        return false;
    __u32 handshake_length = (handshake_length_bytes[0] << 16) |
                             (handshake_length_bytes[1] << 8) |
                             handshake_length_bytes[2];
    offset += 3;

    // Ensure we don't read beyond the packet
    if (offset + handshake_length > skb_len)
        return false;

    // Read client version (2 bytes)
    __u16 client_version;
    if (bpf_skb_load_bytes(skb, offset, &client_version, sizeof(client_version)) < 0)
        return false;
    client_version = bpf_ntohs(client_version);
    log_debug("adamk client version: %d", client_version);
    offset += 2;

    // Validate client version
    if (!is_valid_tls_version(client_version))
        return false;

    // Skip Random (32 bytes)
    offset += 32;

    // Read Session ID Length (1 byte)
    __u8 session_id_len;
    if (bpf_skb_load_bytes(skb, offset, &session_id_len, sizeof(session_id_len)) < 0)
        return false;
    offset += 1;

    // Skip Session ID
    offset += session_id_len;

    // Ensure we don't read beyond the packet
    if (offset + 2 > skb_len)
        return false;

    // Read Cipher Suites Length (2 bytes)
    __u16 cipher_suites_length;
    if (bpf_skb_load_bytes(skb, offset, &cipher_suites_length, sizeof(cipher_suites_length)) < 0)
        return false;
    cipher_suites_length = bpf_ntohs(cipher_suites_length);
    log_debug("adamk client cipher_suites_length: %d", cipher_suites_length);
    offset += 2;

    // Ensure we don't read beyond the packet
    if (offset + cipher_suites_length > skb_len)
        return false;

    // Skip Cipher Suites
    offset += cipher_suites_length;

    // Read Compression Methods Length (1 byte)
    if (offset + 1 > skb_len)
        return false;
    __u8 compression_methods_length;
    if (bpf_skb_load_bytes(skb, offset, &compression_methods_length, sizeof(compression_methods_length)) < 0)
        return false;
    offset += 1;

    // Skip Compression Methods
    offset += compression_methods_length;

    // Read Extensions Length (2 bytes)
    if (offset + 2 > skb_len) {
        return false;
    }
    __u16 extensions_length;
    if (bpf_skb_load_bytes(skb, offset, &extensions_length, sizeof(extensions_length)) < 0) {
        return false;
    }
    extensions_length = bpf_ntohs(extensions_length);
    offset += 2;

    // Ensure we don't read beyond the packet
    if (offset + extensions_length > skb_len) {
        return false;
    }

    // Parse Extensions
    if (!parse_client_hello_extensions(skb, offset, extensions_length)) {
        return false;
    }

    // At this point, we've successfully parsed the ClientHello message
    return true;
}

static __always_inline bool parse_server_hello(tls_record_header_t *hdr, struct __sk_buff *skb, __u64 offset) {
    __u32 skb_len = skb->len;

    // Move offset past handshake type (1 byte)
    offset += 1;

    // Read handshake length (3 bytes)
    __u8 handshake_length_bytes[3];
    if (bpf_skb_load_bytes(skb, offset, handshake_length_bytes, 3) < 0)
        return false;
    __u32 handshake_length = (handshake_length_bytes[0] << 16) |
                             (handshake_length_bytes[1] << 8) |
                             handshake_length_bytes[2];
    offset += 3;

    // Ensure we don't read beyond the packet
    if (offset + handshake_length > skb_len)
        return false;

    // Read server version (2 bytes)
    __u16 server_version;
    if (bpf_skb_load_bytes(skb, offset, &server_version, sizeof(server_version)) < 0)
        return false;
    server_version = bpf_ntohs(server_version);
    log_debug("adamk server version: %d", server_version);
    offset += 2;

    // Validate server version
    if (!is_valid_tls_version(server_version))
        return false;

    // Skip Random (32 bytes)
    offset += 32;

    // Read Session ID Length (1 byte)
    __u8 session_id_len;
    if (bpf_skb_load_bytes(skb, offset, &session_id_len, sizeof(session_id_len)) < 0)
        return false;
    offset += 1;

    // Skip Session ID
    offset += session_id_len;

    // Ensure we don't read beyond the packet
    if (offset + 2 > skb_len)
        return false;

    // Read Cipher Suite (2 bytes)
    __u16 cipher_suite;
    if (bpf_skb_load_bytes(skb, offset, &cipher_suite, sizeof(cipher_suite)) < 0)
        return false;
    cipher_suite = bpf_ntohs(cipher_suite);
    log_debug("adamk server cipher_suite: %d", cipher_suite);
    offset += 2;

    // You can store or process the cipher suite as needed

    // Read Compression Method (1 byte)
    if (offset + 1 > skb_len)
        return false;
    __u8 compression_method;
    if (bpf_skb_load_bytes(skb, offset, &compression_method, sizeof(compression_method)) < 0)
        return false;
    offset += 1;

    // Read Extensions Length (2 bytes) if present
    if (offset + 2 <= skb_len) {
        __u16 extensions_length;
        if (bpf_skb_load_bytes(skb, offset, &extensions_length, sizeof(extensions_length)) < 0)
            return false;
        extensions_length = bpf_ntohs(extensions_length);
        offset += 2;

        // Ensure we don't read beyond the packet
        if (offset + extensions_length > skb_len)
            return false;

        // Process extensions if needed
    }

    // At this point, we've successfully parsed the ServerHello message
    return true;
}

static __always_inline bool is_tls_handshake(tls_record_header_t *hdr, struct __sk_buff *skb, __u64 offset) {
    // Read handshake type
    __u8 handshake_type;
    if (bpf_skb_load_bytes(skb, offset, &handshake_type, sizeof(handshake_type)) < 0)
        return false;

    // Only proceed if it's a ClientHello or ServerHello
    if (handshake_type == TLS_HANDSHAKE_CLIENT_HELLO) {
        log_debug("adamk inspecting client hello");
        return parse_client_hello(hdr, skb, offset);
    } else if (handshake_type == TLS_HANDSHAKE_SERVER_HELLO) {
        log_debug("adamk inspecting server hello");
        return parse_server_hello(hdr, skb, offset);
    } else {
        return false;
    }
}

static __always_inline bool is_tls(struct __sk_buff *skb, __u64 nh_off) {
    __u32 skb_len = skb->len;

    // Ensure there's enough space for TLS record header
    if (nh_off + sizeof(tls_record_header_t) > skb_len)
        return false;

    // Read TLS record header
    tls_record_header_t tls_hdr;
    if (bpf_skb_load_bytes(skb, nh_off, &tls_hdr, sizeof(tls_hdr)) < 0)
        return false;

    // Convert fields to host byte order
    tls_hdr.version = bpf_ntohs(tls_hdr.version);
    tls_hdr.length = bpf_ntohs(tls_hdr.length);

    // Validate version and length
    if (!is_valid_tls_version(tls_hdr.version))
        return false;
    if (tls_hdr.length > TLS_MAX_PAYLOAD_LENGTH)
        return false;

    // Validate version and length
    if (!is_valid_tls_version(tls_hdr.version))
        return false;
    if (tls_hdr.length > TLS_MAX_PAYLOAD_LENGTH)
        return false;

    // Move offset to the start of TLS handshake message
    nh_off += sizeof(tls_record_header_t);

    // Ensure we don't read beyond the packet
    if (nh_off + tls_hdr.length > skb_len)
        return false;

    // Handle based on content type
    switch (tls_hdr.content_type) {
        case TLS_HANDSHAKE: {
            // return is_tls_handshake(&tls_hdr, skb, nh_off);
            bool handshake = is_tls_handshake(&tls_hdr, skb, nh_off);
            log_debug("adamk is_tls_handshake: %d", handshake);
            return handshake;
        }
        case TLS_APPLICATION_DATA:
            return is_valid_tls_app_data(&tls_hdr, skb_len);
        default:
            return false;
    }
}

#endif
