// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include <six.h>

void Six::setError(const std::string &msg) {
    _error_mtx.lock();
    _error = msg;
    _error_mtx.unlock();
}

void Six::clearError() {
    _error_mtx.lock();
    _error = "";
    _error_mtx.unlock();
}

std::string Six::getError() const {
    std::string ret;

    _error_mtx.lock();
    ret = _error;
    _error_mtx.unlock();

    return ret;
}

bool Six::hasError() const {
    bool ret;

    _error_mtx.lock();
    ret = _error != "";
    _error_mtx.unlock();

    return ret;
}
