/* Unless explicitly stated otherwise all files in this repository are licensed
 * under the Apache License Version 2.0.
 * This product includes software developed at Datadog (https://www.datadoghq.com/).
 * Copyright 2016-present Datadog, Inc. */

#ifndef DD_TAGMATCHER_H
#define DD_TAGMATCHER_H

#include <stdbool.h>
#include <stddef.h>

/**
 * Print "Hello groovy world" to stdout.
 */
void tagmatcher_hello(void);

/**
 * Opaque handle to a compiled Rust regex.
 * Only ever used through a pointer; the struct layout is not exposed to C.
 */
typedef struct TagMatcherRegex TagMatcherRegex;

/**
 * Compile a NUL-terminated regex pattern.
 * Returns NULL on failure (invalid UTF-8, compile error, or NULL input).
 * The caller must pass the result to tagmatcher_regex_free exactly once.
 */
TagMatcherRegex *tagmatcher_regex_new(const char *pattern);

/**
 * Return true if haystack[..haystack_len] matches the compiled regex.
 * haystack does not need to be NUL-terminated; only haystack_len bytes are read.
 * Returns false if either pointer is NULL or the bytes are not valid UTF-8.
 */
bool tagmatcher_regex_is_match(const TagMatcherRegex *re, const char *haystack, size_t haystack_len);

/**
 * Free a TagMatcherRegex previously returned by tagmatcher_regex_new.
 * Passing NULL is a no-op.
 */
void tagmatcher_regex_free(TagMatcherRegex *re);

#endif /* DD_TAGMATCHER_H */
