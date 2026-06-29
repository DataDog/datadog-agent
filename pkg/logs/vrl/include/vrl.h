#ifndef VRL_FILTER_H
#define VRL_FILTER_H

#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct vrl_program vrl_program_t;

/**
 * Compile a VRL boolean expression from source.
 * Returns an opaque program handle on success, or NULL on failure.
 * On failure, *err_out is set to a malloc'd error string; free with vrl_free_string.
 */
vrl_program_t *vrl_compile(const char *source, char **err_out);

/**
 * Evaluate the compiled program against a log message string (exposed as .message).
 *
 * Returns:
 *   1  — expression was truthy (match)
 *   0  — expression was falsy, null, or called abort (no match)
 *  -1  — runtime error; *err_out is set; free with vrl_free_string
 */
int vrl_eval(const vrl_program_t *prog, const char *message, size_t len, char **err_out);

/** Free a program handle returned by vrl_compile. */
void vrl_free_program(vrl_program_t *prog);

/** Free a string returned by this library. */
void vrl_free_string(char *s);

#ifdef __cplusplus
}
#endif

#endif /* VRL_FILTER_H */
