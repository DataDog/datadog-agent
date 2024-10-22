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

// TLS enhanced tags structures
typedef struct {
    __u16 offered_versions[6];
    __u8 num_offered_versions;
} tls_client_tags_t;

typedef struct {
    __u16 version;
    __u16 cipher_suite;
} tls_server_tags_t;

typedef struct {
    tls_client_tags_t client_tags;
    tls_server_tags_t server_tags;
} tls_enhanced_tags_t;

#define TLS_HANDSHAKE_CLIENT_HELLO 0x01
#define TLS_HANDSHAKE_SERVER_HELLO 0x02

// Function to check if the given version is a valid TLS version
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

// Helper function to read and validate the TLS record header
static __always_inline bool read_tls_record_header(struct __sk_buff *skb, __u64 nh_off, tls_record_header_t *tls_hdr) {
    __u32 skb_len = skb->len;

    // Ensure there's enough space for TLS record header
    if (nh_off + sizeof(tls_record_header_t) > skb_len)
        return false;

    // Read TLS record header
    if (bpf_skb_load_bytes(skb, nh_off, tls_hdr, sizeof(tls_record_header_t)) < 0)
        return false;

    // Convert fields to host byte order
    tls_hdr->version = bpf_ntohs(tls_hdr->version);
    tls_hdr->length = bpf_ntohs(tls_hdr->length);

    // Validate version and length
    if (!is_valid_tls_version(tls_hdr->version))
        return false;
    if (tls_hdr->length > TLS_MAX_PAYLOAD_LENGTH)
        return false;

    // Ensure we don't read beyond the packet
    if (nh_off + sizeof(tls_record_header_t) + tls_hdr->length > skb_len)
        return false;

    return true;
}

// Function to check if the packet is TLS
static __always_inline bool is_tls(struct __sk_buff *skb, __u64 nh_off, tls_record_header_t *tls_hdr) {
    // Use the helper function to read and validate the TLS record header
    if (!read_tls_record_header(skb, nh_off, tls_hdr))
        return false;

    // Validate content type
    if (tls_hdr->content_type != TLS_HANDSHAKE && tls_hdr->content_type != TLS_APPLICATION_DATA)
        return false;

    return true;
}

// Function to parse ClientHello message
static __always_inline int parse_client_hello(struct __sk_buff *skb, __u64 offset, __u32 skb_len, tls_enhanced_tags_t *tags) {
    // Move offset past handshake type (1 byte)
    offset += 1;

    // Read handshake length (3 bytes)
    __u8 handshake_length_bytes[3];
    if (bpf_skb_load_bytes(skb, offset, handshake_length_bytes, 3) < 0)
        return -1;
    __u32 handshake_length = (handshake_length_bytes[0] << 16) |
                             (handshake_length_bytes[1] << 8) |
                             (handshake_length_bytes[2]);
    offset += 3;

    // Ensure we don't read beyond the packet
    if (offset + handshake_length > skb_len)
        return -1;

    // Read client_version (2 bytes)
    __u16 client_version;
    if (bpf_skb_load_bytes(skb, offset, &client_version, sizeof(client_version)) < 0)
        return -1;
    client_version = bpf_ntohs(client_version);
    offset += 2;

    // Skip Random (32 bytes)
    offset += 32;

    // Read Session ID Length (1 byte)
    __u8 session_id_length;
    if (bpf_skb_load_bytes(skb, offset, &session_id_length, sizeof(session_id_length)) < 0)
        return -1;
    offset += 1;

    // Skip Session ID
    offset += session_id_length;

    // Read Cipher Suites Length (2 bytes)
    __u16 cipher_suites_length;
    if (bpf_skb_load_bytes(skb, offset, &cipher_suites_length, sizeof(cipher_suites_length)) < 0)
        return -1;
    cipher_suites_length = bpf_ntohs(cipher_suites_length);
    offset += 2;

    // Skip Cipher Suites
    offset += cipher_suites_length;

    // Read Compression Methods Length (1 byte)
    __u8 compression_methods_length;
    if (bpf_skb_load_bytes(skb, offset, &compression_methods_length, sizeof(compression_methods_length)) < 0)
        return -1;
    offset += 1;

    // Skip Compression Methods
    offset += compression_methods_length;

    // Check if extensions are present
    if (offset + 2 > skb_len)
        return -1;

    // Read Extensions Length (2 bytes)
    __u16 extensions_length;
    if (bpf_skb_load_bytes(skb, offset, &extensions_length, sizeof(extensions_length)) < 0)
        return -1;
    extensions_length = bpf_ntohs(extensions_length);
    offset += 2;

    // Ensure we don't read beyond the packet
    if (offset + extensions_length > skb_len)
        return -1;

    // Parse Extensions
    __u64 extensions_end = offset + extensions_length;
    #define MAX_EXTENSIONS 16
    __u8 extensions_parsed = 0;

    while (offset + 4 <= extensions_end && extensions_parsed < MAX_EXTENSIONS) {
        // Read Extension Type (2 bytes)
        __u16 extension_type;
        if (bpf_skb_load_bytes(skb, offset, &extension_type, sizeof(extension_type)) < 0)
            return -1;
        extension_type = bpf_ntohs(extension_type);
        offset += 2;

        // Read Extension Length (2 bytes)
        __u16 extension_length;
        if (bpf_skb_load_bytes(skb, offset, &extension_length, sizeof(extension_length)) < 0)
            return -1;
        extension_length = bpf_ntohs(extension_length);
        offset += 2;

        // Ensure we don't read beyond the packet
        if (offset + extension_length > skb_len || offset + extension_length > extensions_end)
            return -1;

        // Check for supported_versions extension (type 43 or 0x002B)
        if (extension_type == 0x002B) {
            // Parse supported_versions extension
            if (offset + 1 > skb_len)
                return -1;

            // Read list length (1 byte)
            __u8 sv_list_length;
            if (bpf_skb_load_bytes(skb, offset, &sv_list_length, sizeof(sv_list_length)) < 0)
                return -1;
            offset += 1;

            // Ensure we don't read beyond the packet
            if (offset + sv_list_length > skb_len || offset + sv_list_length > extensions_end)
                return -1;

            // Parse versions
            __u8 num_versions = 0;
            #define MAX_SUPPORTED_VERSIONS 6
            for (__u8 i = 0; i + 1 < sv_list_length && num_versions < MAX_SUPPORTED_VERSIONS; i += 2, num_versions++) {
                __u16 sv_version;
                if (bpf_skb_load_bytes(skb, offset, &sv_version, sizeof(sv_version)) < 0)
                    return -1;
                sv_version = bpf_ntohs(sv_version);
                offset += 2;

                tags->client_tags.offered_versions[num_versions] = sv_version;
            }
            tags->client_tags.num_offered_versions = num_versions;
        } else {
            // Skip other extensions
            offset += extension_length;
        }

        extensions_parsed++;
    }

    return 0;
}

// Function to parse ServerHello message
static __always_inline int parse_server_hello(struct __sk_buff *skb, __u64 offset, __u32 skb_len, tls_enhanced_tags_t *tags) {
    // Move offset past handshake type (1 byte)
    offset += 1;

    // Read handshake length (3 bytes)
    __u8 handshake_length_bytes[3];
    if (bpf_skb_load_bytes(skb, offset, handshake_length_bytes, 3) < 0)
        return -1;
    __u32 handshake_length = (handshake_length_bytes[0] << 16) |
                             (handshake_length_bytes[1] << 8) |
                             (handshake_length_bytes[2]);
    offset += 3;

    // Ensure we don't read beyond the packet
    if (offset + handshake_length > skb_len)
        return -1;

    __u64 handshake_end = offset + handshake_length;

    // Read server_version (2 bytes)
    __u16 server_version;
    if (bpf_skb_load_bytes(skb, offset, &server_version, sizeof(server_version)) < 0)
        return -1;
    server_version = bpf_ntohs(server_version);
    // Set the version here and try to get the "real" version from the extensions
    // Note: In TLS 1.3, the server_version field is set to 0x0303 (TLS 1.2)
    // The actual version is embedded in the supported_versions extension
    tags->server_tags.version = server_version;
    offset += 2;

    // Skip Random (32 bytes)
    offset += 32;

    // Read Session ID Length (1 byte)
    __u8 session_id_length;
    if (bpf_skb_load_bytes(skb, offset, &session_id_length, sizeof(session_id_length)) < 0)
        return -1;
    offset += 1;

    // Skip Session ID
    offset += session_id_length;

    // Read Cipher Suite (2 bytes)
    __u16 cipher_suite;
    if (bpf_skb_load_bytes(skb, offset, &cipher_suite, sizeof(cipher_suite)) < 0)
        return -1;
    cipher_suite = bpf_ntohs(cipher_suite);
    offset += 2;

    // Skip Compression Method (1 byte)
    offset += 1;

    // Store parsed data into tags
    tags->server_tags.cipher_suite = cipher_suite;

    // Check if there are extensions
    if (offset < handshake_end) {
        // Read Extensions Length (2 bytes)
        if (offset + 2 > skb_len || offset + 2 > handshake_end)
            return -1;
        __u16 extensions_length;
        if (bpf_skb_load_bytes(skb, offset, &extensions_length, sizeof(extensions_length)) < 0)
            return -1;
        extensions_length = bpf_ntohs(extensions_length);
        offset += 2;

        // Ensure we don't read beyond the packet
        if (offset + extensions_length > skb_len || offset + extensions_length > handshake_end)
            return -1;

        // Parse Extensions
        __u64 extensions_end = offset + extensions_length;
        #define MAX_EXTENSIONS 16
        __u8 extensions_parsed = 0;

        while (offset + 4 <= extensions_end && extensions_parsed < MAX_EXTENSIONS) {
            // Read Extension Type (2 bytes)
            __u16 extension_type;
            if (bpf_skb_load_bytes(skb, offset, &extension_type, sizeof(extension_type)) < 0)
                return -1;
            extension_type = bpf_ntohs(extension_type);
            offset += 2;

            // Read Extension Length (2 bytes)
            __u16 extension_length;
            if (bpf_skb_load_bytes(skb, offset, &extension_length, sizeof(extension_length)) < 0)
                return -1;
            extension_length = bpf_ntohs(extension_length);
            offset += 2;

            // Ensure we don't read beyond the packet
            if (offset + extension_length > skb_len || offset + extension_length > extensions_end)
                return -1;

            // Check for supported_versions extension (type 43 or 0x002B)
            if (extension_type == 0x002B) {
                // Parse supported_versions extension
                if (extension_length != 2)
                    return -1;

                if (offset + 2 > skb_len)
                    return -1;

                // Read selected version (2 bytes)
                __u16 selected_version;
                if (bpf_skb_load_bytes(skb, offset, &selected_version, sizeof(selected_version)) < 0)
                    return -1;
                selected_version = bpf_ntohs(selected_version);
                offset += 2;

                tags->server_tags.version = selected_version;
            } else {
                // Skip other extensions
                offset += extension_length;
            }

            extensions_parsed++;
        }
    }

    return 0;
}

// Function to parse the TLS payload and update tls_enhanced_tags_t
static __always_inline int parse_tls_payload(struct __sk_buff *skb, __u64 nh_off, tls_record_header_t *tls_hdr, tls_enhanced_tags_t *tags) {
    // At this point, tls_hdr has already been validated and filled by is_tls()
    __u64 offset = nh_off + sizeof(tls_record_header_t);

    if (tls_hdr->content_type == TLS_HANDSHAKE) {
        __u8 handshake_type;
        if (bpf_skb_load_bytes(skb, offset, &handshake_type, sizeof(handshake_type)) < 0)
            return -1;

        if (handshake_type == TLS_HANDSHAKE_CLIENT_HELLO) {
            log_debug("adamk tls classification: client hello");
            return parse_client_hello(skb, offset, skb->len, tags);
        } else if (handshake_type == TLS_HANDSHAKE_SERVER_HELLO) {
            log_debug("adamk tls classification: server hello");
            return parse_server_hello(skb, offset, skb->len, tags);
        } else {
            return -1;
        }
    } else {
        return -1;
    }
}

#endif // __TLS_H
