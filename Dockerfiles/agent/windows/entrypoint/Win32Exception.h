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

