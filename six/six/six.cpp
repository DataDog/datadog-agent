// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include <six.h>

void Six::setError(const std::string &msg) const
{
    _errorFlag = true;
    _error = msg;
}

void Six::setError(const char *msg) const
{
    _errorFlag = true;
    _error = msg;
}

const char *Six::getError() const
{
    if (!_errorFlag) {
        // error was already fetched, cleanup
        _error = "";
    } else {
        _errorFlag = false;
    }

    return _error.c_str();
}

bool Six::hasError() const
{
    return _errorFlag;
}

void Six::clearError()
{
    _errorFlag = false;
    _error = "";
}

void Six::free(void *ptr)
{
    if (ptr != NULL) {
        ::free(ptr);
    }
}
