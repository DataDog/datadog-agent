#pragma once
#include <Windows.h>
#include "NonCopyable.h"

class LogonCli : private NonCopyable<LogonCli>
{
  private:
    typedef NTSTATUS (*SigNetIsServiceAccount)(_In_opt_ LPWSTR, _In_ LPWSTR, _Out_ BOOL *);
    HMODULE _logonCliDll;
    SigNetIsServiceAccount _fnNetIsServiceAccount;

  public:
    LogonCli();
    LogonCli(LogonCli &&other) noexcept;
    LogonCli &operator=(LogonCli &&) noexcept;
    ~LogonCli();

    NTSTATUS NetIsServiceAccount(_In_opt_ LPWSTR ServerName, _In_ LPWSTR AccountName, _Out_ BOOL *IsService);
};
