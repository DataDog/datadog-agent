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
    // Each prefix is compared only if the buffer is long enough to hold it.
    if (buf_size >= sizeof(ERR)-1 && !bpf_memcmp(buf, ERR, sizeof(ERR)-1)) {
        return true;
    }
    if (buf_size >= sizeof(WRONGTYPE)-1 && !bpf_memcmp(buf, WRONGTYPE, sizeof(WRONGTYPE)-1)) {
        return true;
    }
    return false;
}

// Character classes permitted between a RESP type byte and the CRLF that
// terminates its first field. Passed as a mask so that every RESP type shares
// one validator rather than each growing its own.
#define RESP_CLASS_DIGIT        (1 << 0) // 0-9
#define RESP_CLASS_SIGN         (1 << 1) // '-' or '+', leading position only, for integers and big numbers
#define RESP_CLASS_MINUS        (1 << 2) // '-', leading position only, for the null forms $-1 and *-1
#define RESP_CLASS_DOT          (1 << 3) // '.', for RESP3 doubles
#define RESP_CLASS_ALPHA        (1 << 4) // A-Za-z, for +OK, -NOPE ..., and inf/nan/exponents in doubles
#define RESP_CLASS_PUNCT        (1 << 5) // ' ', '-', '_', for error and status text
#define RESP_CLASS_INNER_SIGN   (1 << 6) // '-' or '+' at any position, for double signs and exponents (-1.5, 1e-7)

// Is c acceptable inside a RESP field of the given classes? `first` marks the
// byte immediately after the type prefix, the only position a leading sign may
// occupy.
static __always_inline bool resp_char_allowed(char c, __u8 allowed_classes, bool first) {
    if ((allowed_classes & RESP_CLASS_DIGIT) && '0' <= c && c <= '9') {
        return true;
    }
    if ((allowed_classes & RESP_CLASS_SIGN) && first && (c == '-' || c == '+')) {
        return true;
    }
    if ((allowed_classes & RESP_CLASS_MINUS) && first && c == '-') {
        return true;
    }
    if ((allowed_classes & RESP_CLASS_DOT) && c == '.') {
        return true;
    }
    // A-Za-z in one comparison: setting bit 5 folds upper case onto lower case,
    // and the unsigned wrap sends everything below 'a' above 25. No other byte
    // lands in 'a'..'z' after the fold.
    if ((allowed_classes & RESP_CLASS_ALPHA) && (__u8)((c | 0x20) - 'a') < 26) {
        return true;
    }
    if ((allowed_classes & RESP_CLASS_PUNCT) && (c == ' ' || c == '-' || c == '_')) {
        return true;
    }
    if ((allowed_classes & RESP_CLASS_INNER_SIGN) && (c == '-' || c == '+')) {
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
// The steps below examine buf[1] through buf[RESP_FIELD_MAX_LEN + 1], and the
// CR must be among them, so a field of at most RESP_FIELD_MAX_LEN bytes is
// accepted. That covers every bulk and array length Redis accepts by default
// (512 MB is nine digits), integers up to ten characters, +OK, +PONG,
// -ERR .... A longer field is rejected rather than accepted on faith — a
// payload we cannot positively identify as RESP must not be classified as
// Redis. The last step reads buf[RESP_FIELD_MAX_LEN + 2], inside the
// CLASSIFICATION_MAX_BUFFER bytes every caller passes (asserted below).
//
// Raising the limit means changing RESP_FIELD_MAX_LEN *and* adding a step —
// the static assertions catch one without the other — and re-checking the
// verifier and program sizes under both CO-RE and runtime compilation.
#define RESP_FIELD_MAX_LEN 10
_Static_assert(RESP_FIELD_MAX_LEN == 10,
               "check_resp_field_and_crlf has explicit steps 1..10 before the final step; change both together");
_Static_assert(RESP_FIELD_MAX_LEN + 2 < CLASSIFICATION_MAX_BUFFER,
               "the last RESP field step must read inside the classification buffer");
static __always_inline bool check_resp_field_and_crlf(const char* buf, __u32 buf_size, __u8 allowed_classes) {
    RESP_FIELD_STEP(1);
    // A sign on its own is not a number: reject $-\r\n, :+\r\n, ,-\r\n. Step 1
    // returned unless buf_size >= 3, so buf[2] is readable. Text is exempt —
    // "-" is a legal (if odd) simple string or error message.
    if (!(allowed_classes & RESP_CLASS_PUNCT) && (buf[1] == '-' || buf[1] == '+') && buf[2] == RESP_TERMINATOR_1) {
        return false;
    }
    RESP_FIELD_STEP(2);
    RESP_FIELD_STEP(3);
    RESP_FIELD_STEP(4);
    RESP_FIELD_STEP(5);
    RESP_FIELD_STEP(6);
    RESP_FIELD_STEP(7);
    RESP_FIELD_STEP(8);
    RESP_FIELD_STEP(9);
    RESP_FIELD_STEP(10);
    RESP_FIELD_STEP(RESP_FIELD_MAX_LEN + 1);
    return false;
}
#undef RESP_FIELD_STEP

// Text fields: simple strings and errors (A-Za-z, 0-9, '.', ' ', '-', '_').
// RESP allows any byte but CR and LF here. Digits are included so that short
// replies carrying numbers (a status word with a count, a short error code)
// still classify; they raise the false-positive rate on random bytes only from
// ~1.35e-7 to ~1.43e-7. Long replies such as +FULLRESYNC <runid> <offset>
// (~50 bytes) exceed RESP_FIELD_MAX_LEN and are rejected by design: the
// connection is classified from the client's request instead.
#define RESP_CLASS_TEXT (RESP_CLASS_ALPHA | RESP_CLASS_DIGIT | RESP_CLASS_DOT | RESP_CLASS_PUNCT)

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
// text. Frames whose first field is longer than RESP_FIELD_MAX_LEN, or whose
// terminator falls outside the buffer, are rejected; giving up is the safe
// direction, and the caller retries on later packets. On uniform random bytes
// the false-positive rate is ~1.43e-7, about 40% of it from the fixed
// three-byte RESP3 null _\r\n.
//
// The switch only chooses which character classes the first field may use;
// the field itself is checked by a single call to check_resp_field_and_crlf
// after it. That is deliberate. The validator is inlined, so every call site
// gets its own copy of the 11-step ladder (~580 instructions), and is_redis is
// itself inlined into several programs, including the Go TLS return uprobes.
// Calling the validator from each switch arm would put four copies in each of
// those programs and push them past the 4,096-instruction limit on pre-5.2
// kernels (E2BIG on 4.14-4.19). The DEBUG=1 build is the binding case, with
// ~180 instructions of headroom; only its runtime-compiled form is loaded in
// tests, not usm-debug.o. Keep it a single call.
static __always_inline bool is_redis(const char* buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, REDIS_MIN_FRAME_LENGTH);

    __u8 classes;
    switch (buf[0]) {
    // Length- and count-prefixed types: <type><decimal>\r\n. A leading '-' is
    // admitted only for the null forms $-1\r\n and *-1\r\n; lengths never
    // carry '+'.
    case RESP_ARRAY_PREFIX:            // Array
    case RESP_BULK_PREFIX:             // Bulk String
    case RESP3_BULK_ERROR_PREFIX:      // Bulk Error
    case RESP3_VERBATIM_STRING_PREFIX: // Verbatim String
    case RESP3_MAP_PREFIX:             // Map
    case RESP3_SET_PREFIX:             // Set
    case RESP3_PUSH_PREFIX:            // Push
        classes = RESP_CLASS_DIGIT | RESP_CLASS_MINUS;
        break;

    // Integers and big numbers may carry a leading '+' or '-'. A big number
    // longer than RESP_FIELD_MAX_LEN will not find its terminator and is
    // rejected.
    case RESP_INTEGER_PREFIX:          // Integer
    case RESP3_BIG_NUMBER_PREFIX:      // Big Number
        classes = RESP_CLASS_DIGIT | RESP_CLASS_SIGN;
        break;

    // Doubles additionally carry '.', a sign or signed exponent (-1.5, 1e-7),
    // and the literals inf, -inf and nan. Only the character set is checked,
    // not the grammar: any letter is admitted, because narrowing it to the
    // letters of inf/nan/e cost ~50 instructions per inlined copy for a gain of
    // ~1.5e-8 in the false-positive rate. INNER_SIGN admits a sign at every
    // position, including the leading one.
    case RESP3_DOUBLE_PREFIX:          // Double
        classes = RESP_CLASS_DIGIT | RESP_CLASS_INNER_SIGN | RESP_CLASS_DOT | RESP_CLASS_ALPHA;
        break;

    // Simple strings: +OK\r\n, +PONG\r\n.
    case RESP_SIMPLE_STRING_PREFIX:    // Simple String
        classes = RESP_CLASS_TEXT;
        break;

    // Errors: -ERR ..., -WRONGTYPE ..., or other CRLF-terminated text.
    case RESP_ERROR_PREFIX:            // Error
        if (check_err_prefix(buf, buf_size)) {
            return true;
        }
        classes = RESP_CLASS_TEXT;
        break;

    // RESP3 null is exactly _\r\n. REDIS_MIN_FRAME_LENGTH guarantees 3 bytes.
    case RESP3_NULL_PREFIX:            // Null
        return buf[1] == RESP_TERMINATOR_1 && buf[2] == RESP_TERMINATOR_2;

    // RESP3 boolean is exactly #t\r\n or #f\r\n.
    case RESP3_BOOLEAN_PREFIX:         // Boolean
        return buf_size > 3 && (buf[1] == 't' || buf[1] == 'f') && buf[2] == RESP_TERMINATOR_1 && buf[3] == RESP_TERMINATOR_2;

    default:
        return false;
    }

    return check_resp_field_and_crlf(buf, buf_size, classes);
}

#endif
