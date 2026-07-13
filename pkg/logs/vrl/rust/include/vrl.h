/* Unless explicitly stated otherwise all files in this repository are licensed
 * under the Apache License Version 2.0.
 * This product includes software developed at Datadog (https://www.datadoghq.com/).
 * Copyright 2016-present Datadog, Inc. */

#ifndef VRL_FILTER_H
#define VRL_FILTER_H

#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Opaque handle to a compiled VRL program. */
typedef struct vrl_program vrl_program_t;

/* Length-delimited byte buffer, heap-allocated by this library. Not
 * NUL-terminated. `data` is NULL with `len == 0` on failure. */
struct vrl_bytes {
  uint8_t *data;
  size_t len;
};

/* Compiles a VRL boolean expression or transform against this crate's
 * curated function list. Returns a heap-allocated program on success, or
 * NULL on failure. On failure, if err_out is non-NULL, *err_out is set to a
 * malloc'd error string (caller must free with vrl_free_string). */
vrl_program_t *vrl_compile(const char *source, char **err_out);

/* Evaluates prog as a boolean predicate against message (exposed as
 * `.message`).
 *
 * Returns:
 *   1  - the program resolved to true (match)
 *   0  - the program resolved to anything else, or called `abort` (no match)
 *  -1  - a runtime error occurred; if err_out is non-NULL, *err_out is set
 *        to a malloc'd error string (caller must free with vrl_free_string) */
int32_t vrl_eval_filter(const vrl_program_t *prog, const char *message, size_t message_len, char **err_out);

/* Runs prog as a transform against message (exposed as `.message`) and
 * returns the resulting `.message` value, serialized to bytes. On failure,
 * returns a zeroed vrl_bytes and, if err_out is non-NULL, sets *err_out
 * (caller must free with vrl_free_string). On success, the caller must free
 * the returned buffer with vrl_free_bytes. */
struct vrl_bytes vrl_eval_transform(const vrl_program_t *prog, const char *message, size_t message_len,
                                    char **err_out);

/* Frees a program returned by vrl_compile. Passing NULL is a no-op. */
void vrl_free_program(vrl_program_t *prog);

/* Frees a string returned by this library via an err_out parameter. Passing
 * NULL is a no-op. */
void vrl_free_string(char *s);

/* Frees a byte buffer returned by vrl_eval_transform. Passing a zeroed
 * buffer is a no-op. */
void vrl_free_bytes(struct vrl_bytes bytes);

#ifdef __cplusplus
}
#endif

#endif /* VRL_FILTER_H */
