#ifndef __TLS_H
#define __TLS_H

#include "tracer/tracer.h"

#define SSL_VERSION20 0x0200
#define SSL_VERSION30 0x0300
#define TLS_VERSION10 0x0301
#define TLS_VERSION11 0x0302
#define TLS_VERSION12 0x0303
#define TLS_VERSION13 0x0304

// TLS Content Types as per RFC 5246 Section 6.2.1
#define TLS_HANDSHAKE 0x16
#define TLS_APPLICATION_DATA 0x17

#define TLS_HANDSHAKE_CLIENT_HELLO 0x01
#define TLS_HANDSHAKE_SERVER_HELLO 0x02

#define TLS_VERSION10_BIT 0x01
#define TLS_VERSION11_BIT 0x02
#define TLS_VERSION12_BIT 0x04
#define TLS_VERSION13_BIT 0x08

// TLS extensions to parse from the Hello message when searching for the SUPPORTED_VERSIONS_EXTENSION
#define MAX_EXTENSIONS 16
#define SUPPORTED_VERSIONS_EXTENSION 0x002B

// this corresponds to 16 KB, which is the maximum TLS record size as per the specification
#define TLS_MAX_PAYLOAD_LENGTH (1 << 14)

// Byte lengths of fields in the TLS handshake header
#define RANDOM_LENGTH 32
#define TLS_HANDSHAKE_LENGTH 3
#define PROTOCOL_VERSION_LENGTH 2
#define SESSION_ID_LENGTH 1
#define CIPHER_SUITES_LENGTH 2
#define COMPRESSION_METHODS_LENGTH 1
#define EXTENSION_TYPE_LENGTH 2

// TLS record layer header structure (https://www.rfc-editor.org/rfc/rfc5246#page-19)
typedef struct {
    __u8 content_type;
    __u16 version;
    __u16 length;
} __attribute__((packed)) tls_record_header_t;

// is_valid_tls_version checks if the version is a valid TLS version
static __always_inline bool is_valid_tls_version(__u16 version) {
    switch (version) {
        case SSL_VERSION20:
        case SSL_VERSION30:
        case TLS_VERSION10:
        case TLS_VERSION11:
        case TLS_VERSION12:
        case TLS_VERSION13:
            return true;
        default:
            return false;
    }
}

// set_tls_offered_version sets the bit corresponding to the offered version in the offered_versions field of tls_info
static __always_inline void set_tls_offered_version(tls_info_t *tls_info, __u16 version) {
    switch (version) {
        case TLS_VERSION10:
            tls_info->offered_versions |= TLS_VERSION10_BIT;
            break;
        case TLS_VERSION11:
            tls_info->offered_versions |= TLS_VERSION11_BIT;
            break;
        case TLS_VERSION12:
            tls_info->offered_versions |= TLS_VERSION12_BIT;
            break;
        case TLS_VERSION13:
            tls_info->offered_versions |= TLS_VERSION13_BIT;
            break;
        default:
            break;
    }
}

// read_tls_record_header reads the TLS record header from the packet
static __always_inline bool read_tls_record_header(struct __sk_buff *skb, __u64 nh_off, tls_record_header_t *tls_hdr) {
    __u32 skb_len = skb->len;

    // Ensure there's enough space for TLS record header
    if (nh_off + sizeof(tls_record_header_t) > skb_len) {
        return false;
    }

    // Read TLS record header
    if (bpf_skb_load_bytes(skb, nh_off, tls_hdr, sizeof(tls_record_header_t)) < 0) {
        return false;
    }

    // Convert fields to host byte order
    tls_hdr->version = bpf_ntohs(tls_hdr->version);
    tls_hdr->length = bpf_ntohs(tls_hdr->length);

    // Validate version and length
    if (!is_valid_tls_version(tls_hdr->version)) {
        return false;
    }
    if (tls_hdr->length > TLS_MAX_PAYLOAD_LENGTH) {
        return false;
    }

    // Ensure we don't read beyond the packet
    return nh_off + sizeof(tls_record_header_t) + tls_hdr->length <= skb_len;
}

// is_tls checks if the packet is a TLS packet and reads the TLS record header
static __always_inline bool is_tls(struct __sk_buff *skb, __u64 nh_off, tls_record_header_t *tls_hdr) {
    // Use the helper function to read and validate the TLS record header
    if (!read_tls_record_header(skb, nh_off, tls_hdr)) {
        return false;
    }

    // Validate content type
    return tls_hdr->content_type == TLS_HANDSHAKE || tls_hdr->content_type == TLS_APPLICATION_DATA;
}

static __always_inline bool parse_tls_handshake_header(struct __sk_buff *skb, __u64 *offset, __u32 *handshake_length, __u16 *protocol_version) {
    // Move offset past handshake type (1 byte)
    *offset += 1;
    __u32 skb_len = skb->len;

    // Read handshake length (3 bytes)
    if (*offset + TLS_HANDSHAKE_LENGTH > skb_len) {
        return false;
    }
    __u8 handshake_length_bytes[TLS_HANDSHAKE_LENGTH];
    if (bpf_skb_load_bytes(skb, *offset, handshake_length_bytes, TLS_HANDSHAKE_LENGTH) < 0) {
        return false;
    }
    *handshake_length = (handshake_length_bytes[0] << 16) |
                        (handshake_length_bytes[1] << 8) |
                        handshake_length_bytes[2];
    *offset += TLS_HANDSHAKE_LENGTH;

    // Ensure we don't read beyond the packet
    if (*offset + *handshake_length > skb_len) {
        return false;
    }

    // Read protocol version (2 bytes)
    if (*offset + PROTOCOL_VERSION_LENGTH > skb_len) {
        return false;
    }
    __u16 version;
    if (bpf_skb_load_bytes(skb, *offset, &version, sizeof(version)) < 0) {
        return false;
    }
    *protocol_version = bpf_ntohs(version);
    *offset += PROTOCOL_VERSION_LENGTH;

    return true;
}

static __always_inline bool skip_random_and_session_id(struct __sk_buff *skb, __u64 *offset) {
    // Skip Random (32 bytes)
    *offset += RANDOM_LENGTH;
    __u32 skb_len = skb->len;

    // Read Session ID Length (1 byte)
    if (*offset + SESSION_ID_LENGTH > skb_len) {
        return false;
    }
    __u8 session_id_length;
    if (bpf_skb_load_bytes(skb, *offset, &session_id_length, sizeof(session_id_length)) < 0) {
        return false;
    }
    *offset += SESSION_ID_LENGTH;

    // Skip Session ID
    *offset += session_id_length;

    // Ensure we don't read beyond the packet
    if (*offset > skb_len) {
        return false;
    }

    return true;
}

static __always_inline bool parse_supported_versions_extension(struct __sk_buff *skb, __u64 *offset, __u64 extensions_end, tls_info_t *tags, bool is_client_hello) {
    __u32 skb_len = skb->len;
    if (is_client_hello) {
        // Read list length (1 byte)
        if (*offset + 1 > skb_len || *offset + 1 > extensions_end) {
            return false;
        }
        __u8 sv_list_length;
        if (bpf_skb_load_bytes(skb, *offset, &sv_list_length, sizeof(sv_list_length)) < 0) {
            return false;
        }
        *offset += 1;

        // Ensure we don't read beyond the packet
        if (*offset + sv_list_length > skb_len || *offset + sv_list_length > extensions_end) {
            return false;
        }

        // Parse the list of supported versions
        __u8 sv_offset = 0;
        __u16 sv_version;
        #define MAX_SUPPORTED_VERSIONS 4
        #pragma unroll(MAX_SUPPORTED_VERSIONS)
        for (int idx = 0; idx < MAX_SUPPORTED_VERSIONS; idx++) {
            if (sv_offset + 1 >= sv_list_length) {
                break;
            }
            if (*offset + 2 > skb_len) {
                return false;
            }

            // Load the supported version
            if (bpf_skb_load_bytes(skb, *offset, &sv_version, sizeof(sv_version)) < 0) {
                return false;
            }
            sv_version = bpf_ntohs(sv_version);
            *offset += 2;

            // Store the version
            set_tls_offered_version(tags, sv_version);

            sv_offset += 2;
        }
    } else {
        // ServerHello
        // Extension Length should be 2
        if (*offset + 2 > skb_len) {
            return false;
        }

        // Read selected version (2 bytes)
        __u16 selected_version;
        if (bpf_skb_load_bytes(skb, *offset, &selected_version, sizeof(selected_version)) < 0) {
            return false;
        }
        selected_version = bpf_ntohs(selected_version);
        *offset += 2;

        tags->chosen_version = selected_version;
    }

    return true;
}


static __always_inline bool parse_tls_extensions(struct __sk_buff *skb, __u64 *offset, __u64 extensions_end, tls_info_t *tags, bool is_client_hello) {
    __u16 extension_type;
    __u16 extension_length;

    #pragma unroll(MAX_EXTENSIONS)
    for (int i = 0; i < MAX_EXTENSIONS; i++) {
        if (*offset + 4 > extensions_end) {
            break;
        }
        // Read Extension Type (2 bytes)
        if (bpf_skb_load_bytes(skb, *offset, &extension_type, sizeof(extension_type)) < 0) {
            return false;
        }
        extension_type = bpf_ntohs(extension_type);
        *offset += 2;

        // Read Extension Length (2 bytes)
        if (bpf_skb_load_bytes(skb, *offset, &extension_length, sizeof(extension_length)) < 0) {
            return false;
        }
        extension_length = bpf_ntohs(extension_length);
        *offset += 2;

        // Ensure we don't read beyond the packet
        if (*offset + extension_length > skb->len || *offset + extension_length > extensions_end) {
            return false;
        }

        // Check for supported_versions extension
        if (extension_type == SUPPORTED_VERSIONS_EXTENSION) {
            if (!parse_supported_versions_extension(skb, offset, extensions_end, tags, is_client_hello)) {
                return false;
            }
        } else {
            // Skip other extensions
            *offset += extension_length;
        }

        // Ensure we don't run past the extensions_end
        if (*offset >= extensions_end) {
            break;
        }
    }

    return true;
}


// parse_client_hello reads the ClientHello message from the TLS handshake and populates select tags
static __always_inline bool parse_client_hello(struct __sk_buff *skb, __u64 offset, tls_info_t *tags) {
    __u32 handshake_length;
    __u16 client_version;
    __u32 skb_len = skb->len;

    // Parse the handshake header
    if (!parse_tls_handshake_header(skb, &offset, &handshake_length, &client_version)) {
        return false;
    }

    // Store client_version in tags (in case supported_versions extension is absent)
    set_tls_offered_version(tags, client_version);

    // TLS 1.2 is the highest version we will see in the header. If the connection is actually a higher version (1.3), 
    // it must be extracted from the extensions. Lower versions (1.0, 1.1) will not have extensions.
    if (client_version != TLS_VERSION12) {
        return true;
    }

    // Skip Random and Session ID
    if (!skip_random_and_session_id(skb, &offset)) {
        return false;
    }

    // Read Cipher Suites Length (2 bytes)
    if (offset + CIPHER_SUITES_LENGTH > skb_len) {
        return false;
    }
    __u16 cipher_suites_length;
    if (bpf_skb_load_bytes(skb, offset, &cipher_suites_length, sizeof(cipher_suites_length)) < 0) {
        return false;
    }
    cipher_suites_length = bpf_ntohs(cipher_suites_length);
    offset += CIPHER_SUITES_LENGTH;

    // Skip Cipher Suites
    offset += cipher_suites_length;

    // Read Compression Methods Length (1 byte)
    if (offset + COMPRESSION_METHODS_LENGTH > skb_len) {
        return false;
    }
    __u8 compression_methods_length;
    if (bpf_skb_load_bytes(skb, offset, &compression_methods_length, sizeof(compression_methods_length)) < 0) {
        return false;
    }
    offset += COMPRESSION_METHODS_LENGTH;

    // Skip Compression Methods
    offset += compression_methods_length;

    // Check if extensions are present
    if (offset + 2 > skb_len) {
        return false;
    }

    // Read Extensions Length (2 bytes)
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

    __u64 extensions_end = offset + extensions_length;

    // Parse Extensions
    return parse_tls_extensions(skb, &offset, extensions_end, tags, true);
}


// parse_server_hello reads the ServerHello message from the TLS handshake and populates select tags
static __always_inline bool parse_server_hello(struct __sk_buff *skb, __u64 offset, tls_info_t *tags) {
    __u32 handshake_length;
    __u16 server_version;
    __u32 skb_len = skb->len;

    // Parse the handshake header
    if (!parse_tls_handshake_header(skb, &offset, &handshake_length, &server_version)) {
        return false;
    }

    // Set the version here and try to get the "real" version from the extensions
    // Note: In TLS 1.3, the server_version field is set to 0x0303 (TLS 1.2)
    // The actual version is embedded in the supported_versions extension
    tags->chosen_version = server_version;

    // Skip Random and Session ID
    if (!skip_random_and_session_id(skb, &offset)) {
        return false;
    }

    // Read Cipher Suite (2 bytes)
    if (offset + CIPHER_SUITES_LENGTH > skb_len) {
        return false;
    }
    __u16 cipher_suite;
    if (bpf_skb_load_bytes(skb, offset, &cipher_suite, sizeof(cipher_suite)) < 0) {
        return false;
    }
    cipher_suite = bpf_ntohs(cipher_suite);
    offset += CIPHER_SUITES_LENGTH;

    // Skip Compression Method (1 byte)
    offset += COMPRESSION_METHODS_LENGTH;

    // Store parsed data into tags
    tags->cipher_suite = cipher_suite;

    // TLS 1.2 is the highest version we will see in the header. If the connection is actually a higher version (1.3), 
    // it must be extracted from the extensions. Lower versions (1.0, 1.1) will not have extensions.
    if (tags->chosen_version != TLS_VERSION12) {
        return true;
    }

    // Check if there are extensions
    if (offset + 2 > skb_len) {
        return false;
    }

    // Read Extensions Length (2 bytes)
    __u16 extensions_length;
    if (bpf_skb_load_bytes(skb, offset, &extensions_length, sizeof(extensions_length)) < 0) {
        return false;
    }
    extensions_length = bpf_ntohs(extensions_length);
    offset += 2;

    // Ensure we don't read beyond the packet
    __u64 handshake_end = offset + handshake_length;
    if (offset + extensions_length > skb_len || offset + extensions_length > handshake_end) {
        return false;
    }

    __u64 extensions_end = offset + extensions_length;

    // Parse Extensions
    return parse_tls_extensions(skb, &offset, extensions_end, tags, false);
}


static __always_inline bool is_tls_handshake_type(struct __sk_buff *skb, tls_record_header_t *tls_hdr, __u64 offset, __u8 expected_handshake_type) {
    if (!tls_hdr) {
        return false;
    }
    if (tls_hdr->content_type != TLS_HANDSHAKE) {
        return false;
    }

    // Read handshake type
    if (offset + 1 > skb->len) {
        return false;
    }  
    __u8 handshake_type;
    if (bpf_skb_load_bytes(skb, offset, &handshake_type, sizeof(handshake_type)) < 0) {
        return false;
    }

    return handshake_type == expected_handshake_type;
}

static __always_inline bool is_tls_handshake_client_hello(struct __sk_buff *skb, tls_record_header_t *tls_hdr, __u64 offset) {
    return is_tls_handshake_type(skb, tls_hdr, offset, TLS_HANDSHAKE_CLIENT_HELLO);
}

static __always_inline bool is_tls_handshake_server_hello(struct __sk_buff *skb, tls_record_header_t *tls_hdr, __u64 offset) {
    return is_tls_handshake_type(skb, tls_hdr, offset, TLS_HANDSHAKE_SERVER_HELLO);
}

#endif // __TLS_H
