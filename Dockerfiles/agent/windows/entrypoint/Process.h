#pragma once
#include <windows.h>
#include <string>

class Process
{
private:
    Process(Process const&) = default;
    Process(Process&&) = default;
public:
    Process();

    HRESULT Create(std::wstring const& processCommandLine);

    HANDLE GetProcessHandle() const;

    HRESULT GetExitCode(DWORD& exitCode) const;

    ~Process();
private:
    PROCESS_INFORMATION _processInfo;
    STARTUPINFO         _startupInfo;
};

