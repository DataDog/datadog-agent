// entrypoint.cpp : This file contains the 'main' function. Program execution begins and ends there.
//

#include <Windows.h>
#include <tchar.h>
#include <string>
#include <iostream>
#include <filesystem>
#include <array>
#include <sstream>
#include "Process.h"
#include "Service.h"
#include "Win32Exception.h"

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
        break;

    default:
        break;
    }

    return TRUE;
}

void ExecuteInitScripts()
{
    auto directoryIt = std::filesystem::directory_iterator("entrypoint-ps1");
    for (auto& script : directoryIt)
    {
        Process pwsh = Process::Create(L"pwsh " + script.path().wstring());
        DWORD exitCode = pwsh.WaitForExit();
        if (exitCode != 0)
        {
            std::cout << "[ENTRYPOINT][WARNING] " << script.path() << " exited with code [" << std::hex << exitCode << "]" << std::endl;
        }
    }
}

void RunService(std::wstring const& serviceName)
{
    Service service(serviceName);
    std::wcout << L"[ENTRYPOINT][INFO] Starting service " << serviceName << std::endl;
    service.Start();
    std::wcout << L"[ENTRYPOINT][INFO] Success. Waiting for exit signal." << std::endl;
    WaitForSingleObject(StopEvent, INFINITE);
    std::wcout << L"[ENTRYPOINT][INFO] Stopping service " << serviceName << std::endl;
    try
    {
        service.Stop();
    }
    catch (...)
    {
        std::wcout << L"[ENTRYPOINT][INFO] Could not stop " << serviceName << ". Trying to kill process." << std::endl;
        TerminateProcess(OpenProcess(PROCESS_ALL_ACCESS, FALSE, service.PID()), STATUS_TIMEOUT);
        throw;
    }
}

void RunExecutable(std::wstring const& command)
{
    std::wcout << L"[ENTRYPOINT][INFO] Starting process " << command << std::endl;
    Process process = Process::Create(command);
    std::wcout << GetLastError() << std::endl;
    HANDLE events[2] =
    {
        // Process handle needs to be last so that WaitForMultipleObjects
        // would return our StopEvent first in case they are signaled at the same time
        StopEvent,
        process.GetProcessHandle()
    };
    const DWORD waitResult = WaitForMultipleObjects(2, events, FALSE, INFINITE);
    DWORD exitCode;
    if (waitResult == WAIT_FAILED)
    {
        throw Win32Exception("Failed to wait for objects");
    }

    if (waitResult == WAIT_OBJECT_0)
    {
        exitCode = process.WaitForExit();
    }
    else
    {
        exitCode = process.GetExitCode();
        SetEvent(StopEvent);
    }
    std::wcout << L"[ENTRYPOINT][INFO] Command '" << command << L"' exited with code [0x" << std::hex << exitCode << L"]" << std::endl;
}

int _tmain(int argc, _TCHAR** argv)
{
    HRESULT hr = S_OK;

    if (argc <= 1)
    {
        std::cout << "Usage: entrypoint.exe <service> | <executable> <args>" << std::endl;
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
        try
        {
            ExecuteInitScripts();
            std::wstring command = argv[1];

            const std::array <std::wstring, 3> servicesName =
            {
                L"datadogagent",
                L"datadog-process-agent",
                L"datadog-trace-agent"
            };

            for (const std::wstring& serviceName : servicesName)
            {
                if (command == serviceName)
                {
                    RunService(serviceName);
                    goto Cleanup;
                }
            }

            std::wstringstream commandLine;
            commandLine << command;
            for (int i = 2; i < argc; ++i)
            {
                commandLine << L" " << argv[i];
            }
            RunExecutable(commandLine.str());
        }
        catch (std::exception & ex)
        {
            std::cout << "[ENTRYPOINT][ERROR] " << ex.what() << std::endl;
        }
    }

Cleanup:
    if (StopEvent != INVALID_HANDLE_VALUE && StopEvent != nullptr)
    {
        CloseHandle(StopEvent);
        StopEvent = INVALID_HANDLE_VALUE;
    }
    return hr;
}
