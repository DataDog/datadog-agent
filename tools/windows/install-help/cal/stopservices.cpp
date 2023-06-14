#include "stdafx.h"

static BOOL StopDependentServices(SC_HANDLE hScManager, SC_HANDLE hService);
static VOID DoStopSvc(const wchar_t *);
VOID DoStopAllServices()
{
    /*
     * temporary, clunky workaround to account for subservices running when main
     * agent is not
     */
    DoStopSvc(L"datadog-system-probe");
    DoStopSvc(L"datadog-process-agent");
    DoStopSvc(L"datadog-trace-agent");
    DoStopSvc(L"datadogagent");
}
int doesServiceExist(std::wstring &svcName)
{
    SC_HANDLE hScManager = NULL;
    SC_HANDLE hService = NULL;
    int retval = 0;
    // Get a handle to the SCM database.

    hScManager = OpenSCManager(NULL,                   // local computer
                               NULL,                   // ServicesActive database
                               SC_MANAGER_ALL_ACCESS); // full access rights

    if (NULL == hScManager)
    {
        WcaLog(LOGMSG_STANDARD, "OpenSCManager failed (%d)\n", GetLastError());
        return -1;
    }

    // Get a handle to the service.

    hService = OpenService(hScManager,      // SCM database
                           svcName.c_str(), // name of service
                           SERVICE_STOP | SERVICE_QUERY_STATUS | SERVICE_ENUMERATE_DEPENDENTS);

    if (hService == NULL)
    {
        DWORD err = GetLastError();
        if (err == ERROR_SERVICE_DOES_NOT_EXIST)
        {
            // this is an expected error
            retval = 0;
            WcaLog(LOGMSG_STANDARD, "Requested service does not exist");
        }
        else
        {
            WcaLog(LOGMSG_STANDARD, "Unexpected error querying service %d 0x%x", err, err);
            retval = -1;
        }
    }
    else
    {
        WcaLog(LOGMSG_STANDARD, "Requested service exists in SCM database");
        retval = 1;
        CloseServiceHandle(hService);
    }
    CloseServiceHandle(hScManager);
    return retval;
}
//
// Purpose:
//   Stops the service.
//
// Parameters:
//   None
//
// Return value:
//   None
//
VOID DoStopSvc(const wchar_t *inSvcName)
{
    SERVICE_STATUS_PROCESS ssp;
    DWORD dwStartTime = GetTickCount();
    DWORD dwBytesNeeded;
    DWORD dwTimeout = 30000; // 30-second time-out
    DWORD dwWaitTime;
    SC_HANDLE hScManager = NULL;
    SC_HANDLE hService = NULL;
    std::wstring svcName = inSvcName;

    // Get a handle to the SCM database.
    WcaLog(LOGMSG_STANDARD, "Stopping service %S", svcName.c_str());
    hScManager = OpenSCManager(NULL,                   // local computer
                               NULL,                   // ServicesActive database
                               SC_MANAGER_ALL_ACCESS); // full access rights

    if (NULL == hScManager)
    {
        WcaLog(LOGMSG_STANDARD, "OpenSCManager failed (%d)\n", GetLastError());
        return;
    }

    // Get a handle to the service.

    hService = OpenService(hScManager,      // SCM database
                           svcName.c_str(), // name of service
                           SERVICE_STOP | SERVICE_QUERY_STATUS | SERVICE_ENUMERATE_DEPENDENTS);

    if (hService == NULL)
    {
        DWORD err = GetLastError();
        if (ERROR_SERVICE_DOES_NOT_EXIST == err)
        {
            WcaLog(LOGMSG_STANDARD, "Didn't stop service: Service not found (this is expected on new installs)");
        }
        else
        {
            WcaLog(LOGMSG_STANDARD, "Didn't stop service: OpenService failed (%d)\n", err);
        }
        CloseServiceHandle(hScManager);
        return;
    }

    // Make sure the service is not already stopped.

    if (!QueryServiceStatusEx(hService, SC_STATUS_PROCESS_INFO, (LPBYTE)&ssp, sizeof(SERVICE_STATUS_PROCESS),
                              &dwBytesNeeded))
    {
        WcaLog(LOGMSG_STANDARD, "QueryServiceStatusEx failed (%d)\n", GetLastError());
        goto stop_cleanup;
    }

    if (ssp.dwCurrentState == SERVICE_STOPPED)
    {
        WcaLog(LOGMSG_STANDARD, "Service is already stopped.\n");
        goto stop_cleanup;
    }

    // If a stop is pending, wait for it.

    while (ssp.dwCurrentState == SERVICE_STOP_PENDING)
    {
        WcaLog(LOGMSG_STANDARD, "Service stop pending...\n");

        // Do not wait longer than the wait hint. A good interval is
        // one-tenth of the wait hint but not less than 1 second
        // and not more than 10 seconds.

        dwWaitTime = ssp.dwWaitHint / 10;

        if (dwWaitTime < 1000)
            dwWaitTime = 1000;
        else if (dwWaitTime > 10000)
            dwWaitTime = 10000;

        Sleep(dwWaitTime);

        if (!QueryServiceStatusEx(hService, SC_STATUS_PROCESS_INFO, (LPBYTE)&ssp, sizeof(SERVICE_STATUS_PROCESS),
                                  &dwBytesNeeded))
        {
            WcaLog(LOGMSG_STANDARD, "QueryServiceStatusEx failed (%d)\n", GetLastError());
            goto stop_cleanup;
        }

        if (ssp.dwCurrentState == SERVICE_STOPPED)
        {
            WcaLog(LOGMSG_STANDARD, "Service stopped successfully.\n");
            goto stop_cleanup;
        }

        if (GetTickCount() - dwStartTime > dwTimeout)
        {
            WcaLog(LOGMSG_STANDARD, "Service stop timed out.\n");
            goto stop_cleanup;
        }
    }

    // If the service is running, dependencies must be stopped first.

    StopDependentServices(hScManager, hService);

    // Send a stop code to the service.

    if (!ControlService(hService, SERVICE_CONTROL_STOP, (LPSERVICE_STATUS)&ssp))
    {
        WcaLog(LOGMSG_STANDARD, "ControlService failed (%d)\n", GetLastError());
        goto stop_cleanup;
    }

    // Wait for the service to stop.

    while (ssp.dwCurrentState != SERVICE_STOPPED)
    {
        Sleep(ssp.dwWaitHint);
        if (!QueryServiceStatusEx(hService, SC_STATUS_PROCESS_INFO, (LPBYTE)&ssp, sizeof(SERVICE_STATUS_PROCESS),
                                  &dwBytesNeeded))
        {
            WcaLog(LOGMSG_STANDARD, "QueryServiceStatusEx failed (%d)\n", GetLastError());
            goto stop_cleanup;
        }

        if (ssp.dwCurrentState == SERVICE_STOPPED)
            break;

        if (GetTickCount() - dwStartTime > dwTimeout)
        {
            WcaLog(LOGMSG_STANDARD, "Wait timed out\n");
            goto stop_cleanup;
        }
    }
    WcaLog(LOGMSG_STANDARD, "Service stopped successfully\n");

stop_cleanup:
    if (hService)
    {
        CloseServiceHandle(hService);
    }
    if (hScManager)
    {
        CloseServiceHandle(hScManager);
    }
}

BOOL StopDependentServices(SC_HANDLE hScManager, SC_HANDLE hService)
{
    DWORD i;
    DWORD dwBytesNeeded;
    DWORD dwCount;

    LPENUM_SERVICE_STATUS lpDependencies = NULL;
    ENUM_SERVICE_STATUS ess;
    SC_HANDLE hDepService;
    SERVICE_STATUS_PROCESS ssp;

    DWORD dwStartTime = GetTickCount();
    DWORD dwTimeout = 30000; // 30-second time-out

    // Pass a zero-length buffer to get the required buffer size.
    if (EnumDependentServices(hService, SERVICE_ACTIVE, lpDependencies, 0, &dwBytesNeeded, &dwCount))
    {
        // If the Enum call succeeds, then there are no dependent
        // services, so do nothing.
        return TRUE;
    }
    else
    {
        if (GetLastError() != ERROR_MORE_DATA)
            return FALSE; // Unexpected error

        // Allocate a buffer for the dependencies.
        lpDependencies = (LPENUM_SERVICE_STATUS)HeapAlloc(GetProcessHeap(), HEAP_ZERO_MEMORY, dwBytesNeeded);

        if (!lpDependencies)
            return FALSE;

        __try
        {
            // Enumerate the dependencies.
            if (!EnumDependentServices(hService, SERVICE_ACTIVE, lpDependencies, dwBytesNeeded, &dwBytesNeeded,
                                       &dwCount))
                return FALSE;

            for (i = 0; i < dwCount; i++)
            {
                ess = *(lpDependencies + i);
                // Open the service.
                hDepService = OpenService(hScManager, ess.lpServiceName, SERVICE_STOP | SERVICE_QUERY_STATUS);

                if (!hDepService)
                    return FALSE;

                __try
                {
                    // Send a stop code.
                    if (!ControlService(hDepService, SERVICE_CONTROL_STOP, (LPSERVICE_STATUS)&ssp))
                        return FALSE;

                    // Wait for the service to stop.
                    while (ssp.dwCurrentState != SERVICE_STOPPED)
                    {
                        Sleep(ssp.dwWaitHint);
                        if (!QueryServiceStatusEx(hDepService, SC_STATUS_PROCESS_INFO, (LPBYTE)&ssp,
                                                  sizeof(SERVICE_STATUS_PROCESS), &dwBytesNeeded))
                            return FALSE;

                        if (ssp.dwCurrentState == SERVICE_STOPPED)
                            break;

                        if (GetTickCount() - dwStartTime > dwTimeout)
                            return FALSE;
                    }
                }
                __finally
                {
                    // Always release the service handle.
                    CloseServiceHandle(hDepService);
                }
            }
        }
        __finally
        {
            // Always free the enumeration buffer.
            HeapFree(GetProcessHeap(), 0, lpDependencies);
        }
    }
    return TRUE;
}

//
// Purpose:
//   Starts the service if possible.
//
// Parameters:
//   None
//
// Return value:
//   None
//
DWORD DoStartSvc(std::wstring &svcname)
{
    SERVICE_STATUS_PROCESS ssStatus;
    DWORD dwOldCheckPoint;
    DWORD dwStartTickCount;
    DWORD dwWaitTime;
    DWORD dwBytesNeeded;
    DWORD err = 0;
    SC_HANDLE schSCManager = NULL;
    WcaLog(LOGMSG_STANDARD, "Starting service %S", svcname.c_str());
    // Get a handle to the SCM database.

    schSCManager = OpenSCManager(NULL,                   // local computer
                                 NULL,                   // servicesActive database
                                 SC_MANAGER_ALL_ACCESS); // full access rights

    if (NULL == schSCManager)
    {
        err = GetLastError();
        WcaLog(LOGMSG_STANDARD, "Failed to openSCManager %d", err);
        return err;
    }
    SC_HANDLE schService = NULL;
    // Get a handle to the service.

    schService = OpenService(schSCManager,        // SCM database
                             svcname.c_str(),     // name of service
                             SERVICE_ALL_ACCESS); // full access

    if (schService == NULL)
    {
        err = GetLastError();
        WcaLog(LOGMSG_STANDARD, "Failed to Open Service %d", err);
        goto doneStartService;
    }

    // Check the status in case the service is not stopped.

    if (!QueryServiceStatusEx(schService,                     // handle to service
                              SC_STATUS_PROCESS_INFO,         // information level
                              (LPBYTE)&ssStatus,              // address of structure
                              sizeof(SERVICE_STATUS_PROCESS), // size of structure
                              &dwBytesNeeded))                // size needed if buffer is too small
    {
        err = GetLastError();
        WcaLog(LOGMSG_STANDARD, "QueryServiceStatusEx failed (%d)\n", err);
        goto doneStartService;
    }

    // Check if the service is already running. It would be possible
    // to stop the service here, but for simplicity this example just returns.

    if (ssStatus.dwCurrentState != SERVICE_STOPPED && ssStatus.dwCurrentState != SERVICE_STOP_PENDING)
    {
        WcaLog(LOGMSG_STANDARD, "Cannot start the service because it is already running\n");
        err = ERROR_ALREADY_EXISTS;
        goto doneStartService;
    }

    // Save the tick count and initial checkpoint.

    dwStartTickCount = GetTickCount();
    dwOldCheckPoint = ssStatus.dwCheckPoint;

    // Wait for the service to stop before attempting to start it.

    while (ssStatus.dwCurrentState == SERVICE_STOP_PENDING)
    {
        // Do not wait longer than the wait hint. A good interval is
        // one-tenth of the wait hint but not less than 1 second
        // and not more than 10 seconds.

        dwWaitTime = ssStatus.dwWaitHint / 10;

        if (dwWaitTime < 1000)
            dwWaitTime = 1000;
        else if (dwWaitTime > 10000)
            dwWaitTime = 10000;

        Sleep(dwWaitTime);

        // Check the status until the service is no longer stop pending.

        if (!QueryServiceStatusEx(schService,                     // handle to service
                                  SC_STATUS_PROCESS_INFO,         // information level
                                  (LPBYTE)&ssStatus,              // address of structure
                                  sizeof(SERVICE_STATUS_PROCESS), // size of structure
                                  &dwBytesNeeded))                // size needed if buffer is too small
        {
            err = GetLastError();
            WcaLog(LOGMSG_STANDARD, "QueryServiceStatusEx failed (%d)\n", err);
            goto doneStartService;
        }

        if (ssStatus.dwCheckPoint > dwOldCheckPoint)
        {
            // Continue to wait and check.

            dwStartTickCount = GetTickCount();
            dwOldCheckPoint = ssStatus.dwCheckPoint;
        }
        else
        {
            if (GetTickCount() - dwStartTickCount > ssStatus.dwWaitHint)
            {
                err = ERROR_TIMEOUT;
                WcaLog(LOGMSG_STANDARD, "Timeout waiting for service to stop\n");
                goto doneStartService;
            }
        }
    }

    // Attempt to start the service.

    if (!StartService(schService, // handle to service
                      0,          // number of arguments
                      NULL))      // no arguments
    {
        err = GetLastError();
        WcaLog(LOGMSG_STANDARD, "StartService failed (%d)\n", err);
        goto doneStartService;
    }
    else
        WcaLog(LOGMSG_STANDARD, "Service start pending...\n");

    // Check the status until the service is no longer start pending.

    if (!QueryServiceStatusEx(schService,                     // handle to service
                              SC_STATUS_PROCESS_INFO,         // info level
                              (LPBYTE)&ssStatus,              // address of structure
                              sizeof(SERVICE_STATUS_PROCESS), // size of structure
                              &dwBytesNeeded))                // if buffer too small
    {
        err = GetLastError();
        WcaLog(LOGMSG_STANDARD, "QueryServiceStatusEx failed (%d)\n", err);
        goto doneStartService;
    }

    // Save the tick count and initial checkpoint.

    dwStartTickCount = GetTickCount();
    dwOldCheckPoint = ssStatus.dwCheckPoint;

    while (ssStatus.dwCurrentState == SERVICE_START_PENDING)
    {
        // Do not wait longer than the wait hint. A good interval is
        // one-tenth the wait hint, but no less than 1 second and no
        // more than 10 seconds.

        dwWaitTime = ssStatus.dwWaitHint / 10;

        if (dwWaitTime < 1000)
            dwWaitTime = 1000;
        else if (dwWaitTime > 10000)
            dwWaitTime = 10000;

        Sleep(dwWaitTime);

        // Check the status again.

        if (!QueryServiceStatusEx(schService,                     // handle to service
                                  SC_STATUS_PROCESS_INFO,         // info level
                                  (LPBYTE)&ssStatus,              // address of structure
                                  sizeof(SERVICE_STATUS_PROCESS), // size of structure
                                  &dwBytesNeeded))                // if buffer too small
        {
            WcaLog(LOGMSG_STANDARD, "QueryServiceStatusEx failed (%d)\n", err);
            break;
        }

        if (ssStatus.dwCheckPoint > dwOldCheckPoint)
        {
            // Continue to wait and check.

            dwStartTickCount = GetTickCount();
            dwOldCheckPoint = ssStatus.dwCheckPoint;
        }
        else
        {
            if (GetTickCount() - dwStartTickCount > ssStatus.dwWaitHint)
            {
                // No progress made within the wait hint.
                WcaLog(LOGMSG_STANDARD, "Exiting start loop; no progress made after %d ms",
                       (int)(GetTickCount() - dwStartTickCount));
                break;
            }
        }
    }

    // Determine whether the service is running.

    if (ssStatus.dwCurrentState == SERVICE_RUNNING)
    {
        WcaLog(LOGMSG_STANDARD, "Service started successfully (Elapsed %d)\n",
               (int)(GetTickCount() - dwStartTickCount));
    }
    else if (ssStatus.dwCurrentState == SERVICE_START_PENDING)
    {
        WcaLog(LOGMSG_STANDARD, "Service start in progress, continuing install (Elapsed %d)\n",
               (int)(GetTickCount() - dwStartTickCount));
    }
    else
    {
        WcaLog(LOGMSG_STANDARD, "Service not started. (Elapsed %d)\n", (int)(GetTickCount() - dwStartTickCount));
        WcaLog(LOGMSG_STANDARD, "  Current State: %d\n", ssStatus.dwCurrentState);
        WcaLog(LOGMSG_STANDARD, "  Exit Code: %d\n", ssStatus.dwWin32ExitCode);
        WcaLog(LOGMSG_STANDARD, "  Check Point: %d\n", ssStatus.dwCheckPoint);
        WcaLog(LOGMSG_STANDARD, "  Wait Hint: %d\n", ssStatus.dwWaitHint);
        err = ERROR_SERVICE_SPECIFIC_ERROR;
    }
doneStartService:
    if (schService)
    {
        CloseServiceHandle(schService);
    }
    if (schSCManager)
    {
        CloseServiceHandle(schSCManager);
    }
    return err;
}

class serviceDef
{
  private:
    const wchar_t *svcName;
    const wchar_t *displayName;
    const wchar_t *displayDescription;
    DWORD access;
    DWORD serviceType;
    DWORD startType;
    DWORD dwErrorControl;
    const wchar_t *lpBinaryPathName;
    const wchar_t *lpLoadOrderGroup;
    LPDWORD lpdwTagId;
    const wchar_t *lpDependencies; // list of single-null-terminated strings, double null at end
    const wchar_t *lpServiceStartName;
    const wchar_t *lpPassword;

  public:
    serviceDef()
        : svcName(NULL)
        , displayName(NULL)
        , displayDescription(NULL)
        , access(SERVICE_ALL_ACCESS)
        , serviceType(SERVICE_WIN32_OWN_PROCESS)
        , startType(SERVICE_DEMAND_START)
        , dwErrorControl(SERVICE_ERROR_NORMAL)
        , lpBinaryPathName(NULL)
        , lpLoadOrderGroup(NULL)
        , // not needed
        lpdwTagId(NULL)
        , // no tag identifier
        lpDependencies(NULL)
        , // no dependencies to start
        lpServiceStartName(NULL)
        ,                // will set to LOCAL_SYSTEM by default
        lpPassword(NULL) // no password for LOCAL_SYSTEM
    {
    }
    serviceDef(const wchar_t *name)
        : svcName(name)
        , displayName(NULL)
        , displayDescription(NULL)
        , access(SERVICE_ALL_ACCESS)
        , serviceType(SERVICE_WIN32_OWN_PROCESS)
        , startType(SERVICE_DEMAND_START)
        , dwErrorControl(SERVICE_ERROR_NORMAL)
        , lpBinaryPathName(NULL)
        , lpLoadOrderGroup(NULL)
        , // not needed
        lpdwTagId(NULL)
        , // no tag identifier
        lpDependencies(NULL)
        , // no dependencies to start
        lpServiceStartName(NULL)
        ,                // will set to LOCAL_SYSTEM by default
        lpPassword(NULL) // no password for LOCAL_SYSTEM
    {
    }

    serviceDef(const wchar_t *name, const wchar_t *display, const wchar_t *desc, const wchar_t *path,
               const wchar_t *deps, DWORD st, const wchar_t *user, const wchar_t *pass)
        : svcName(name)
        , displayName(display)
        , displayDescription(desc)
        , access(SERVICE_ALL_ACCESS)
        , serviceType(SERVICE_WIN32_OWN_PROCESS)
        , startType(st)
        , dwErrorControl(SERVICE_ERROR_NORMAL)
        , lpBinaryPathName(path)
        , lpLoadOrderGroup(NULL)
        , // not needed
        lpdwTagId(NULL)
        , // no tag identifier
        lpDependencies(deps)
        , lpServiceStartName(user)
        , lpPassword(pass) // no password for LOCAL_SYSTEM
    {
    }

    DWORD create(SC_HANDLE hMgr)
    {
        DWORD retval = 0;
        WcaLog(LOGMSG_STANDARD, "serviceDef::create()");
        SC_HANDLE hService =
            CreateService(hMgr, this->svcName, this->displayName, this->access, this->serviceType, this->startType,
                          this->dwErrorControl, this->lpBinaryPathName, this->lpLoadOrderGroup, this->lpdwTagId,
                          this->lpDependencies, this->lpServiceStartName, this->lpPassword);
        if (!hService)
        {

            retval = GetLastError();
            WcaLog(LOGMSG_STANDARD, "Failed to CreateService %d", retval);
            return retval;
        }
        WcaLog(LOGMSG_STANDARD, "Created Service");
        if (this->startType == SERVICE_AUTO_START)
        {
            // make it delayed-auto-start
            SERVICE_DELAYED_AUTO_START_INFO inf = {TRUE};
            WcaLog(LOGMSG_STANDARD, "setting to delayed auto start");
            ChangeServiceConfig2(hService, SERVICE_CONFIG_DELAYED_AUTO_START_INFO, (LPVOID)&inf);
            WcaLog(LOGMSG_STANDARD, "done setting to delayed auto start");
        }
        // set the description
        if (this->displayDescription)
        {
            WcaLog(LOGMSG_STANDARD, "setting description");
            SERVICE_DESCRIPTION desc = {(LPWSTR)this->displayDescription};
            ChangeServiceConfig2(hService, SERVICE_CONFIG_DESCRIPTION, (LPVOID)&desc);
            WcaLog(LOGMSG_STANDARD, "done setting description");
        }
        // set the error recovery actions
        SC_ACTION actions[4] = {
            {SC_ACTION_RESTART, 60000}, // restart after 60 seconds
            {SC_ACTION_RESTART, 60000}, // restart after 60 seconds
            {SC_ACTION_RESTART, 60000}, // restart after 60 seconds
            {SC_ACTION_NONE, 0},        // restart after 60 seconds
        };
        SERVICE_FAILURE_ACTIONS failactions = {60,   // reset count after 60 seconds
                                               NULL, // no reboot message
                                               NULL, // no command to execute
                                               4,    // 4 actions
                                               actions};
        WcaLog(LOGMSG_STANDARD, "Setting failure actions");
        ChangeServiceConfig2(hService, SERVICE_CONFIG_FAILURE_ACTIONS, (LPVOID)&failactions);
        WcaLog(LOGMSG_STANDARD, "Done with create() %d", retval);
        return retval;
    }

    DWORD destroy(SC_HANDLE hMgr)
    {
        SC_HANDLE hService = OpenService(hMgr, this->svcName, DELETE);
        if (!hService)
        {
            return GetLastError();
        }
        DWORD retval = 0;
        if (!DeleteService(hService))
        {
            retval = GetLastError();
        }
        CloseServiceHandle(hService);
        return retval;
    }
    DWORD verify(SC_HANDLE hMgr)
    {
        SC_HANDLE hService = OpenService(hMgr, this->svcName, SC_MANAGER_ALL_ACCESS);
        if (!hService)
        {
            return GetLastError();
        }
        DWORD retval = 0;
#define QUERY_BUF_SIZE 8192
        //////
        // from 6.11 to 6.12, the location of the service binary changed.  Check the location
        // vs the expected location, and change if it's different
        char *buf = new char[QUERY_BUF_SIZE];
        memset(buf, 0, QUERY_BUF_SIZE);
        QUERY_SERVICE_CONFIGW *cfg = (QUERY_SERVICE_CONFIGW *)buf;
        DWORD needed = 0;
        if (!QueryServiceConfigW(hService, cfg, QUERY_BUF_SIZE, &needed))
        {
            // shouldn't ever fail.  WE're supplying the largest possible buffer
            // according to the docs.
            retval = GetLastError();
            WcaLog(LOGMSG_STANDARD, "Failed to query service status %d\n", retval);
            goto done_verify;
        }
        if (_wcsicmp(cfg->lpBinaryPathName, this->lpBinaryPathName) == 0)
        {
            // nothing to do, already correctly configured
            WcaLog(LOGMSG_STANDARD, "Service path already correct");
        }
        else
        {
            BOOL bRet = ChangeServiceConfigW(hService, SERVICE_NO_CHANGE, SERVICE_NO_CHANGE, SERVICE_NO_CHANGE,
                                             this->lpBinaryPathName, NULL, NULL, NULL, NULL, NULL, NULL);
            if (!bRet)
            {
                retval = GetLastError();
                WcaLog(LOGMSG_STANDARD, "Failed to update service config %d\n", retval);
                goto done_verify;
            }
            WcaLog(LOGMSG_STANDARD, "Updated path for existing service");
        }
        {
            WcaLog(LOGMSG_STANDARD, "Resetting dependencies");
            BOOL bRet = ChangeServiceConfigW(hService, SERVICE_NO_CHANGE, SERVICE_NO_CHANGE, SERVICE_NO_CHANGE, NULL,
                                             NULL, NULL, this->lpDependencies, NULL, NULL, NULL);
            if (!bRet)
            {
                retval = GetLastError();
                WcaLog(LOGMSG_STANDARD, "Failed to update service dependency config %d\n", retval);
                goto done_verify;
            }
            WcaLog(LOGMSG_STANDARD, "Updated dependencies for existing service, dependencies now %S",
                   this->lpDependencies);
        }

    done_verify:
        CloseServiceHandle(hService);
        if (buf)
        {
            delete[] buf;
        }

        return retval;
    }
    const wchar_t *getServiceName() const
    {
        return this->svcName;
    }
};

static wchar_t *probeDepsNoNPM = L"datadogagent\0\0";
static wchar_t *probeDepsWithNPM = L"datadogagent\0ddnpm\0\0";

int installServices(CustomActionData &data, PSID sid, const wchar_t *password)
{
    SC_HANDLE hScManager = NULL;
    SC_HANDLE hService = NULL;
    int retval = 0;
    // Get a handle to the SCM database.

#define NUM_SERVICES 4
    serviceDef services[NUM_SERVICES] = {
        serviceDef(agentService.c_str(), L"Datadog Agent", L"Send metrics to Datadog", agent_exe.c_str(), NULL,
                   SERVICE_AUTO_START, data.FullyQualifiedUsername().c_str(), password),
        serviceDef(traceService.c_str(), L"Datadog Trace Agent", L"Send tracing metrics to Datadog", trace_exe.c_str(),
                   L"datadogagent\0\0", SERVICE_DEMAND_START, data.FullyQualifiedUsername().c_str(), password),
        serviceDef(processService.c_str(), L"Datadog Process Agent", L"Send process metrics to Datadog",
                   process_exe.c_str(), L"datadogagent\0\0", SERVICE_DEMAND_START, NULL, NULL),
        serviceDef(systemProbeService.c_str(), L"Datadog System Probe", L"Send network metrics to Datadog",
                   sysprobe_exe.c_str(), probeDepsNoNPM, SERVICE_DEMAND_START,
                   NULL, NULL)

    };
    // by default, don't add sysprobe
    int servicesToInstall = NUM_SERVICES;

    WcaLog(LOGMSG_STANDARD, "Installing services");
    hScManager = OpenSCManager(NULL,                   // local computer
                               NULL,                   // ServicesActive database
                               SC_MANAGER_ALL_ACCESS); // full access rights

    if (NULL == hScManager)
    {
        WcaLog(LOGMSG_STANDARD, "OpenSCManager failed (%d)\n", GetLastError());
        return -1;
    }
    for (int i = 0; i < servicesToInstall; i++)
    {
        WcaLog(LOGMSG_STANDARD, "installing service %d", i);
        retval = services[i].create(hScManager);
        if (retval != 0)
        {
            WcaLog(LOGMSG_STANDARD, "Failed to install service %d %d 0x%x, rolling back", i, retval, retval);
            for (int rbi = i - 1; rbi >= 0; rbi--)
            {
                DWORD rbret = services[rbi].destroy(hScManager);
                if (rbret != 0)
                {
                    WcaLog(LOGMSG_STANDARD, "Failed to roll back service install %d 0x%x", rbret, rbret);
                }
            }
            break;
        }
    }
    WcaLog(LOGMSG_STANDARD, "done installing services");
    UINT er = EnableServiceForUser(sid, traceService);
    if (0 != er)
    {
        WcaLog(LOGMSG_STANDARD, "Warning, unable to enable trace service for dd user %d", er);
    }
    er = EnableServiceForUser(sid, processService);
    if (0 != er)
    {
        WcaLog(LOGMSG_STANDARD, "Warning, unable to enable process service for dd user %d", er);
    }
    er = EnableServiceForUser(sid, systemProbeService);
    if (0 != er)
    {
        WcaLog(LOGMSG_STANDARD, "Warning, unable to enable system probe service for dd user %d", er);
    }
    // need to enable user rights for the datadogagent service (main service)
    // so that it can restart itself
    er = EnableServiceForUser(sid, agentService);
    if (0 != er)
    {
        WcaLog(LOGMSG_STANDARD, "Warning, unable to enable agent service for dd user %d", er);
    }
    WcaLog(LOGMSG_STANDARD, "done setting service rights %d", retval);
    CloseServiceHandle(hScManager);
    return retval;
}
int uninstallServices()
{
    SC_HANDLE hScManager = NULL;
    SC_HANDLE hService = NULL;
    int retval = 0;
    // Get a handle to the SCM database.
#define NUM_SERVICES 4
    serviceDef services[NUM_SERVICES] = {
        serviceDef(agentService.c_str()),
        serviceDef(traceService.c_str()),
        serviceDef(processService.c_str()),
        serviceDef(systemProbeService.c_str()),
    };
    WcaLog(LOGMSG_STANDARD, "Uninstalling services");
    hScManager = OpenSCManager(NULL,                   // local computer
                               NULL,                   // ServicesActive database
                               SC_MANAGER_ALL_ACCESS); // full access rights

    if (NULL == hScManager)
    {
        WcaLog(LOGMSG_STANDARD, "OpenSCManager failed (%d)\n", GetLastError());
        return -1;
    }
    for (int i = NUM_SERVICES - 1; i >= 0; i--)
    {
        WcaLog(LOGMSG_STANDARD, "deleting service service %d", i);
        DWORD rbret = services[i].destroy(hScManager);
        if (rbret != 0)
        {
            auto lastErrStr = GetErrorMessageStrW(rbret);
            WcaLog(LOGMSG_STANDARD, "Failed to uninstall service %S (%d)", lastErrStr.c_str(), rbret);
        }
    }
    WcaLog(LOGMSG_STANDARD, "done uinstalling services");
    CloseServiceHandle(hScManager);
    return retval;
}
int verifyServices(CustomActionData &data)
{
    SC_HANDLE hScManager = NULL;
    SC_HANDLE hService = NULL;
    int retval = 0;
    // Get a handle to the SCM database.
#define NUM_SERVICES 4
#define SYSPROBE_INDEX 3
    serviceDef services[NUM_SERVICES] = {
        serviceDef(agentService.c_str(), L"Datadog Agent", L"Send metrics to Datadog", agent_exe.c_str(),
                   L"winmgmt\0\0", SERVICE_AUTO_START, data.FullyQualifiedUsername().c_str(), NULL),
        serviceDef(traceService.c_str(), L"Datadog Trace Agent", L"Send tracing metrics to Datadog", trace_exe.c_str(),
                   L"datadogagent\0\0", SERVICE_DEMAND_START, data.FullyQualifiedUsername().c_str(), NULL),
        serviceDef(processService.c_str(), L"Datadog Process Agent", L"Send process metrics to Datadog",
                   process_exe.c_str(), L"datadogagent\0\0", SERVICE_DEMAND_START, NULL, NULL),
        serviceDef(systemProbeService.c_str(), L"Datadog System Probe", L"Send network metrics to Datadog",
                   sysprobe_exe.c_str(), probeDepsNoNPM, SERVICE_DEMAND_START,
                   NULL, NULL)

    };
    int servicesToInstall = NUM_SERVICES;
    WcaLog(LOGMSG_STANDARD, "Installing services");
    hScManager = OpenSCManager(NULL,                   // local computer
                               NULL,                   // ServicesActive database
                               SC_MANAGER_ALL_ACCESS); // full access rights

    if (NULL == hScManager)
    {
        WcaLog(LOGMSG_STANDARD, "OpenSCManager failed (%d)\n", GetLastError());
        return -1;
    }
    for (int i = 0; i < servicesToInstall; i++)
    {
        WcaLog(LOGMSG_STANDARD, "updating service %d", i);
        retval = services[i].verify(hScManager);
        if (retval != 0)
        {
            if (ERROR_SERVICE_DOES_NOT_EXIST == retval && i > 1)
            {
                // i > 1 b/c we can't do this for core or trace, since they run as
                // ddagentuser and we don't have the password.  process & npm run
                // as local system, so there's no password to need.

                // since we're adding a new service later (npm), on upgrade we
                // must have the core agent.  Any of the subservices, if they're not
                // present, accept that (they might be newly added) and just try
                // to install it instead.

                // this only works b/c the NPM service is running as LOCAL_SYSTEM rather
                // than ddagentuser; otherwise, we wouldn't have the password at this
                // point and this wouldn't work.
                retval = services[i].create(hScManager);
                if (0 != retval)
                {
                    // if we can't create it, don't fail the upgrade,just log and
                    // continue on.  The existing services can/should still function
                    WcaLog(LOGMSG_STANDARD, "Failed to create new service during upgrade %S %d %d 0x%x",
                           services[i].getServiceName(), i, retval, retval);
                    WcaLog(LOGMSG_STANDARD, "Allowing upgrade to proceed");
                    // since we're allowing the upgrade to continue, reset the error code to zero
                    // in case this is the last one. Don't want to fail the upgrade by mistake
                    retval = 0;
                    continue;
                }
                // else

                // since we just created this service, we need to allow the datadog
                // agent core service to start/stop it
                retval = EnableServiceForUser(data.Sid(), services[i].getServiceName());
                if (0 != retval)
                {
                    WcaLog(LOGMSG_STANDARD, "Failed to modify service permissions for %S",
                           services[i].getServiceName());
                    // since we're allowing the upgrade to continue, reset the error code to zero
                    // in case this is the last one. Don't want to fail the upgrade by mistake
                    retval = 0;
                    continue;
                }
            }
            else
            {
                WcaLog(LOGMSG_STANDARD, "Failed to verify service %d %d 0x%x, rolling back", i, retval, retval);
                break;
            }
        }
    }

    WcaLog(LOGMSG_STANDARD, "done updating services");

    CloseServiceHandle(hScManager);
    return retval;
}
