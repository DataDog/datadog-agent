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

#define LLM_BODY_BUFFER_SIZE 1024

// The request body (the user's prompt + system message) can be far larger than
// a response tail, so it gets its own, much bigger capture window. This covers
// full prompts up to one HTTP/2 DATA frame (the client's default max frame
// size), i.e. essentially all real inputs, instead of the 1 KB response window.
#define LLM_REQ_BUFFER_SIZE 16384

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

// llm_req_body_t is the larger request-body capture buffer.
typedef struct {
    __u32 len;
    __u8 data[LLM_REQ_BUFFER_SIZE];
} llm_req_body_t; // retained for the userspace struct size; body is streamed via llm_request_events

// Connections flagged by userspace as LLM traffic; gates body capture.
BPF_HASH_MAP(llm_monitored_connections, llm_conn_key_t, __u8, 1024)
// Latest captured response body TAIL per LLM connection (read by userspace).
// The token usage object lives near the end of the response JSON.
BPF_HASH_MAP(llm_response_bodies, llm_conn_key_t, llm_body_t, 1024)
// Latest captured response body HEAD per LLM connection. The assistant's
// message content lives near the start of the response JSON.
BPF_HASH_MAP(llm_response_heads, llm_conn_key_t, llm_body_t, 1024)
// Per-CPU scratch to build the body off-stack (avoids the 512B stack limit).
BPF_PERCPU_ARRAY_MAP(llm_body_scratch, llm_body_t, 1)

// llm_req_event_t streams each captured request body to userspace as it is
// written. Streaming (vs a single per-connection map slot) means a connection
// firing several requests in quick succession — e.g. both turns of a tool
// conversation — no longer overwrites the previous body before userspace reads
// it: every request body is delivered, in order.
typedef struct {
    llm_conn_key_t key;
    __u32 stream_id; // HTTP/2 stream id (from the request's frame header)
    __u32 pid;       // client PID that wrote the request (for service resolution)
    __u32 len;
    __u32 _pad;
    __u8 data[LLM_REQ_BUFFER_SIZE];
} llm_req_event_t;
BPF_RINGBUF_MAP(llm_request_events, 1 << 21)
BPF_PERCPU_ARRAY_MAP(llm_req_event_scratch, llm_req_event_t, 1)

// llm_resp_event_t streams a large window from the START of each response to
// userspace as it completes. A continuous consumer sees every turn in order —
// unlike the poll-batched map reads, which only ever see the latest response
// and so lose intermediate turns (e.g. a tool-call generation before its
// follow-up). The window is the request-sized buffer so it captures the full
// assistant answer (near the start) and, for responses up to that size, the
// usage/finish fields (near the end) too.
typedef struct {
    llm_conn_key_t key;
    __u32 stream_id;  // HTTP/2 stream id (from the response's first frame header)
    __u32 end_stream; // 1 if this read starts with a frame carrying END_STREAM
    __u32 len;
    __u32 _pad;
    __u8 data[LLM_REQ_BUFFER_SIZE];
} llm_resp_event_t;
BPF_RINGBUF_MAP(llm_response_events, 1 << 21)
BPF_PERCPU_ARRAY_MAP(llm_event_scratch, llm_resp_event_t, 1)

// bpf_memset (used by READ_INTO_USER_BUFFER) can only unroll up to ~512 bytes,
// so we read the LLM_BODY_BUFFER_SIZE window in LLM_BODY_CHUNK-sized chunks.
#define LLM_BODY_CHUNK 512

// Minimum read/write size to capture. HTTP/2 writes/reads include tiny control
// frames (WINDOW_UPDATE, PING, SETTINGS, 9-byte frame headers); without this
// floor a trailing tiny frame would overwrite the JSON body we captured.
#define LLM_MIN_CAPTURE 32
READ_INTO_USER_BUFFER(llmo, LLM_BODY_CHUNK)

// llmo_read_body reads LLM_BODY_BUFFER_SIZE bytes from src into dst, in
// LLM_BODY_CHUNK chunks (LLM_BODY_BUFFER_SIZE must be a multiple of the chunk).
static __always_inline void llmo_read_body(__u8 *dst, char *src) {
#pragma unroll
    for (int i = 0; i < LLM_BODY_BUFFER_SIZE / LLM_BODY_CHUNK; i++) {
        read_into_user_buffer_llmo((char *)dst + i * LLM_BODY_CHUNK, src + i * LLM_BODY_CHUNK);
    }
}

// llmo_read_req_body reads the larger LLM_REQ_BUFFER_SIZE request window.
static __always_inline void llmo_read_req_body(__u8 *dst, char *src) {
#pragma unroll
    for (int i = 0; i < LLM_REQ_BUFFER_SIZE / LLM_BODY_CHUNK; i++) {
        read_into_user_buffer_llmo((char *)dst + i * LLM_BODY_CHUNK, src + i * LLM_BODY_CHUNK);
    }
}

// llmo_frame_hdr reads the 9-byte HTTP/2 frame header at the start of buffer and
// extracts the stream id (low 31 bits of the last 4 bytes) and the END_STREAM
// flag (defined only for DATA and HEADERS frames). Best-effort: meaningful only
// when buffer begins at a frame boundary — the first write of a request or the
// first read of a response, which is the common case. A mid-frame continuation
// read yields a bogus id/flag the caller treats as "same/current stream".
static __always_inline void llmo_frame_hdr(char *buffer, __u32 *stream_id, __u32 *end_stream) {
    *stream_id = 0;
    *end_stream = 0;
    __u8 hdr[9];
    if (bpf_probe_read_user(hdr, sizeof(hdr), buffer) < 0) {
        return;
    }
    *stream_id = ((__u32)(hdr[5] & 0x7f) << 24) | ((__u32)hdr[6] << 16) | ((__u32)hdr[7] << 8) | (__u32)hdr[8];
    __u8 ftype = hdr[3];
    __u8 fflags = hdr[4];
    if ((ftype == 0x00 || ftype == 0x01) && (fflags & 0x01)) {
        *end_stream = 1;
    }
}

// llmo_maybe_capture_body copies up to LLM_BODY_BUFFER_SIZE bytes of the
// decrypted request buffer into llm_request_bodies, but only for connections
// userspace has marked as LLM traffic in llm_monitored_connections.
static __always_inline void llmo_maybe_capture_body(conn_tuple_t *t, char *buffer, __u64 len) {
    // Skip tiny writes (HTTP/2 control frames) so they don't overwrite the
    // captured request JSON body.
    if (len < LLM_MIN_CAPTURE) {
        return;
    }

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
    llm_req_event_t *ev = bpf_map_lookup_elem(&llm_req_event_scratch, &zero);
    if (ev == NULL) {
        return;
    }

    ev->key = key;
    __u32 sid = 0, eos = 0;
    llmo_frame_hdr(buffer, &sid, &eos);
    ev->stream_id = sid;
    ev->pid = bpf_get_current_pid_tgid() >> 32;
    ev->len = len < LLM_REQ_BUFFER_SIZE ? len : LLM_REQ_BUFFER_SIZE;
    llmo_read_req_body(ev->data, buffer);
    bpf_ringbuf_output(&llm_request_events, ev, sizeof(*ev), 0);
    log_debug("[llmo] req streamed len=%u stream=%u pid=%u", ev->len, ev->stream_id, ev->pid);
}

// llmo_maybe_capture_response captures the TAIL of the decrypted response
// buffer for LLM-flagged connections. The token usage object is near the end
// of the response JSON, so we grab the last LLM_BODY_BUFFER_SIZE bytes.
static __always_inline void llmo_maybe_capture_response(conn_tuple_t *t, char *buffer, __u64 len) {
    if (len < LLM_MIN_CAPTURE) {
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

    body->len = len < LLM_BODY_BUFFER_SIZE ? len : LLM_BODY_BUFFER_SIZE;

    // TAIL capture (token usage lives near the end of the response JSON).
    __u64 off = len > LLM_BODY_BUFFER_SIZE ? len - LLM_BODY_BUFFER_SIZE : 0;
    llmo_read_body(body->data, buffer + off);
    bpf_map_update_with_telemetry(llm_response_bodies, &key, body, BPF_ANY);

    // HEAD capture (the assistant's message content lives near the start).
    llmo_read_body(body->data, buffer);
    bpf_map_update_with_telemetry(llm_response_heads, &key, body, BPF_ANY);

    // Stream the tail to userspace as an event, so a continuous consumer sees
    // every response's usage in order (poll-batched map reads miss turns).
    llm_resp_event_t *ev = bpf_map_lookup_elem(&llm_event_scratch, &zero);
    if (ev != NULL) {
        ev->key = key;
        // Stream a large window from the START of the response: the assistant
        // answer is near the start, and for responses up to this size the
        // usage/finish fields (near the end) are included too.
        __u32 sid = 0, eos = 0;
        llmo_frame_hdr(buffer, &sid, &eos);
        ev->stream_id = sid;
        ev->end_stream = eos;
        ev->len = len < LLM_REQ_BUFFER_SIZE ? len : LLM_REQ_BUFFER_SIZE;
        llmo_read_req_body(ev->data, buffer);
        bpf_ringbuf_output(&llm_response_events, ev, sizeof(*ev), 0);
        log_debug("[llmo] resp streamed len=%u stream=%u eos=%u", ev->len, ev->stream_id, ev->end_stream);
    }
    log_debug("[llmo] response stored len=%u off=%llu", body->len, off);
}

#endif
