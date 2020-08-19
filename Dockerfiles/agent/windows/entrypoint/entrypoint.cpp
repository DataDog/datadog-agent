// entrypoint.cpp : This file contains the 'main' function. Program execution begins and ends there.
//

#include <Windows.h>
#include <tchar.h>
#include <string>
#include <iostream>
#include <filesystem>
#include <sstream>
#include "Process.h"
#include "Service.h"
#include "Win32Exception.h"
#include <fstream>
#include <thread>
#include <map>

namespace
{
    HANDLE StopEvent = INVALID_HANDLE_VALUE;
    const std::map<std::wstring, std::filesystem::path> services =
    {
        {L"datadogagent", "C:\\ProgramData\\Datadog\\logs\\agent.log"},
        {L"datadog-process-agent", "C:\\ProgramData\\Datadog\\logs\\process-agent.log"},
        {L"datadog-trace-agent", "C:\\ProgramData\\Datadog\\logs\\trace-agent.log"},
    };
    std::string FormatErrorCode(DWORD errorCode)
    {
        std::stringstream sstream;
        sstream << "[" << errorCode << " (0x" << std::hex << errorCode << ")]";
        return sstream.str();
    }
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

void StreamLogsToStdout(std::filesystem::path const& logFilePath)
{
    std::ifstream::pos_type lastPosition;
    while (true)
    {
        std::ifstream logFile(logFilePath);
        if (logFile)
        {
            logFile.seekg(0, std::ifstream::end);
            auto fpos = logFile.tellg();
            if (lastPosition > fpos)
            {
                // New file
                lastPosition = 0;
            }
            logFile.seekg(lastPosition);
            std::streambuf* pbuf = logFile.rdbuf();
            while (pbuf->sgetc() != EOF)
            {
                std::cout.put(pbuf->sbumpc());
            }
            lastPosition = fpos;
            logFile.close();
        }
        Sleep(1000);
    }
}

void RunService(std::wstring const& serviceName, std::filesystem::path const& logsPath)
{
    Service service(serviceName);
    std::wcout << L"[ENTRYPOINT][INFO] Starting service " << serviceName << std::endl;
    service.Start();
    std::wcout << L"[ENTRYPOINT][INFO] Success. Waiting for exit signal." << std::endl;
    std::thread logThread(StreamLogsToStdout, logsPath);
    logThread.detach();
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

void Cleanup()
{
    CloseHandle(StopEvent);
    StopEvent = nullptr;
}
int _tmain(int argc, _TCHAR** argv)
{
    DWORD exitCode = -1;

    if (argc <= 1)
    {
        std::cout << "Usage: entrypoint.exe <service> | <executable> <args>" << std::endl;
        return -1;
    }

    StopEvent = CreateEvent(
        nullptr,            // default security attributes
        TRUE,               // manual-reset event
        FALSE,              // initial state is non-signaled
        nullptr             // object name
    );

    if (StopEvent == nullptr)
    {
        std::cout << "[ENTRYPOINT][ERROR] Failed to create event with error: " << FormatErrorCode(GetLastError()) << std::endl;
        return -1;
    }

    if (!SetConsoleCtrlHandler(CtrlHandle, TRUE))
    {
        std::cout << "[ENTRYPOINT][ERROR] Failed to set control handle with error: " << FormatErrorCode(GetLastError()) << std::endl;
        Cleanup();
        return -1;
    }

    if (SUCCEEDED(hr) && StopEvent != INVALID_HANDLE_VALUE)
    {
        try
        {
            ExecuteInitScripts();
            std::wstring command = argv[1];


            for (auto service : services)
            {
                if (command == service.first)
                {
                    RunService(service.first, service.second);
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
        exitCode = 0;
    }
    {
        std::cout << "[ENTRYPOINT][ERROR] " << ex.what() << ". Error: " << FormatErrorCode(ex.GetErrorCode()) << std::endl;
    }
    Cleanup();
    return exitCode;
}
