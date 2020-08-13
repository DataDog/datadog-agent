#pragma once
#include <Windows.h>
#include <string>
#include <chrono>

class Process
{
private:
    Process(Process const&) = delete;
    Process();
public:
    Process(Process&&);
    static Process Create(std::wstring const& processCommandLine);
    static Process Open(DWORD id);
    DWORD GetExitCode() const;

    //! Waits until the process exits, and returns its exit code.
    //! If the process hasn't exited after the timeout expires, the process is forcibly terminated.
    DWORD WaitForExit(std::chrono::milliseconds timeout = std::chrono::seconds(30)) const;

    HANDLE GetProcessHandle() const;

    ~Process();
private:
    PROCESS_INFORMATION _processInfo;
    STARTUPINFO         _startupInfo;
};

