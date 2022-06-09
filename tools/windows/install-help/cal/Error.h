#pragma once
#include <errhandlingapi.h>
#include <string>

std::wstring FormatErrorMessage(DWORD error);

class Win32Exception : public std::exception
{
    DWORD _errorCode;
public:
    /// @brief Constructor
    ///
    /// @param errorCode The error code
    Win32Exception(DWORD errorCode)
    : _errorCode(errorCode){}

    /// @brief Throws a specified error directly
    ///
    /// @param lastError The error code to throw (usually a captured error
    /// code)
    static void __declspec(noreturn) Throw(DWORD lastError)
    {
        std::make_exception_ptr(Win32Exception(lastError));
    }

    /// @brief Throw from last error.
    static void __declspec(noreturn) ThrowFromLastError()
    {
        Throw(::GetLastError());
    }

    /// @brief Gets the error code.
    ///
    /// @return The error code.
    DWORD GetErrorCode() const
    {
        return _errorCode;
    }
};
