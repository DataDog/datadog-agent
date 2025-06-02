#ifndef __TLS_H
#define __TLS_H

#include "tracer/tracer.h"

// TLS version constants (SSL versions are deprecated, included for completeness)
#define SSL_VERSION20 0x0200
#define SSL_VERSION30 0x0300
#define TLS_VERSION10 0x0301
#define TLS_VERSION11 0x0302
#define TLS_VERSION12 0x0303
#define TLS_VERSION13 0x0304

// TLS Content Types (https://www.rfc-editor.org/rfc/rfc5246#page-19 6.2. Record Layer)
#define TLS_HANDSHAKE          0x16
#define TLS_APPLICATION_DATA   0x17
#define TLS_CHANGE_CIPHER_SPEC 0x14
#define TLS_ALERT              0x15

// TLS Handshake Types
#define TLS_HANDSHAKE_CLIENT_HELLO 0x01
#define TLS_HANDSHAKE_SERVER_HELLO 0x02

// Bitmask constants for offered versions
#define TLS_VERSION10_BIT 1 << 0
#define TLS_VERSION11_BIT 1 << 1
#define TLS_VERSION12_BIT 1 << 2
#define TLS_VERSION13_BIT 1 << 3

// Maximum number of extensions to parse when looking for SUPPORTED_VERSIONS_EXTENSION
#define MAX_EXTENSIONS 16
// The supported_versions extension for TLS 1.3 is described in RFC 8446 Section 4.2.1
#define SUPPORTED_VERSIONS_EXTENSION 0x002B

// Maximum TLS record payload size (16 KB)
#define TLS_MAX_PAYLOAD_LENGTH (1 << 14)

// The following field lengths and message formats are defined by the TLS specifications
// For TLS 1.2 (and earlier) see:
// RFC 5246 - The Transport Layer Security (TLS) Protocol Version 1.2
// https://tools.ietf.org/html/rfc5246
// Particularly Section 7.4 details handshake messages and their fields, and Section 6.2.1
// covers the TLS record layer.
// For TLS 1.3, see:
// RFC 8446 - The Transport Layer Security (TLS) Protocol Version 1.3
// https://tools.ietf.org/html/rfc8446
// Many handshake structures are similar, but some extensions (like supported_versions) are defined here
#define TLS_HANDSHAKE_LENGTH          3  // Handshake length is 3 bytes (RFC 5246 Section 7.4)
#define TLS_HELLO_MESSAGE_HEADER_SIZE 4  // handshake_type(1) + length(3)
#define RANDOM_LENGTH                 32 // Random field length in Client/Server Hello (RFC 5246 Section 7.4.1.2)
#define PROTOCOL_VERSION_LENGTH       2  // Protocol version field is 2 bytes (RFC 5246 Section 6.2.1)
#define SESSION_ID_LENGTH             1  // Session ID length field is 1 byte (RFC 5246 Section 7.4.1.2)
#define CIPHER_SUITES_LENGTH          2  // Cipher Suites length field is 2 bytes (RFC 5246 Section 7.4.1.2)
#define COMPRESSION_METHODS_LENGTH    1  // Compression Methods length field is 1 byte (RFC 5246 Section 7.4.1.2)
#define EXTENSION_TYPE_LENGTH         2  // Extension Type field is 2 bytes (RFC 5246 Section 7.4.1.4)
#define EXTENSION_LENGTH_FIELD        2  // Extension Length field is 2 bytes (RFC 5246 Section 7.4.1.4)

// For single-byte fields (list lengths, etc.)
#define SINGLE_BYTE_LENGTH           1

// Minimum extension header length = Extension Type (2 bytes) + Extension Length (2 bytes) = 4 bytes
#define MIN_EXTENSION_HEADER_LENGTH (EXTENSION_TYPE_LENGTH + EXTENSION_LENGTH_FIELD)

// Maximum number of supported versions we unroll for (all TLS versions)
#define MAX_SUPPORTED_VERSIONS 4

// TLS record layer header structure (RFC 5246)
typedef struct {
    __u8 content_type;
    __u16 version;
    __u16 length;
} __attribute__((packed)) tls_record_header_t;

// Checks if the TLS version is valid
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

// TLS Record Header (RFC 5246 Section 6.2.1)
//
//  +---------+---------+---------+-----------+
//  | type(1) |     version(2)    | length(2) |
//  +---------+---------+---------+-----------+
//  type:    1 byte (TLS_CONTENT_TYPE)
//  version: 2 bytes (e.g., 0x03 0x03 for TLS 1.2)
//  length:  2 bytes (total number of payload bytes following this header)

// read_tls_record_header reads the TLS record header from the packet
// Reference: RFC 5246 Section 6.2.1 (Record Layer), https://tools.ietf.org/html/rfc5246#section-6.2.1
// Validates the record header fields (content_type, version, length) and checks for correctness within packet bounds.
static __always_inline bool read_tls_record_header(struct __sk_buff *skb, __u32 header_offset, __u32 data_end, tls_record_header_t *tls_hdr) {
    // Ensure there's enough space for TLS record header
    if (header_offset + sizeof(tls_record_header_t) > data_end) {
        return false;
    }

    // Read TLS record header
    if (bpf_skb_load_bytes(skb, header_offset, tls_hdr, sizeof(tls_record_header_t)) < 0) {
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
    return header_offset + sizeof(tls_record_header_t) + tls_hdr->length <= data_end;
}

// TLS Handshake Message Header (RFC 5246 Section 7.4)
//  +---------+---------+---------+---------+
//  | handshake_type(1) |   length(3 bytes) |
//  +---------+---------+---------+---------+
//
// The handshake_type identifies the handshake message (e.g., ClientHello, ServerHello).
// length indicates the size of the handshake message that follows (not including these 4 bytes).

// is_valid_tls_handshake checks if the TLS handshake message is valid
// The function expects the record to have already been validated. It further checks that the
// handshake_type and handshake_length are consistent.
static __always_inline bool is_valid_tls_handshake(struct __sk_buff *skb, __u32 header_offset, __u32 data_end, const tls_record_header_t *hdr) {
    // At this point, we know from read_tls_record_header() that:
    // - hdr->version is a valid TLS version
    // - hdr->length fits entirely within the packet (header_offset + hdr->length <= data_end)

    __u32 handshake_offset = header_offset + sizeof(tls_record_header_t);

    // Ensure we don't read beyond the packet
    if (handshake_offset + SINGLE_BYTE_LENGTH > data_end) {
        return false;
    }
    // Read handshake_type (1 byte)
    __u8 handshake_type;
    if (bpf_skb_load_bytes(skb, handshake_offset, &handshake_type, SINGLE_BYTE_LENGTH) < 0) {
        return false;
    }

    // Read handshake_length (3 bytes)
    __u32 length_offset = handshake_offset + SINGLE_BYTE_LENGTH;
    if (length_offset + TLS_HANDSHAKE_LENGTH > data_end) {
        return false;
    }
    __u8 handshake_length_bytes[TLS_HANDSHAKE_LENGTH];
    if (bpf_skb_load_bytes(skb, length_offset, handshake_length_bytes, TLS_HANDSHAKE_LENGTH) < 0) {
        return false;
    }

    __u32 handshake_length = (handshake_length_bytes[0] << 16) |
                             (handshake_length_bytes[1] << 8) |
                              handshake_length_bytes[2];

    // Verify that the handshake message length plus the 4-byte handshake header (1 byte type + 3 bytes length)
    // matches the total length defined in the record header.
    // If handshake_length + TLS_HELLO_MESSAGE_HEADER_SIZE != hdr->length, the handshake message structure is inconsistent.
    if (handshake_length + TLS_HELLO_MESSAGE_HEADER_SIZE != hdr->length) {
        return false;
    }

    // Check that the handshake_type is one of the expected values (ClientHello or ServerHello).
    // This ensures we are dealing with a known handshake message type.
    if (handshake_type != TLS_HANDSHAKE_CLIENT_HELLO && handshake_type != TLS_HANDSHAKE_SERVER_HELLO) {
        return false;
    }

    // At this point, we've confirmed:
    // - The handshake message fits within the record.
    // - The handshake_type is a known TLS Hello message.
    // - The handshake_length matches the record header's length.
    return true;
}

// is_tls checks if the packet is a TLS packet by reading and validating the TLS record header
// Reference: RFC 5246 Section 6.2.1 (Record Layer), https://tools.ietf.org/html/rfc5246#section-6.2.1
// Validates that content_type matches known TLS types (Handshake, Application Data, etc.).
static __always_inline bool is_tls(struct __sk_buff *skb, __u32 header_offset, __u32 data_end, tls_record_header_t *tls_hdr) {
    // Read and validate the TLS record header
    if (!read_tls_record_header(skb, header_offset, data_end, tls_hdr)) {
        return false;
    }

    switch (tls_hdr->content_type) {
    case TLS_HANDSHAKE:
        return is_valid_tls_handshake(skb, header_offset, data_end, tls_hdr);
    case TLS_APPLICATION_DATA:
    case TLS_CHANGE_CIPHER_SPEC:
    case TLS_ALERT:
        return true;
    default:
        return false;
    }
}

// parse_tls_handshake_header extracts handshake_length and protocol_version from a TLS handshake message
// References:
// - RFC 5246 Section 7.4 (Handshake Protocol Overview), https://tools.ietf.org/html/rfc5246#section-7.4
// For ClientHello and ServerHello, this includes parsing the handshake type (skipped prior) and the 3-byte length field, followed by a 2-byte protocol version field.
static __always_inline bool parse_tls_handshake_header(struct __sk_buff *skb, __u32 *offset, __u32 data_end, __u32 *handshake_length, __u16 *protocol_version) {
    *offset += SINGLE_BYTE_LENGTH; // Move past handshake type (1 byte)

    // Read handshake length (3 bytes)
    if (*offset + TLS_HANDSHAKE_LENGTH > data_end) {
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
    if (*offset + *handshake_length > data_end) {
        return false;
    }

    // Read protocol version (2 bytes)
    if (*offset + PROTOCOL_VERSION_LENGTH > data_end) {
        return false;
    }
    __u16 version;
    if (bpf_skb_load_bytes(skb, *offset, &version, PROTOCOL_VERSION_LENGTH) < 0) {
        return false;
    }
    *protocol_version = bpf_ntohs(version);
    *offset += PROTOCOL_VERSION_LENGTH;

    return true;
}

// skip_random_and_session_id Skips the Random (32 bytes) and the Session ID from the TLS Hello messages
// References:
// - RFC 5246 Section 7.4.1.2 (Client Hello and Server Hello): https://tools.ietf.org/html/rfc5246#section-7.4.1.2
// ClientHello and ServerHello contain a "random" field (32 bytes) followed by a "session_id_length" (1 byte)
// and a session_id of that length. This helper increments the offset accordingly after reading and skipping these fields.
static __always_inline bool skip_random_and_session_id(struct __sk_buff *skb, __u32 *offset, __u32 data_end) {
    // Skip Random (32 bytes)
    *offset += RANDOM_LENGTH;

    // Read Session ID Length (1 byte)
    if (*offset + SESSION_ID_LENGTH > data_end) {
        return false;
    }
    __u8 session_id_length;
    if (bpf_skb_load_bytes(skb, *offset, &session_id_length, SESSION_ID_LENGTH) < 0) {
        return false;
    }
    *offset += SESSION_ID_LENGTH;

    // Skip Session ID
    *offset += session_id_length;

    // Ensure we don't read beyond the packet
    return *offset <= data_end;
}

// parse_supported_versions_extension looks for the supported_versions extension in the ClientHello or ServerHello and populates tags
// References:
// - For TLS 1.3 supported_versions extension: RFC 8446 Section 4.2.1: https://tools.ietf.org/html/rfc8446#section-4.2.1
// For ClientHello this extension contains a list of supported versions (2 bytes each) preceded by a 1-byte length.
//  supported_versions extension structure:
//   +-----+--------------------+
//   | len(1) | versions(2 * N) |
//   +-----+--------------------+
// For ServerHello (TLS 1.3), it contains a single selected_version (2 bytes).
//   +---------------------+
//   | selected_version(2) |
//   +---------------------+
static __always_inline bool parse_supported_versions_extension(struct __sk_buff *skb, __u32 *offset, __u32 data_end, __u32 extensions_end, tls_info_t *tags, bool is_client_hello) {
    if (is_client_hello) {
        // Read supported version list length (1 byte)
        if (*offset + SINGLE_BYTE_LENGTH > data_end || *offset + SINGLE_BYTE_LENGTH > extensions_end) {
            return false;
        }
        __u8 sv_list_length;
        if (bpf_skb_load_bytes(skb, *offset, &sv_list_length, SINGLE_BYTE_LENGTH) < 0) {
            return false;
        }
        *offset += SINGLE_BYTE_LENGTH;

        if (*offset + sv_list_length > data_end || *offset + sv_list_length > extensions_end) {
            return false;
        }

        // Parse the list of supported versions (2 bytes each)
        __u8 sv_offset = 0;
        __u16 sv_version;
        #pragma unroll(MAX_SUPPORTED_VERSIONS)
        for (int idx = 0; idx < MAX_SUPPORTED_VERSIONS; idx++) {
            if (sv_offset + 1 >= sv_list_length) {
                break;
            }
            // Each supported version is 2 bytes
            if (*offset + PROTOCOL_VERSION_LENGTH > data_end) {
                return false;
            }

            if (bpf_skb_load_bytes(skb, *offset, &sv_version, PROTOCOL_VERSION_LENGTH) < 0) {
                return false;
            }
            sv_version = bpf_ntohs(sv_version);
            *offset += PROTOCOL_VERSION_LENGTH;

            set_tls_offered_version(tags, sv_version);
            sv_offset += PROTOCOL_VERSION_LENGTH;
        }
    } else {
        // ServerHello
        // The selected_version field is 2 bytes
        if (*offset + PROTOCOL_VERSION_LENGTH > data_end) {
            return false;
        }

        // Read selected version (2 bytes)
        __u16 selected_version;
        if (bpf_skb_load_bytes(skb, *offset, &selected_version, PROTOCOL_VERSION_LENGTH) < 0) {
            return false;
        }
        selected_version = bpf_ntohs(selected_version);
        *offset += PROTOCOL_VERSION_LENGTH;

        tags->chosen_version = selected_version;
    }

    return true;
}

// parse_tls_extensions parses TLS extensions in both ClientHello and ServerHello
// References:
// - RFC 5246 Section 7.4.1.4 (Hello Extensions): https://tools.ietf.org/html/rfc5246#section-7.4.1.4
// - For TLS 1.3 supported_versions extension: RFC 8446 Section 4.2.1: https://tools.ietf.org/html/rfc8446#section-4.2.1
// This function iterates over extensions, reading the extension_type and extension_length, and if it encounters 
// the supported_versions extension, it calls parse_supported_versions_extension to handle it.
// ASCII snippet for a single extension:
//   +---------+---------+--------------------------------+
//   | ext_type(2) | ext_length(2) | ext_data(ext_length) |
//   +---------+---------+--------------------------------+
// For multiple extensions, they are just concatenated one after another.
static __always_inline bool parse_tls_extensions(struct __sk_buff *skb, __u32 *offset, __u32 data_end, __u32 extensions_end, tls_info_t *tags, bool is_client_hello) {
    __u16 extension_type;
    __u16 extension_length;

    #pragma unroll(MAX_EXTENSIONS)
    for (int i = 0; i < MAX_EXTENSIONS; i++) {
        if (*offset + MIN_EXTENSION_HEADER_LENGTH > extensions_end) {
            break;
        }

        // Read Extension Type (2 bytes)
        if (bpf_skb_load_bytes(skb, *offset, &extension_type, EXTENSION_TYPE_LENGTH) < 0) {
            return false;
        }
        extension_type = bpf_ntohs(extension_type);
        *offset += EXTENSION_TYPE_LENGTH;

        // Read Extension Length (2 bytes)
        if (bpf_skb_load_bytes(skb, *offset, &extension_length, EXTENSION_LENGTH_FIELD) < 0) {
            return false;
        }
        extension_length = bpf_ntohs(extension_length);
        *offset += EXTENSION_LENGTH_FIELD;

        if (*offset + extension_length > data_end || *offset + extension_length > extensions_end) {
            return false;
        }

        if (extension_type == SUPPORTED_VERSIONS_EXTENSION) {
            if (!parse_supported_versions_extension(skb, offset, data_end, extensions_end, tags, is_client_hello)) {
                return false;
            }
        } else {
            // Skip other extensions
            *offset += extension_length;
        }

        if (*offset >= extensions_end) {
            break;
        }
    }

    return true;
}

// parse_client_hello parses the ClientHello message and populates tags
// Reference: RFC 5246 Section 7.4.1.2 (Client Hello), https://tools.ietf.org/html/rfc5246
// Structure (simplified):
// handshake_type (1 byte), length (3 bytes), version (2 bytes), random(32 bytes), session_id_length(1 byte), session_id(variable), cipher_suites_length(2 bytes), cipher_suites(variable), compression_methods_length(1 byte), compression_methods(variable), extensions_length(2 bytes), extensions(variable)
// After the handshake header (handshake_type + length), the ClientHello fields are:
// +----------------------------+
// | client_version (2)         |
// +----------------------------+
// | random (32)                |
// +----------------------------+
// | session_id_length (1)      |
// | session_id (...)           |
// +----------------------------+
// | cipher_suites_length(2)    |
// | cipher_suites(...)         |
// +----------------------------+
// | compression_methods_len(1) |
// | compression_methods(...)   |
// +----------------------------+
// | extensions_length (2)      |
// | extensions(...)            |
// +----------------------------+
static __always_inline bool parse_client_hello(struct __sk_buff *skb, __u32 offset, __u32 data_end, tls_info_t *tags) {
    __u32 handshake_length;
    __u16 client_version;

    if (!parse_tls_handshake_header(skb, &offset, data_end, &handshake_length, &client_version)) {
        return false;
    }

    set_tls_offered_version(tags, client_version);

    // TLS 1.2 is the highest version we will see in the header. If the connection is actually a higher version (1.3),
    // it must be extracted from the extensions. Lower versions (1.0, 1.1) will not have extensions.
    if (client_version != TLS_VERSION12) {
        return true;
    }

    if (!skip_random_and_session_id(skb, &offset, data_end)) {
        return false;
    }

    // Read Cipher Suites Length (2 bytes)
    if (offset + CIPHER_SUITES_LENGTH > data_end) {
        return false;
    }
    __u16 cipher_suites_length;
    if (bpf_skb_load_bytes(skb, offset, &cipher_suites_length, CIPHER_SUITES_LENGTH) < 0) {
        return false;
    }
    cipher_suites_length = bpf_ntohs(cipher_suites_length);
    offset += CIPHER_SUITES_LENGTH;

    // Skip Cipher Suites
    offset += cipher_suites_length;

    // Read Compression Methods Length (1 byte)
    if (offset + COMPRESSION_METHODS_LENGTH > data_end) {
        return false;
    }
    __u8 compression_methods_length;
    if (bpf_skb_load_bytes(skb, offset, &compression_methods_length, COMPRESSION_METHODS_LENGTH) < 0) {
        return false;
    }
    offset += COMPRESSION_METHODS_LENGTH;

    // Skip Compression Methods
    offset += compression_methods_length;

    // Check if extensions are present
    if (offset + EXTENSION_LENGTH_FIELD > data_end) {
        return false;
    }

    // Read Extensions Length (2 bytes)
    __u16 extensions_length;
    if (bpf_skb_load_bytes(skb, offset, &extensions_length, EXTENSION_LENGTH_FIELD) < 0) {
        return false;
    }
    extensions_length = bpf_ntohs(extensions_length);
    offset += EXTENSION_LENGTH_FIELD;

    if (offset + extensions_length > data_end) {
        return false;
    }

    __u32 extensions_end = offset + extensions_length;

    return parse_tls_extensions(skb, &offset, data_end, extensions_end, tags, true);
}

// parse_server_hello parses the ServerHello message and populates tags
// Reference: RFC 5246 Section 7.4.1.2 (Server Hello), https://tools.ietf.org/html/rfc5246
// Structure (simplified):
// handshake_type(1), length(3), version(2), random(32), session_id_length(1), session_id(variable), cipher_suite(2), compression_method(1), extensions_length(2), extensions(variable)
// After the handshake header (handshake_type + length), the ServerHello fields are:
// +------------------------+
// | server_version (2)     |
// +------------------------+
// | random (32)            |
// +------------------------+
// | session_id_length (1)  |
// | session_id (...)       |
// +------------------------+
// | cipher_suite (2)       |
// +------------------------+
// | compression_method (1) |
// +------------------------+
// | extensions_length(2)   |
// | extensions(...)        |
// +------------------------+
static __always_inline bool parse_server_hello(struct __sk_buff *skb, __u32 offset, __u32 data_end, tls_info_t *tags) {
    __u32 handshake_length;
    __u16 server_version;

    if (!parse_tls_handshake_header(skb, &offset, data_end, &handshake_length, &server_version)) {
        return false;
    }

    // Set the version here and try to get the "real" version from the extensions if possible
    // Note: In TLS 1.3, the server_version field is set to 1.2
    // The actual version is embedded in the supported_versions extension
    tags->chosen_version = server_version;

    if (!skip_random_and_session_id(skb, &offset, data_end)) {
        return false;
    }

    // Read Cipher Suite (2 bytes)
    if (offset + CIPHER_SUITES_LENGTH > data_end) {
        return false;
    }
    __u16 cipher_suite;
    if (bpf_skb_load_bytes(skb, offset, &cipher_suite, CIPHER_SUITES_LENGTH) < 0) {
        return false;
    }
    cipher_suite = bpf_ntohs(cipher_suite);
    offset += CIPHER_SUITES_LENGTH;

    // Skip Compression Method (1 byte)
    offset += COMPRESSION_METHODS_LENGTH;

    tags->cipher_suite = cipher_suite;

    // TLS 1.2 is the highest version we will see in the header. If the connection is actually a higher version (1.3),
    // it must be extracted from the extensions. Lower versions (1.0, 1.1) will not have extensions.
    if (tags->chosen_version != TLS_VERSION12) {
        return true;
    }

    if (offset + EXTENSION_LENGTH_FIELD > data_end) {
        return false;
    }

    // Read Extensions Length (2 bytes)
    __u16 extensions_length;
    if (bpf_skb_load_bytes(skb, offset, &extensions_length, EXTENSION_LENGTH_FIELD) < 0) {
        return false;
    }
    extensions_length = bpf_ntohs(extensions_length);
    offset += EXTENSION_LENGTH_FIELD;

    __u32 handshake_end = offset + handshake_length;
    if (offset + extensions_length > data_end || offset + extensions_length > handshake_end) {
        return false;
    }

    __u32 extensions_end = offset + extensions_length;

    return parse_tls_extensions(skb, &offset, data_end, extensions_end, tags, false);
}

// is_tls_handshake_type checks if the handshake type at the given offset matches the expected type (e.g., ClientHello or ServerHello)
// References:
// - RFC 5246 Section 7.4 (Handshake Protocol Overview), https://tools.ietf.org/html/rfc5246#section-7.4
// The handshake_type is a single byte enumerated value.
static __always_inline bool is_tls_handshake_type(struct __sk_buff *skb, __u32 offset, __u32 data_end, __u8 expected_handshake_type) {
    // The handshake type is a single byte enumerated value
    if (offset + SINGLE_BYTE_LENGTH > data_end) {
        return false;
    }
    __u8 handshake_type;
    if (bpf_skb_load_bytes(skb, offset, &handshake_type, SINGLE_BYTE_LENGTH) < 0) {
        return false;
    }

    return handshake_type == expected_handshake_type;
}

// is_tls_handshake_client_hello checks if the packet is a TLS ClientHello message
static __always_inline bool is_tls_handshake_client_hello(struct __sk_buff *skb, __u32 offset, __u32 data_end) {
    return is_tls_handshake_type(skb, offset, data_end, TLS_HANDSHAKE_CLIENT_HELLO);
}

// is_tls_handshake_server_hello checks if the packet is a TLS ServerHello message
static __always_inline bool is_tls_handshake_server_hello(struct __sk_buff *skb, __u32 offset, __u32 data_end) {
    return is_tls_handshake_type(skb, offset, data_end, TLS_HANDSHAKE_SERVER_HELLO);
}

#endif // __TLS_H
