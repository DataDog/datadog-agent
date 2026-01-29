// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#ifndef DD_GET_TEXT_EMBEDDINGS_H
#define DD_GET_TEXT_EMBEDDINGS_H

#ifdef __cplusplus
extern "C" {
#endif

#include <stddef.h>

void dd_get_text_embeddings_init(char **err);

size_t dd_get_text_embeddings_get_embeddings_size(void);

void dd_get_text_embeddings_get_embeddings(const char *text, float *buffer, char **err);

#ifdef __cplusplus
} // extern "C"
#endif

#endif // DD_GET_TEXT_EMBEDDINGS_H

