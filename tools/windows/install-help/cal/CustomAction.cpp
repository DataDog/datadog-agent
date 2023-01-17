#include "stdafx.h"

extern "C" UINT __stdcall FinalizeInstall(MSIHANDLE hInstall)
{

    HRESULT hr = S_OK;
    UINT er = ERROR_SUCCESS;
    std::optional<CustomActionData> data;
    hr = WcaInitialize(hInstall, "CA: FinalizeInstall");
    ExitOnFailure(hr, "Failed to initialize");
    WcaLog(LOGMSG_STANDARD, "Initialized.");

#ifdef _DEBUG
    MessageBox(NULL, L"hi", L"bye", MB_OK);
#endif
    // first, get the necessary initialization data
    // need the dd-agent-username (if provided)
    // need the dd-agent-password (if provided)
    try
    {
        auto propertyView = std::make_shared<DeferredCAPropertyView>(hInstall);
        data.emplace(propertyView);
    }
    catch (std::exception &)
    {
        WcaLog(LOGMSG_STANDARD, "Failed to load custom action property data");
        er = ERROR_INSTALL_FAILURE;
        goto LExit;
    }

    er = doFinalizeInstall(data.value());

LExit:
    if (er == ERROR_SUCCESS)
    {
        er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    }
    return WcaFinalize(er);
}

extern "C" UINT __stdcall PreStopServices(MSIHANDLE hInstall)
{
    HRESULT hr = S_OK;
    UINT er = ERROR_SUCCESS;

    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    hr = WcaInitialize(hInstall, "CA: PreStopServices");
    ExitOnFailure(hr, "Failed to initialize");

    WcaLog(LOGMSG_STANDARD, "Initialized.");
    DoStopAllServices();
    WcaLog(LOGMSG_STANDARD, "Waiting for prestop to complete");
    Sleep(10000);
    WcaLog(LOGMSG_STANDARD, "Prestop complete");
LExit:
    er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    return WcaFinalize(er);
}

extern "C" UINT __stdcall PostStartServices(MSIHANDLE hInstall)
{
    HRESULT hr = S_OK;
    DWORD er = ERROR_SUCCESS;

    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    hr = WcaInitialize(hInstall, "CA: PostStartServices");
    ExitOnFailure(hr, "Failed to initialize");

    WcaLog(LOGMSG_STANDARD, "Initialized.");
#ifdef _DEBUG
    MessageBox(NULL, L"PostStartServices", L"PostStartServices", MB_OK);
#endif

    er = DoStartSvc(agentService);
    WcaLog(LOGMSG_STANDARD, "Waiting for start to complete");
    Sleep(5000);
    WcaLog(LOGMSG_STANDARD, "start complete");
    if (ERROR_SUCCESS != er)
    {
        hr = -1;
    }
LExit:
    er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    return WcaFinalize(er);
}

extern "C" UINT __stdcall DoUninstall(MSIHANDLE hInstall)
{
    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    HRESULT hr = WcaInitialize(hInstall, "CA: DoUninstall");
    UINT er = ERROR_SUCCESS;
    ExitOnFailure(hr, "Failed to initialize");

    WcaLog(LOGMSG_STANDARD, "Initialized.");
    initializeStringsFromStringTable();
    er = doUninstallAs(UNINSTALL_UNINSTALL);
    if (er != 0)
    {
        hr = -1;
    }
LExit:
    er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    return WcaFinalize(er);
}

extern "C" UINT __stdcall DoRollback(MSIHANDLE hInstall)
{
    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    HRESULT hr = WcaInitialize(hInstall, "CA: DoRollback");
    UINT er = ERROR_SUCCESS;
    ExitOnFailure(hr, "Failed to initialize");

    WcaLog(LOGMSG_STANDARD, "Initialized.");

#ifdef _DEBUG
    MessageBoxA(NULL, "DoRollback", "DoRollback", MB_OK);
#endif
    WcaLog(LOGMSG_STANDARD, "Giving services a chance to settle...");
    Sleep(10000);
    WcaLog(LOGMSG_STANDARD, "Proceeding with rollback");
    initializeStringsFromStringTable();
    // we'll need to stop the services manually if we got far enough to start
    // them before installation failed.
    DoStopAllServices();
    er = doUninstallAs(UNINSTALL_ROLLBACK);
    if (er != 0)
    {
        hr = -1;
    }
LExit:
    er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    return WcaFinalize(er);
}

HMODULE hDllModule;

// DllMain - Initialize and cleanup WiX custom action utils.
extern "C" BOOL WINAPI DllMain(__in HINSTANCE hInst, __in ULONG ulReason, __in LPVOID)
{
    switch (ulReason)
    {
    case DLL_PROCESS_ATTACH:
        WcaGlobalInitialize(hInst);
        // initialize random number generator
        hDllModule = (HMODULE)hInst;
        initializeStringsFromStringTable();
        break;

    case DLL_PROCESS_DETACH:
        WcaGlobalFinalize();
        break;
    }

    return TRUE;
}

/*
 * Immediate custom action executed at the DDAgentUserDlg dialog
 *
 * Checks the provided username and password with the system
 * state to ensure install should not fail.
 *
 */
extern "C" UINT __stdcall ValidateDDAgentUserDlgInput(MSIHANDLE hInstall)
{
    std::optional<CustomActionData> data;
    bool shouldResetPassword = false;
    std::wstring resultMessage;

    HRESULT hr = WcaInitialize(hInstall, "CA: " __FUNCTION__);
    if (FAILED(hr))
    {
        return WcaFinalize(ERROR_INSTALL_FAILURE);
    }
    WcaLog(LOGMSG_STANDARD, "Initialized.");

    try
    {
        auto propertyView = std::make_shared<ImmediateCAPropertyView>(hInstall);
        data.emplace(propertyView);
    }
    catch (std::exception &)
    {
        WcaLog(LOGMSG_STANDARD, "Failed to load installer property data");
        return WcaFinalize(ERROR_INSTALL_FAILURE);
    }

    if (!canInstall(data.value(), shouldResetPassword, &resultMessage))
    {
        MsiSetProperty(hInstall, L"DDAgentUser_Valid", L"False");
        MsiSetProperty(hInstall, L"DDAgentUser_ResultMessage", resultMessage.c_str());
        // Not an error. Must return success for the installer to continue
    }
    else
    {
        MsiSetProperty(hInstall, L"DDAgentUser_Valid", L"True");
        MsiSetProperty(hInstall, L"DDAgentUser_ResultMessage", L"");
    }

    return WcaFinalize(ERROR_SUCCESS);
}
