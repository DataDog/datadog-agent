// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

#include "rtloader.h"
#include "rtloader_mem.h"

#define GET_DIANGOSES_FAILURE_DIAGNOSES_BEGIN                                                                          \
    "[{\"result\":3, \"diagnosis\": \"check's get_diagnoses() method failed\", \"rawerror\": \""
#define GET_DIANGOSES_FAILURE_DIAGNOSES_END "\"}]"

RtLoader::RtLoader(cb_memory_tracker_t memtrack_cb)
    : _error()
    , _errorFlag(false)
{
    _set_memory_tracker_cb(memtrack_cb);
};

void RtLoader::setError(const std::string &msg) const
{
    _errorFlag = true;
    _error = msg;
}

void RtLoader::setError(const char *msg) const
{
    _errorFlag = true;
    _error = msg;
}

const char *RtLoader::getError() const
{
    if (!_errorFlag) {
        // error was already fetched, cleanup
        _error = "";
    } else {
        _errorFlag = false;
    }

    return _error.c_str();
}

bool RtLoader::hasError() const
{
    return _errorFlag;
}

void RtLoader::clearError()
{
    _errorFlag = false;
    _error = "";
}

void RtLoader::free(void *ptr)
{
    if (ptr != NULL) {
        _free(ptr);
    }
}

char *RtLoader::_createInternalErrorDiagnoses(const char *errorMessage)
{
    std::string getDiagnoseFailureDiagnoses
        = std::string(GET_DIANGOSES_FAILURE_DIAGNOSES_BEGIN) + errorMessage + GET_DIANGOSES_FAILURE_DIAGNOSES_END;

    return strdupe(getDiagnoseFailureDiagnoses.c_str());
}
