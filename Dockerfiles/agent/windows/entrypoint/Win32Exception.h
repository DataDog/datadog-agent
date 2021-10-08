// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

#pragma once

#include <Windows.h>
#include <stdexcept>

class Win32Exception : public std::runtime_error
{
private:
    DWORD _error;
public:
    Win32Exception(DWORD error, const std::string& msg);
    explicit Win32Exception(const std::string& msg);
    DWORD GetErrorCode() const;

    static std::string FormatErrorMessage(DWORD error, const std::string& msg);
};

