#ifndef __TLS_MISCLASSIFICATION_DIAG_TYPES_H
#define __TLS_MISCLASSIFICATION_DIAG_TYPES_H

#include "ktypes.h"

// Types for the TLS-reported-as-plaintext diagnostics. Kept separate from
// tls-misclassification-diag.h (which declares the BPF map and helpers) so this file can be
// included by kprobe_types.go for cgo -godefs, where the BPF map machinery and kernel uapi headers
// are not available. Same split as tls-certs-types.h vs tls-certs.h.

// Reasons a diagnostic event is recorded. Keep in sync with the tlsDiag* constants in
// pkg/network/tracer/connection/ebpf_tracer.go.
typedef enum {
    // A plausible TLS record header was seen whose record ran past the end of the packet, so
    // is_tls() returned false on genuine TLS bytes (link 1).
    TLS_DIAG_RECORD_EXCEEDS_PACKET = 1,
    // An app-layer/db classifier matched a buffer that begins with a plausible TLS record
    // header — the classification is almost certainly a false positive on ciphertext (link 2).
    TLS_DIAG_APPLAYER_ON_TLS_PAYLOAD = 2,
    // Redis was matched on a port Redis never serves. Classification is purely content-based
    // (there are no port heuristics in it), so this is a false positive by definition (link 2).
    TLS_DIAG_REDIS_NONSTANDARD_PORT = 3,
    // A plausible TLS record header arrived on a connection whose app layer was already
    // recorded, so is_tls() can never run again for it (link 3).
    TLS_DIAG_LOCKED_OUT_BY_APPLAYER = 4,
    // A handshake record fit entirely within the packet, yet is_tls() still rejected it because
    // is_valid_tls_handshake() failed. That happens when the handshake type is anything other
    // than ClientHello or ServerHello (Certificate, ServerKeyExchange, Finished, ...), when a
    // record carries several coalesced handshake messages so handshake_length + 4 != record
    // length, or when one handshake message is fragmented across records. Distinguishes
    // "rejected for size" (link 1) from "rejected for structure", which is the path by which a
    // brand-new connection can be affected.
    TLS_DIAG_HANDSHAKE_INVALID = 5,
} tls_diag_reason_t;

typedef struct {
    // Time the entry was first created, from bpf_ktime_get_ns().
    __u64 timestamp;
    // How many times this (tuple, reason) pair has fired since the entry was created. Lets
    // userspace log once per connection while still reporting magnitude.
    __u32 hits;
    __u16 sport;
    __u16 dport;
    // The protocol_t already recorded at the application layer, or 0 if none.
    __u16 app_layer_proto;
    // TLS record content_type observed, when the reason involves a TLS header.
    __u8 tls_content_type;
    // A tls_diag_reason_t value.
    __u8 reason;
    // Whether userspace has already logged this entry, so repeat drains stay quiet.
    __u8 logged;
    __u8 _pad[3];
} tls_diag_event_t;

#endif // __TLS_MISCLASSIFICATION_DIAG_TYPES_H
