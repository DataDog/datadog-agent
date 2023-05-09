// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#include "diagnoseutils.h"
#include "stringutils.h"

#include <stdlib.h>
#include <string.h>

size_t get_diagnoses_mem_size(Py_ssize_t numDiagnoses, PyObject *diagnoses_list)
{
    Py_ssize_t idx = 0;

    // Calculate and allocate buffer size
    size_t bufferSize = sizeof(diagnosis_set_t) + (numDiagnoses * sizeof(diagnosis_t));
    for (idx = 0; idx < numDiagnoses; idx++) {
        PyObject *diagnosisObj = PyList_GetItem(diagnoses_list, idx); // borrowed ref
        if (diagnosisObj == NULL) {
            return 0;
        }

        bufferSize += attr_as_string_size(diagnosisObj, "name");
        bufferSize += attr_as_string_size(diagnosisObj, "diagnosis");
        bufferSize += attr_as_string_size(diagnosisObj, "description");
        bufferSize += attr_as_string_size(diagnosisObj, "remediation");
        bufferSize += attr_as_string_size(diagnosisObj, "raw_error");
    }

    return bufferSize;
}

int serialize_diagnoses(Py_ssize_t numDiagnoses, PyObject *diagnoses_list, diagnosis_set_t *diagnoses,
                        size_t bufferSize)
{
    Py_ssize_t idx = 0;

    size_t currentOffset = sizeof(diagnosis_set_t) + (numDiagnoses * sizeof(diagnosis_t));

    // Initialize header
    diagnoses->byteCount = bufferSize;
    diagnoses->diangosesCount = numDiagnoses;
    diagnoses->diagnosesItems = (diagnosis_t *)((size_t)(void *)diagnoses + sizeof(diagnosis_set_t));

    for (idx = 0; idx < numDiagnoses; idx++) {
        PyObject *diagnosisObj = PyList_GetItem(diagnoses_list, idx); // borrowed ref
        if (diagnosisObj == NULL) {
            return -1;
        }
        size_t copiedSize = 0;
        diagnosis_t *diagnosis = diagnoses->diagnosesItems + idx;

        // result
        diagnosis->result = (size_t)attr_as_long(diagnosisObj, "result");

        // name
        copiedSize = copy_attr_as_string_at(diagnosisObj, "name", diagnoses, currentOffset, bufferSize);
        if (copiedSize > 0) {
            diagnosis->name = string_buf_from_offset(diagnoses, currentOffset);
            currentOffset += copiedSize;
        }

        // diagnosis
        copiedSize = copy_attr_as_string_at(diagnosisObj, "diagnosis", diagnoses, currentOffset, bufferSize);
        if (copiedSize > 0) {
            diagnosis->diagnosis = string_buf_from_offset(diagnoses, currentOffset);
            currentOffset += copiedSize;
        }

        // description
        copiedSize = copy_attr_as_string_at(diagnosisObj, "description", diagnoses, currentOffset, bufferSize);
        if (copiedSize > 0) {
            diagnosis->description = string_buf_from_offset(diagnoses, currentOffset);
            currentOffset += copiedSize;
        }

        // remediation
        copiedSize = copy_attr_as_string_at(diagnosisObj, "remediation", diagnoses, currentOffset, bufferSize);
        if (copiedSize > 0) {
            diagnosis->remediation = string_buf_from_offset(diagnoses, currentOffset);
            currentOffset += copiedSize;
        }

        // raw_error
        copiedSize = copy_attr_as_string_at(diagnosisObj, "raw_error", diagnoses, currentOffset, bufferSize);
        if (copiedSize > 0) {
            diagnosis->raw_error = string_buf_from_offset(diagnoses, currentOffset);
            currentOffset += copiedSize;
        }
    }

    // Sanity check. Calculated and copied size should match
    if (currentOffset != bufferSize) {
        return -1;
    }

    return 0;
}
