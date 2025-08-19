// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

#include "Win32Exception.h"
#include <vector>
#include <sstream>

Win32Exception::Win32Exception(DWORD error, const std::string& msg)
: runtime_error(FormatErrorMessage(error, msg))
, _error(error)
{
}

Win32Exception::Win32Exception(const std::string& msg)
: Win32Exception(GetLastError(), msg)
{
}

DWORD Win32Exception::GetErrorCode() const
{
    return _error;
}

std::string Win32Exception::FormatErrorMessage(DWORD error, const std::string& msg)
{
    static const int BUFFERLENGTH = 1024;
    std::vector<char> buf(BUFFERLENGTH);
    // Unfortunately runtime_error doesn't support wstring
    FormatMessageA(FORMAT_MESSAGE_FROM_SYSTEM, nullptr, error, 0, buf.data(),
                   BUFFERLENGTH - 1, nullptr);
    std::stringstream sstream;
    sstream << msg << ": " << buf.data();
    return sstream.str();
}
