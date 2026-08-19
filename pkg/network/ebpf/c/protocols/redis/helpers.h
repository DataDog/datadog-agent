#ifndef __REDIS_HELPERS_H
#define __REDIS_HELPERS_H

#include "protocols/classification/common.h"
#include "protocols/redis/defs.h"

static __always_inline __maybe_unused void convert_method_to_upper_case(char* method) {
    #pragma unroll (MAX_METHOD_LEN)
    for (int i = 0; i < MAX_METHOD_LEN; i++) {
        if ('a' <= method[i] && method[i] <= 'z') {
            method[i] = method[i] - 'a' + 'A';
        }
    }
}

// Checks the buffer represents an error according to https://redis.io/docs/reference/protocol-spec/#resp-errors
static __always_inline bool check_err_prefix(const char* buf, __u32 buf_size) {
#define ERR "-ERR "
#define WRONGTYPE "-WRONGTYPE "

    // memcmp returns
    // 0 when s1 == s2,
    // !0 when s1 != s2.
    bool match = !(bpf_memcmp(buf, ERR, sizeof(ERR)-1)
        && bpf_memcmp(buf, WRONGTYPE, sizeof(WRONGTYPE)-1));

    return match;
}

// Character classes permitted between a RESP type byte and the CRLF that
// terminates its first field. Passed as a mask so that every RESP type shares
// one validator rather than each growing its own.
#define RESP_CLASS_DIGIT (1 << 0) // 0-9
#define RESP_CLASS_SIGN  (1 << 1) // '-' or '+', leading position only
#define RESP_CLASS_DOT   (1 << 2) // '.', for RESP3 doubles
#define RESP_CLASS_ALPHA (1 << 3) // A-Za-z, for +OK, -ERR, inf/nan
#define RESP_CLASS_PUNCT (1 << 4) // ' ', '-', '_', for error and status text

// Is c acceptable inside a RESP field of the given classes? `first` marks the
// byte immediately after the type prefix, the only position a sign may occupy.
static __always_inline bool resp_char_allowed(char c, __u8 allowed_classes, bool first) {
    if ((allowed_classes & RESP_CLASS_DIGIT) && '0' <= c && c <= '9') {
        return true;
    }
    if ((allowed_classes & RESP_CLASS_SIGN) && first && (c == '-' || c == '+')) {
        return true;
    }
    if ((allowed_classes & RESP_CLASS_DOT) && c == '.') {
        return true;
    }
    if ((allowed_classes & RESP_CLASS_ALPHA) && (('A' <= c && c <= 'Z') || ('a' <= c && c <= 'z'))) {
        return true;
    }
    if ((allowed_classes & RESP_CLASS_PUNCT) && (c == ' ' || c == '-' || c == '_')) {
        return true;
    }
    return false;
}

// Examines one fixed position N of the field. If buf[N] is CR the field ends
// there, and the frame is valid iff at least one payload byte preceded it and
// LF follows. Otherwise buf[N] must be an allowed character.
//
// N is always a literal. That is the whole point: a loop over this buffer gets
// unrolled into offsets the verifier cannot bound — the runtime-compiled build
// rejects it with "invalid access to map value" one byte past the end — and
// masking the index to a power of two does not help, because the optimiser
// discards the mask it can already prove redundant. Literal indices give the
// verifier constant offsets with nothing left to prove. Do not convert this
// back into a loop.
#define RESP_FIELD_STEP(N)                                                  \
    do {                                                                    \
        if ((N) + 1 >= buf_size) {                                          \
            return false;                                                   \
        }                                                                   \
        if (buf[(N)] == RESP_TERMINATOR_1) {                                \
            return (N) > 1 && buf[(N) + 1] == RESP_TERMINATOR_2;            \
        }                                                                   \
        if (!resp_char_allowed(buf[(N)], allowed_classes, (N) == 1)) {      \
            return false;                                                   \
        }                                                                   \
    } while (0)

// Validates the first field of a RESP frame: the bytes between the type prefix
// at buf[0] and the CRLF that terminates the field.
//
// Only the first RESP_FIELD_MAX_LEN bytes are examined. A field longer than
// that is rejected rather than accepted on faith — a payload we cannot
// positively identify as RESP must not be classified as Redis. That covers
// every field we classify on in practice: length and count prefixes, +OK,
// +PONG, -ERR ....
#define RESP_FIELD_MAX_LEN 9
static __always_inline bool check_resp_field_and_crlf(const char* buf, __u32 buf_size, __u8 allowed_classes) {
    RESP_FIELD_STEP(1);
    RESP_FIELD_STEP(2);
    RESP_FIELD_STEP(3);
    RESP_FIELD_STEP(4);
    RESP_FIELD_STEP(5);
    RESP_FIELD_STEP(6);
    RESP_FIELD_STEP(7);
    RESP_FIELD_STEP(8);
    RESP_FIELD_STEP(9);
    return false;
}

// Text fields: the character set the previous validator accepted for simple
// strings and errors (A-Za-z plus '.', ' ', '-', '_'), minus the unbounded scan.
#define RESP_CLASS_TEXT (RESP_CLASS_ALPHA | RESP_CLASS_DOT | RESP_CLASS_PUNCT)

// is_redis reports whether buf holds the start of a RESP frame.
//
// The type byte alone is not sufficient evidence. A bare switch on buf[0]
// accepts 14 of 256 values, so it matches ~5.5% of arbitrary payloads — enough
// to claim TLS ciphertext. That is not a cosmetic mis-label: once the app layer
// is set to Redis, the encryption-layer gate in protocol_classifier_entrypoint
// stops running is_tls() for the life of the connection, and the connection is
// reported with tls_encrypted:false. Measured on one staging cluster over one
// hour: 6.00k connections wrongly tagged redis, of which 16.8GB was Kafka.
//
// So every type byte is now followed by a check on the shape of the frame's
// first field — a decimal length or count, a signed number, or CRLF-terminated
// text. Frames whose terminator falls outside the classification buffer are
// rejected; giving up is the safe direction, and the caller retries on later
// packets.
static __always_inline bool is_redis(const char* buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, REDIS_MIN_FRAME_LENGTH);

    switch (buf[0]) {
    // Length- and count-prefixed types: <type><decimal>\r\n. The sign class also
    // admits the null forms $-1\r\n and *-1\r\n.
    case RESP_ARRAY_PREFIX:            // Array
    case RESP_BULK_PREFIX:             // Bulk String
    case RESP3_BULK_ERROR_PREFIX:      // Bulk Error
    case RESP3_VERBATIM_STRING_PREFIX: // Verbatim String
    case RESP3_MAP_PREFIX:             // Map
    case RESP3_SET_PREFIX:             // Set
    case RESP3_PUSH_PREFIX:            // Push
    // Integers and big numbers are the same shape. A big number longer than the
    // classification buffer will not find its terminator and is rejected.
    case RESP_INTEGER_PREFIX:          // Integer
    case RESP3_BIG_NUMBER_PREFIX:      // Big Number
        return check_resp_field_and_crlf(buf, buf_size,RESP_CLASS_DIGIT | RESP_CLASS_SIGN);

    // Doubles additionally carry '.' and the literals inf, -inf and nan.
    case RESP3_DOUBLE_PREFIX:          // Double
        return check_resp_field_and_crlf(buf, buf_size,RESP_CLASS_DIGIT | RESP_CLASS_SIGN | RESP_CLASS_DOT | RESP_CLASS_ALPHA);

    // Simple strings: +OK\r\n, +PONG\r\n.
    case RESP_SIMPLE_STRING_PREFIX:    // Simple String
        return check_resp_field_and_crlf(buf, buf_size,RESP_CLASS_TEXT);

    // Errors: -ERR ..., -WRONGTYPE ..., or other CRLF-terminated text.
    case RESP_ERROR_PREFIX:            // Error
        return check_err_prefix(buf, buf_size) || check_resp_field_and_crlf(buf, buf_size,RESP_CLASS_TEXT);

    // RESP3 null is exactly _\r\n. REDIS_MIN_FRAME_LENGTH guarantees 3 bytes.
    case RESP3_NULL_PREFIX:            // Null
        return buf[1] == RESP_TERMINATOR_1 && buf[2] == RESP_TERMINATOR_2;

    // RESP3 boolean is exactly #t\r\n or #f\r\n.
    case RESP3_BOOLEAN_PREFIX:         // Boolean
        return buf_size > 3 && (buf[1] == 't' || buf[1] == 'f') && buf[2] == RESP_TERMINATOR_1 && buf[3] == RESP_TERMINATOR_2;

    default:
        return false;
    }
}

#endif
