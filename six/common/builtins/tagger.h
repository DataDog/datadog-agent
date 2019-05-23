// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

#ifndef DATADOG_AGENT_SIX_TAGGER_H
#define DATADOG_AGENT_SIX_TAGGER_H
#include <Python.h>
#include <six_types.h>

#define TAGGER_MODULE_NAME "tagger"

#ifdef DATADOG_AGENT_THREE
PyMODINIT_FUNC PyInit_tagger(void);
#endif


#ifdef __cplusplus
extern "C" {
#endif

#ifdef DATADOG_AGENT_TWO
void Py2_init_tagger();
#endif

void _set_tags_cb(cb_tags_t);

#ifdef __cplusplus
}
#endif

#endif  // DATADOG_AGENT_SIX_TAGGER_H
