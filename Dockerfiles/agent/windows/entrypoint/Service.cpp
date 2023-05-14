// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

#include <memory>
#include "Service.h"
#include "Win32Exception.h"

namespace
{
    auto heapFree = [](LPENUM_SERVICE_STATUS p) { HeapFree(GetProcessHeap(), 0, p); };
    typedef std::unique_ptr<ENUM_SERVICE_STATUS, decltype(heapFree)> ENUM_SERVICE_STATUS_PTR;
}

Service::Service(std::wstring const& name)
: _scManagerHandle(OpenSCManager(nullptr, nullptr, SC_MANAGER_CONNECT))
, _serviceHandle(nullptr)
{
    if (_scManagerHandle == nullptr)
    {
        throw Win32Exception("Could not open the service control manager");
    }
    _serviceHandle = OpenService(
        _scManagerHandle,
        name.c_str(),
        SERVICE_START | SERVICE_STOP | SERVICE_QUERY_STATUS | SERVICE_ENUMERATE_DEPENDENTS);
    if (_serviceHandle == nullptr)
    {
        throw Win32Exception("Could not open the service");
    }
}

Service::~Service()
{
    CloseServiceHandle(_scManagerHandle);
    CloseServiceHandle(_serviceHandle);
}

DWORD Service::PID()
{
    return _processId;
}

void Service::Start(std::chrono::milliseconds timeout)
{
    if (!StartService(_serviceHandle, 0, nullptr))
    {
        const DWORD lastError = GetLastError();
        if (lastError != ERROR_SERVICE_ALREADY_RUNNING)
        {
            throw Win32Exception("Could not start the service");
        }
    }

    SERVICE_STATUS_PROCESS serviceStatus;
    do
    {
        DWORD unused = 0;
        if (!QueryServiceStatusEx(_serviceHandle,
            SC_STATUS_PROCESS_INFO,
            reinterpret_cast<LPBYTE>(&serviceStatus),
            sizeof(SERVICE_STATUS_PROCESS),
            &unused))
        {
            throw Win32Exception("Could not query the service status");
        }
        
        if (serviceStatus.dwCurrentState != SERVICE_RUNNING)
        {
            timeout -= std::chrono::seconds(1);
            Sleep(1000);
            if (timeout.count() <= 0)
            {
                throw std::exception("Timeout while starting the service");
            }
        }
    } while (serviceStatus.dwCurrentState != SERVICE_RUNNING);
    _processId = serviceStatus.dwProcessId;
}

void Service::Stop(std::chrono::milliseconds timeout)
{
    DWORD sizeNeededDependentServices;
    DWORD countDependentServices;

    if (!EnumDependentServices(
        _serviceHandle,
        SERVICE_ACTIVE,
        nullptr,
        0,
        &sizeNeededDependentServices,
        &countDependentServices))
    {
        // If the Enum call fails, then there are dependent services to be stopped first
        if (GetLastError() != ERROR_MORE_DATA)
        {
            // The last error must be ERROR_MORE_DATA
            throw Win32Exception("Unexpected error while fetching dependent services");
        }

        ENUM_SERVICE_STATUS_PTR depSvcs(
            static_cast<LPENUM_SERVICE_STATUS>(
                HeapAlloc(GetProcessHeap(), HEAP_ZERO_MEMORY, sizeNeededDependentServices)), heapFree);

        if (!EnumDependentServices(
            _serviceHandle,
            SERVICE_ACTIVE,
            depSvcs.get(),
            sizeNeededDependentServices,
            &sizeNeededDependentServices,
            &countDependentServices))
        {
            throw Win32Exception("Could not enumerate dependent services");
        }
        for (DWORD i = 0; i < countDependentServices; ++i)
        {
            // Note that by giving dependent services the same timeout
            // we may exceed our timeout ourselves.
            Service(depSvcs.get()[i].lpServiceName).Stop(timeout);
        }
    }

    SERVICE_STATUS_PROCESS serviceStatus;
    if (!ControlService(_serviceHandle, SERVICE_CONTROL_STOP, reinterpret_cast<LPSERVICE_STATUS>(&serviceStatus)))
    {
        if (GetLastError() == ERROR_SERVICE_CANNOT_ACCEPT_CTRL &&
            (serviceStatus.dwCurrentState == SERVICE_STOPPED ||
             serviceStatus.dwCurrentState == SERVICE_STOP_PENDING))
        {
            // Service is already shut(ting) down
            return;
        }
        throw Win32Exception("Could not stop the service");
    }

    while (serviceStatus.dwCurrentState != SERVICE_STOPPED)
    {
        DWORD unused = 0;
        if (!QueryServiceStatusEx(_serviceHandle,
            SC_STATUS_PROCESS_INFO,
            reinterpret_cast<LPBYTE>(&serviceStatus),
            sizeof(SERVICE_STATUS_PROCESS),
            &unused))
        {
            throw Win32Exception("Could not query service status");
        }

        if (serviceStatus.dwCurrentState != SERVICE_STOPPED)
        {
            auto waitTime = std::chrono::milliseconds(serviceStatus.dwWaitHint) / 10;
            if (waitTime < std::chrono::seconds(1))
            {
                waitTime = std::chrono::seconds(1);
            }
            else if (waitTime > std::chrono::seconds(10))
            {
                waitTime = std::chrono::seconds(10);
            }
            Sleep(static_cast<DWORD>(waitTime.count()));
            timeout -= waitTime;
            if (timeout.count() <= 0)
            {
                throw std::exception("Timeout while stopping the service");
            }
        }
    }
}
