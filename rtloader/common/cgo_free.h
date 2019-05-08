// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_CGO_FREE_H
#define DATADOG_AGENT_SIX_CGO_FREE_H
#include <six_types.h>

#ifdef __cplusplus
extern "C" {
#endif

void _set_cgo_free_cb(cb_cgo_free_t);
void DATADOG_AGENT_SIX_API cgo_free(void *ptr);

#ifdef __cplusplus
}
#endif

#endif
