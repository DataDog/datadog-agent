// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#ifndef DATADOG_AGENT_DIAGNOSEUTILS_H
#define DATADOG_AGENT_DIAGNOSEUTILS_H

/*! \file diagnoseutils.h
    \brief RtLoader diagnose wrapper header file.

    The prototypes here defined provide functions to allocate and free memory.
    The goal is to allow us to track allocations if desired.
*/

#include "rtloader_types.h"

#include <Python.h>
#include <stdlib.h>

#define MEM_DEPRECATION_MSG                                                                                            \
    "raw primitives should not be used in the context"                                                                 \
    "of the rtloader"

#ifdef __cplusplus
extern "C" {
#endif

size_t get_diagnoses_mem_size(Py_ssize_t numDiagnoses, PyObject *diagnoses_list);

int serialize_diagnoses(Py_ssize_t numDiagnoses, PyObject *diagnoses_list, diagnosis_set_t *diagnoses,
                        size_t bufferSize);

#ifdef __cplusplus
}
#endif

#endif
