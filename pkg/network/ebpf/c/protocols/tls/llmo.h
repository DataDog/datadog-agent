#ifndef __LLMO_H
#define __LLMO_H

#include "ktypes.h"
#include "bpf_builtins.h"
#include "bpf_telemetry.h"
#include "map-defs.h"
#include "conn_tuple.h"

#include "protocols/read_into_buffer.h"

// LLM Observability (LLMO) PoC: capture a fixed-size window of the decrypted
// request body for connections that userspace has flagged as LLM traffic.

#define LLM_BODY_BUFFER_SIZE 512

// llm_conn_key_t mirrors pkg/network/types.ConnectionKey (4x u64 + 2x u16) so
// that userspace can build the exact same key from a transaction's ConnTuple().
typedef struct {
    __u64 saddr_h;
    __u64 saddr_l;
    __u64 daddr_h;
    __u64 daddr_l;
    __u16 sport;
    __u16 dport;
    // Explicit padding so the layout (40 bytes) is deterministic and matches
    // the userspace key; it is always zeroed so map lookups byte-match.
    __u32 _pad;
} llm_conn_key_t;

typedef struct {
    __u32 len;
    __u8 data[LLM_BODY_BUFFER_SIZE];
} llm_body_t;

// Connections flagged by userspace as LLM traffic; gates body capture.
BPF_HASH_MAP(llm_monitored_connections, llm_conn_key_t, __u8, 1024)
// Latest captured request body per LLM connection (read by userspace).
BPF_HASH_MAP(llm_request_bodies, llm_conn_key_t, llm_body_t, 1024)
// Latest captured response body TAIL per LLM connection (read by userspace).
// The token usage object lives near the end of the response JSON.
BPF_HASH_MAP(llm_response_bodies, llm_conn_key_t, llm_body_t, 1024)
// Per-CPU scratch to build the body off-stack (avoids the 512B stack limit).
BPF_PERCPU_ARRAY_MAP(llm_body_scratch, llm_body_t, 1)

READ_INTO_USER_BUFFER(llmo, LLM_BODY_BUFFER_SIZE)

// llmo_maybe_capture_body copies up to LLM_BODY_BUFFER_SIZE bytes of the
// decrypted request buffer into llm_request_bodies, but only for connections
// userspace has marked as LLM traffic in llm_monitored_connections.
static __always_inline void llmo_maybe_capture_body(conn_tuple_t *t, char *buffer, __u64 len) {
    llm_conn_key_t key;
    // Zero the whole key (including padding) so it byte-matches the key written
    // by userspace, which is required for the hash map lookup to hit.
    bpf_memset(&key, 0, sizeof(key));
    key.saddr_h = t->saddr_h;
    key.saddr_l = t->saddr_l;
    key.daddr_h = t->daddr_h;
    key.daddr_l = t->daddr_l;
    key.sport = t->sport;
    key.dport = t->dport;

    log_debug("[llmo] write hook sport=%u dport=%u", key.sport, key.dport);
    if (bpf_map_lookup_elem(&llm_monitored_connections, &key) == NULL) {
        log_debug("[llmo] gate MISS sport=%u dport=%u", key.sport, key.dport);
        return;
    }
    log_debug("[llmo] gate HIT, capturing len=%llu", len);

    const __u32 zero = 0;
    llm_body_t *body = bpf_map_lookup_elem(&llm_body_scratch, &zero);
    if (body == NULL) {
        return;
    }

    body->len = len < LLM_BODY_BUFFER_SIZE ? len : LLM_BODY_BUFFER_SIZE;
    read_into_user_buffer_llmo((char *)body->data, buffer);
    bpf_map_update_with_telemetry(llm_request_bodies, &key, body, BPF_ANY);
    log_debug("[llmo] body stored len=%u", body->len);
}

// llmo_maybe_capture_response captures the TAIL of the decrypted response
// buffer for LLM-flagged connections. The token usage object is near the end
// of the response JSON, so we grab the last LLM_BODY_BUFFER_SIZE bytes.
// Minimum response read size to capture. HTTP/2 reads include tiny (9-byte)
// frame headers and control frames; without this floor a trailing tiny read
// would overwrite the usage-bearing chunk in llm_response_bodies.
#define LLM_MIN_RESPONSE_CAPTURE 32

static __always_inline void llmo_maybe_capture_response(conn_tuple_t *t, char *buffer, __u64 len) {
    if (len < LLM_MIN_RESPONSE_CAPTURE) {
        return;
    }

    llm_conn_key_t key;
    bpf_memset(&key, 0, sizeof(key));
    key.saddr_h = t->saddr_h;
    key.saddr_l = t->saddr_l;
    key.daddr_h = t->daddr_h;
    key.daddr_l = t->daddr_l;
    key.sport = t->sport;
    key.dport = t->dport;

    if (bpf_map_lookup_elem(&llm_monitored_connections, &key) == NULL) {
        return;
    }

    const __u32 zero = 0;
    llm_body_t *body = bpf_map_lookup_elem(&llm_body_scratch, &zero);
    if (body == NULL) {
        return;
    }

    __u64 off = len > LLM_BODY_BUFFER_SIZE ? len - LLM_BODY_BUFFER_SIZE : 0;
    body->len = len < LLM_BODY_BUFFER_SIZE ? len : LLM_BODY_BUFFER_SIZE;
    read_into_user_buffer_llmo((char *)body->data, buffer + off);
    bpf_map_update_with_telemetry(llm_response_bodies, &key, body, BPF_ANY);
    log_debug("[llmo] response stored len=%u off=%llu", body->len, off);
}

#endif
