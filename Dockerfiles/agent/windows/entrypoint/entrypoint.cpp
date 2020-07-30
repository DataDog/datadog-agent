// entrypoint.cpp : This file contains the 'main' function. Program execution begins and ends there.
//

#include <Windows.h>
#include <tchar.h>
#include <string>
#include <iostream>
#include <filesystem>
#include "Process.h"

namespace
{
    HANDLE StopEvent = INVALID_HANDLE_VALUE;
}

BOOL WINAPI CtrlHandle(DWORD dwCtrlType)
{
    switch (dwCtrlType)
    {
    case CTRL_C_EVENT:
    case CTRL_CLOSE_EVENT:
    case CTRL_BREAK_EVENT:
    case CTRL_LOGOFF_EVENT:
    case CTRL_SHUTDOWN_EVENT:
        std::cout << "[ENTRYPOINT][INFO] CTRL signal received, shutting down..." << std::endl;
        SetEvent(StopEvent);
        StopEvent = INVALID_HANDLE_VALUE;
        break;

    default:
        break;
    }

    return TRUE;
}

HRESULT WaitForProcessToExit(Process& process, DWORD timeoutValueInMs = 30000)
{
    HRESULT hr = S_OK;

    const DWORD waitResult = WaitForSingleObject(process.GetProcessHandle(), timeoutValueInMs);
    if (waitResult == WAIT_TIMEOUT)
    {
        if (!TerminateProcess(process.GetProcessHandle(), STATUS_TIMEOUT))
        {
            hr = HRESULT_FROM_WIN32(GetLastError());
            std::cout << "[ENTRYPOINT][ERROR] Failed to terminate process with error [" << std::hex << hr << "]" << std::endl;
        }
    }
    else if (waitResult == WAIT_OBJECT_0)
    {
        DWORD exitCode;
        hr = process.GetExitCode(exitCode);
        if (hr != S_OK)
        {
            std::cout << "[ENTRYPOINT][ERROR] Failed get process exit code with error [0x" << std::hex << hr << "]" << std::endl;
        }
        else
        {
            std::cout << std::endl << "[ENTRYPOINT][INFO] Process exited with code [0x" << std::hex << exitCode << "]" << std::endl;
            // Store exitCode in hr so that we can return that as our own exit code
            hr = exitCode;
        }
    }
    else
    {
        hr = HRESULT_FROM_WIN32(GetLastError());
        std::cout << "[ENTRYPOINT][ERROR] Failed to wait for process with error [0x" << std::hex << hr << "]" << std::endl;
    }

    return hr;
}

HRESULT MonitorProcess(std::wstring const & processCommandLine, bool restartUntilStopReceived)
{
    HRESULT hr = S_OK;
    do
    {
        std::cout << "[ENTRYPOINT][INFO] Starting process..." << std::endl;
        Process process;
        hr = process.Create(processCommandLine);
        if (hr != S_OK)
        {
            std::cout << "[ENTRYPOINT][ERROR] Failed to create process with error [0x" << std::hex << hr << "]" << std::endl;
            break;
        }
        HANDLE events[2] =
        {
            // Process handle needs to be last so that WaitForMultipleObjects
            // would return our StopEvent first in case they are signaled at the same time
            StopEvent,
            process.GetProcessHandle()
        };
        const DWORD waitResult = WaitForMultipleObjects(2, events, FALSE, INFINITE);

        if (waitResult == WAIT_OBJECT_0)
        {
            // Our stop event was signaled, we need to check the process
            hr = WaitForProcessToExit(process);
            break;
        }

        DWORD exitCode;
        hr = process.GetExitCode(exitCode);
        if (hr != S_OK)
        {
            std::cout << "[ENTRYPOINT][ERROR] Failed get process exit code with error [0x" << std::hex << hr << "]" << std::endl;
            break;
        }
        std::cout << "[ENTRYPOINT][INFO] Process exited with exit code [0x" << std::hex << exitCode << "]." << std::endl;

        if (restartUntilStopReceived)
        {
            std::cout << "[ENTRYPOINT][WARNING] Process exited before receiving stop signal, restarting..." << std::endl;
        }
    } while (restartUntilStopReceived);
    return hr;
}

HRESULT ExecuteInitScripts()
{
    HRESULT hr = S_OK;
    std::error_code ec;
    auto directoryIt = std::filesystem::directory_iterator("entrypoint-ps1", ec);
    if (ec)
    {
        hr = ec.value();
        std::cout << "[ENTRYPOINT][ERROR] Failed to get iterator to init scripts folder with error [0x" << std::hex << hr << "]" << std::endl;
        return hr;
    }

    for (auto& script : directoryIt)
    {
        std::wstring processCommandLine = L"pwsh " + script.path().wstring();
        Process pwsh;
        hr = pwsh.Create(processCommandLine);
        if (hr == S_OK)
        {
            hr = WaitForProcessToExit(pwsh);
        }
        if (hr != S_OK)
        {
            std::cout << "[ENTRYPOINT][ERROR] Failed to run init script " << script.path() << " with error [0x" << std::hex << hr << "]" << std::endl;
            break;
        }
    }

    return hr;
}

int _tmain(int argc, _TCHAR** argv)
{
    HRESULT hr = S_OK;

    if (argc <= 1)
    {
        std::cout << "Usage: entrypoint.exe <agent path> <agent args>" << std::endl;
        goto Cleanup;
    }

    StopEvent = CreateEvent(
        nullptr,            // default security attributes
        TRUE,               // manual-reset event
        FALSE,              // initial state is non-signaled
        nullptr             // object name
    );

    if (StopEvent == nullptr)
    {
        hr = HRESULT_FROM_WIN32(GetLastError());
        goto Cleanup;
    }

    if (!SetConsoleCtrlHandler(CtrlHandle, TRUE))
    {
        hr = HRESULT_FROM_WIN32(GetLastError());
        std::cout << "[ENTRYPOINT][ERROR] Failed to set control handle with error [0x" << std::hex << hr << "]" << std::endl;
        goto Cleanup;
    }

    if (SUCCEEDED(hr) && StopEvent != INVALID_HANDLE_VALUE)
    {
        if ((hr = ExecuteInitScripts()) != S_OK)
        {
            goto Cleanup;
        }

        std::wstring processCommandLine = argv[1];
        for (int i = 2; i < argc; ++i)
        {
            processCommandLine += L" ";
            processCommandLine += argv[i];
        }

        hr = MonitorProcess(processCommandLine, false);
    }

Cleanup:
    if (StopEvent != INVALID_HANDLE_VALUE && StopEvent != nullptr)
    {
        CloseHandle(StopEvent);
        StopEvent = INVALID_HANDLE_VALUE;
    }
    return hr;
}
