// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

#include "Process.h"
#include "Win32Exception.h"

Process::Process()
: _processInfo()
, _startupInfo()
{
    ZeroMemory(&_startupInfo, sizeof _startupInfo);
    _startupInfo.cb = sizeof _startupInfo;
    ZeroMemory(&_processInfo, sizeof _processInfo);
}

Process::Process(Process&& other)
: _processInfo(other._processInfo)
, _startupInfo(other._startupInfo)
{
    other._processInfo.hProcess = nullptr;
    other._processInfo.hThread = nullptr;
}

Process Process::Create(std::wstring const& processCommandLine)
{
    Process process;
    if (!CreateProcess(
        nullptr, // Module name
        &const_cast<std::wstring&>(processCommandLine)[0], // Command line
        nullptr, // Process handle not inheritable
        nullptr, // Thread handle not inheritable
        FALSE, // Set handle inheritance to FALSE
        0, // No creation flags
        nullptr, // Use parent's environment block
        nullptr, // Use parent's starting directory 
        &process._startupInfo, // Pointer to STARTUPINFO structure
        &process._processInfo) // Pointer to PROCESS_INFORMATION structure
    )
    {
        throw Win32Exception("Could not create process");
    }
    return process;
}

Process Process::Open(DWORD id)
{
    Process process;
    process._processInfo.hProcess = OpenProcess(PROCESS_ALL_ACCESS, FALSE, id);
    return process;
}

DWORD Process::GetExitCode() const
{
    DWORD exitCode;
    if (!GetExitCodeProcess(_processInfo.hProcess, &exitCode))
    {
        throw Win32Exception("Could not get exit code");
    }
    return exitCode;
}

DWORD Process::WaitForExit(std::chrono::milliseconds timeout) const
{
    const DWORD waitResult = WaitForSingleObject(_processInfo.hProcess, static_cast<DWORD>(timeout.count()));
    if (waitResult == WAIT_TIMEOUT)
    {
        if (!TerminateProcess(_processInfo.hProcess, STATUS_TIMEOUT))
        {
            throw Win32Exception("Failed to terminate process");
        }
        throw Win32Exception("Process took too long to exit and was terminated");
    }
    if (waitResult == WAIT_OBJECT_0)
    {
        return GetExitCode();
    }
    throw Win32Exception("WaitForSingleObject failed");
}

HANDLE Process::GetProcessHandle() const
{
    return _processInfo.hProcess;
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
