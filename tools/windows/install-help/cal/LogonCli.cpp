#include "stdafx.h"
#include "LogonCli.h"
#include <exception>

LogonCli::LogonCli()
    : _logonCliDll(LoadLibrary(L"Logoncli.dll"))
{
    if (_logonCliDll == nullptr)
    {
        throw std::exception("could not load the logoncli DLL");
    }
    _fnNetIsServiceAccount =
        reinterpret_cast<SigNetIsServiceAccount>(GetProcAddress(_logonCliDll, "NetIsServiceAccount"));
    if (_fnNetIsServiceAccount == nullptr)
    {
        throw std::exception("could not find the procedure NetIsServiceAccount in the logoncli DLL");
    }
}

LogonCli::LogonCli(LogonCli &&other) noexcept
    : _logonCliDll(std::move(other._logonCliDll))
    , _fnNetIsServiceAccount(std::move(other._fnNetIsServiceAccount))
{
}

LogonCli &LogonCli::operator=(LogonCli &&other) noexcept
{
    _logonCliDll = std::move(other._logonCliDll);
    _fnNetIsServiceAccount = std::move(other._fnNetIsServiceAccount);
    return *this;
}

// ReSharper disable once CppMemberFunctionMayBeConst
NTSTATUS LogonCli::NetIsServiceAccount(_In_opt_ LPWSTR ServerName, _In_ LPWSTR AccountName, _Out_ BOOL *IsService)
{
    return _fnNetIsServiceAccount(ServerName, AccountName, IsService);
}

LogonCli::~LogonCli()
{
    FreeLibrary(_logonCliDll);
}
