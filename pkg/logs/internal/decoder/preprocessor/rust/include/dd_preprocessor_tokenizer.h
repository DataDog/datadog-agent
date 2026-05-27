/* Unless explicitly stated otherwise all files in this repository are licensed
 * under the Apache License Version 2.0.
 * This product includes software developed at Datadog (https://www.datadoghq.com/).
 * Copyright 2024-present Datadog, Inc. */

#ifndef DD_PREPROCESSOR_TOKENIZER_H
#define DD_PREPROCESSOR_TOKENIZER_H

#include <stddef.h>
#include <stdint.h>

/* Opaque handle to the Rust tokenizer. */
typedef struct dd_tokenizer dd_tokenizer;

/* Create a tokenizer. max_eval_bytes: 0 = unlimited.
 * Returns NULL on failure (should never happen). */
dd_tokenizer *dd_tokenizer_new(size_t max_eval_bytes);

/* Free a tokenizer. NULL is a no-op. */
void dd_tokenizer_free(dd_tokenizer *t);

/* Tokenize input bytes. Writes tokens and indices into caller-owned buffers.
 *
 * Returns the number of tokens written (>= 0) on success,
 * or -1 if capacity is insufficient.
 *
 * tokens_out:  caller-allocated buffer for token bytes (uint8_t)
 * indices_out: caller-allocated buffer for start indices (int32_t)
 * capacity:    size of both buffers (in elements) */
int32_t dd_tokenizer_tokenize(dd_tokenizer *t,
                              const uint8_t *input,
                              size_t input_len,
                              uint8_t *tokens_out,
                              int32_t *indices_out,
                              size_t capacity);

#endif /* DD_PREPROCESSOR_TOKENIZER_H */
