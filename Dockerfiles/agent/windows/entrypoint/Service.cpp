#include "Service.h"
#include "Win32Exception.h"

Service::Service(std::wstring const& name)
: _scManagerHandler(OpenSCManager(nullptr, nullptr, SC_MANAGER_CONNECT))
, _serviceHandle(nullptr)
{
    if (_scManagerHandler == nullptr)
    {
        throw Win32Exception("Could not establish a connection to the service control manager");
    }
    _serviceHandle = OpenService(_scManagerHandler, name.c_str(), SERVICE_START | SERVICE_STOP | SERVICE_QUERY_STATUS);
    if (_serviceHandle == nullptr)
    {
        throw Win32Exception("Could not open the service");
    }
}

Service::~Service()
{
    CloseServiceHandle(_scManagerHandler);
    CloseServiceHandle(_serviceHandle);
}

void Service::Start(std::chrono::milliseconds timeout)
{
    // If the function fails, the return value is zero
    if (StartService(_serviceHandle, 0, nullptr) == 0)
    {
        const DWORD lastError = GetLastError();
        if (lastError != ERROR_SERVICE_ALREADY_RUNNING)
        {
            throw Win32Exception("Could not start the service");
        }
    }

    while (timeout.count() > 0)
    {
        DWORD unused = 0;
        SERVICE_STATUS_PROCESS serviceStatus;

        if (!QueryServiceStatusEx(_serviceHandle,
            SC_STATUS_PROCESS_INFO,
            reinterpret_cast<LPBYTE>(&serviceStatus),
            sizeof(SERVICE_STATUS_PROCESS),
            &unused))
        {
            throw Win32Exception("Could not query the service status");
        }
        _processId = serviceStatus.dwProcessId;
        
        if (serviceStatus.dwCurrentState == SERVICE_RUNNING)
        {
            break;
        }
        if (serviceStatus.dwCurrentState == SERVICE_START_PENDING)
        {
            timeout -= std::chrono::seconds(1);
            Sleep(static_cast<DWORD>(timeout.count()));
        }
        else
        {
            throw std::exception("Could not start the service");
        }
    }
}

void Service::Stop(std::chrono::milliseconds timeout)
{
    while (timeout.count() > 0)
    {
        DWORD unused = 0;
        SERVICE_STATUS_PROCESS serviceStatus;

        if (!QueryServiceStatusEx(_serviceHandle,
            SC_STATUS_PROCESS_INFO,
            reinterpret_cast<LPBYTE>(&serviceStatus),
            sizeof(SERVICE_STATUS_PROCESS),
            &unused))
        {
            throw Win32Exception("Could not query service status");
        }

        if (serviceStatus.dwCurrentState == SERVICE_STOPPED)
        {
            break;
        }
        if (serviceStatus.dwCurrentState == SERVICE_STOP_PENDING ||
            serviceStatus.dwCurrentState == SERVICE_START_PENDING ||
            serviceStatus.dwCurrentState == SERVICE_PAUSE_PENDING ||
            serviceStatus.dwCurrentState == SERVICE_CONTINUE_PENDING)
        {
            timeout -= std::chrono::seconds(1);
            Sleep(static_cast<DWORD>(timeout.count()));
        }
        else if (serviceStatus.dwCurrentState == SERVICE_RUNNING || serviceStatus.dwCurrentState == SERVICE_PAUSED)
        {
            if (!ControlService(_serviceHandle, SERVICE_CONTROL_STOP, reinterpret_cast<LPSERVICE_STATUS>(&serviceStatus)))
            {
                throw Win32Exception("Could not stop the service");
            }
        }
        else
        {
            throw std::exception("Could not start the service");
        }
    }
}
