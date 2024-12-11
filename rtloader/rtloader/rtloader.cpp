// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

#include "rtloader.h"
#include "rtloader_mem.h"
#include <iomanip>
#include <sstream>

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
    std::ostringstream o;

    o << GET_DIANGOSES_FAILURE_DIAGNOSES_BEGIN;

    if (errorMessage != nullptr) {
        while (*errorMessage) {
            char c = *errorMessage;
            switch (c) {
            case '"':
                o << "\\\"";
                break;
            case '\\':
                o << "\\\\";
                break;
            case '\b':
                o << "\\b";
                break;
            case '\f':
                o << "\\f";
                break;
            case '\n':
                o << "\\n";
                break;
            case '\r':
                o << "\\r";
                break;
            case '\t':
                o << "\\t";
                break;
            default:
                if ('\x00' <= c && c <= '\x1f') {
                    o << "\\u" << std::hex << std::setw(4) << std::setfill('0') << (int)c;
                } else {
                    o << c;
                }
            }

            errorMessage++;
        }
    }

    o << GET_DIANGOSES_FAILURE_DIAGNOSES_END;

    return strdupe(o.str().c_str());
}
