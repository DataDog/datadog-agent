#include <cstdarg>
#include <cstdint>
#include <cstdlib>
#include <ostream>
#include <new>

/// Default minimum number of matches required for consolidation
/// Matches the Java implementation's requirement of at least 3 key-value pairs
constexpr static const uintptr_t KeyValuePattern_DEFAULT_MIN_MATCHES = 3;

constexpr static const uintptr_t KeyValuePatternSpans_DEFAULT_MIN_MATCHES = 3;

/// Signature version constants
/// Signature updates are described in detail in
/// <https://datadoghq.atlassian.net/wiki/spaces/DA/pages/2449736193/2022+Signature+improvements>
constexpr static const int32_t LATEST_SIGNATURE_VERSION = 8;

/// Special characters used for boundary detection in the automaton
/// Beginning of file/string marker
constexpr static const uint32_t BOF = '\u{2}';

/// End of file/string marker
constexpr static const uint32_t EOF = '\u{0}';

/// Output entry for a single log in a batch tokenization request.
///
/// On success:  `ptr` is non-null, `len`/`cap` are the FlatBuffer Vec dimensions,
///              `err_ptr` is null. Free the buffer with `patterns_vec_free`.
/// On error:    `ptr` is null, `len`/`cap` are 0,
///              `err_ptr` is a non-null C string. Free with `patterns_str_free`.
///
/// Each slot is independent; one failure does not affect sibling slots.
struct BatchTokenEntry {
  uint8_t *ptr;
  uintptr_t len;
  uintptr_t cap;
  /// Per-slot error string (null = success). Caller must free with `patterns_str_free`.
  char *err_ptr;
};

extern "C" {

/// Free a C string allocated by Rust
///
/// **IMPORTANT**: Go must call this function to free all strings returned by Rust.
/// Failure to do so will cause memory leaks.
///
/// # Safety
///
/// * `str` must be a valid, non-null pointer to a C string allocated by Rust
/// * `str` must only be freed once
void patterns_str_free(char *str);

/// Free a byte buffer allocated by Rust
///
/// # Safety
///
/// * `ptr` must be a valid pointer allocated by Rust with the given `len` and `cap`
/// * `ptr` must only be freed once
void patterns_vec_free(uint8_t *ptr, uintptr_t len, uintptr_t cap);

/// Get the last error message
///
/// Returns a C string that must be freed with `patterns_str_free`
///
/// # Safety
///
/// This function is always safe to call
char *patterns_get_last_err_str();

/// Get and clear the last error message
///
/// Returns a C string that must be freed with `patterns_str_free`
///
/// # Safety
///
/// This function is always safe to call
char *patterns_get_last_err_str_and_reset();

/// Clear the last error message
///
/// # Safety
///
/// This function is always safe to call
void patterns_reset_last_err_str();

/// Tokenize a single log line and return tokens as FlatBuffers
///
/// # Returns
///
/// Returns a FlatBuffers `TokenizeResponse`, or null on error.
/// - The returned buffer must be freed with `patterns_vec_free`
/// - On error, call `patterns_get_last_err_str()` to get the error message
///
/// # Safety
///
/// * `log_ptr` must be a valid null-terminated C string or null
uint8_t *patterns_tokenize_log(const char *log_ptr, uintptr_t *out_len, uintptr_t *out_cap);

/// Tokenize a batch of log lines.
///
/// Processes `count` logs from `log_ptrs` and writes one `BatchTokenEntry` per
/// input into `out_entries`. The caller must pre-allocate `out_entries` with
/// room for `count` entries.
///
/// Each entry is independent:
/// - On success the entry's `ptr`/`len`/`cap` hold a FlatBuffer `TokenizeResponse`
///   that must be freed with `patterns_vec_free`.
/// - On per-log failure `ptr` is null and `err_ptr` holds a C string that must
///   be freed with `patterns_str_free`.
///
/// The thread-local last-error string is NOT modified by this function; all
/// error information lives in the per-entry `err_ptr` fields.
///
/// # Safety
///
/// * `log_ptrs` must be a valid pointer to an array of `count` null-terminated C strings
/// * `out_entries` must be a valid pointer to a pre-allocated array of `count` `BatchTokenEntry`
/// * Both arrays must remain valid for the duration of this call
///
/// # Panics
///
/// Panics if a per-slot error string contains interior null bytes (extremely unlikely
/// in practice; Rust error messages do not contain null bytes).
void patterns_tokenize_logs_batch(const char *const *log_ptrs,
                                  uintptr_t count,
                                  BatchTokenEntry *out_entries);

}  // extern "C"
