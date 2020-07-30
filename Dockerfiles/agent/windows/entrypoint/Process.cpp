#include "Process.h"

Process::Process()
{
    ZeroMemory(&_startupInfo, sizeof _startupInfo);
    _startupInfo.cb = sizeof _startupInfo;
    ZeroMemory(&_processInfo, sizeof _processInfo);
}

HRESULT Process::Create(std::wstring const& processCommandLine)
{
    if (!CreateProcess(nullptr, // Module name
                       &const_cast<std::wstring&>(processCommandLine)[0], // Command line
                       nullptr, // Process handle not inheritable
                       nullptr, // Thread handle not inheritable
                       FALSE, // Set handle inheritance to FALSE
                       0, // No creation flags
                       nullptr, // Use parent's environment block
                       nullptr, // Use parent's starting directory 
                       &_startupInfo, // Pointer to STARTUPINFO structure
                       &_processInfo) // Pointer to PROCESS_INFORMATION structure
    )
    {
        return HRESULT_FROM_WIN32(GetLastError());
    }
    return S_OK;
}

HANDLE Process::GetProcessHandle() const
{
    return _processInfo.hProcess;
}

HRESULT Process::GetExitCode(DWORD& exitCode) const
{
    HRESULT hr = S_OK;
    if (!GetExitCodeProcess(GetProcessHandle(), &exitCode))
    {
        hr = HRESULT_FROM_WIN32(GetLastError());
    }
    return hr;
}

Process::~Process()
{
    if (_processInfo.hProcess != nullptr)
    {
        CloseHandle(_processInfo.hProcess);
    }
    if (_processInfo.hThread)
    {
        CloseHandle(_processInfo.hThread);
    }
}
