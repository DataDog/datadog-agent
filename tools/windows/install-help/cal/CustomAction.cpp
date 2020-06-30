#include "stdafx.h"

extern "C" UINT __stdcall FinalizeInstall(MSIHANDLE hInstall) {
    
    HRESULT hr = S_OK;
    UINT er = ERROR_SUCCESS;
    CustomActionData data;
    // first, get the necessary initialization data
    // need the dd-agent-username (if provided)
    // need the dd-agent-password (if provided)
    hr = WcaInitialize(hInstall, "CA: FinalizeInstall");
    ExitOnFailure(hr, "Failed to initialize");
    WcaLog(LOGMSG_STANDARD, "Initialized.");

#ifdef _DEBUG
    MessageBox(NULL, L"hi", L"bye", MB_OK);
#endif
    if(!data.init(hInstall)){
        WcaLog(LOGMSG_STANDARD, "Failed to load custom action property data");
        er = ERROR_INSTALL_FAILURE;
        goto LExit;
    }
    er =  doFinalizeInstall(data);

LExit:
    if (er == ERROR_SUCCESS) {
        er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    }
    return WcaFinalize(er);

}




extern "C" UINT __stdcall PreStopServices(MSIHANDLE hInstall) {
    HRESULT hr = S_OK;
    UINT er = ERROR_SUCCESS;

    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    hr = WcaInitialize(hInstall, "CA: PreStopServices");
    ExitOnFailure(hr, "Failed to initialize");
    
    WcaLog(LOGMSG_STANDARD, "Initialized.");
    DoStopSvc(agentService);
    WcaLog(LOGMSG_STANDARD, "Waiting for prestop to complete");
    Sleep(10000);
    WcaLog(LOGMSG_STANDARD, "Prestop complete");
LExit:
    er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    return WcaFinalize(er);

}

extern "C" UINT __stdcall PostStartServices(MSIHANDLE hInstall) {
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
    if (ERROR_SUCCESS != er) {
        hr = -1;
    }
LExit:
    er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    return WcaFinalize(er);

}


extern "C" UINT __stdcall DoUninstall(MSIHANDLE hInstall) {
    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    HRESULT hr = WcaInitialize(hInstall, "CA: DoUninstall");
    UINT er = 0;
    ExitOnFailure(hr, "Failed to initialize");
     
    WcaLog(LOGMSG_STANDARD, "Initialized.");
    initializeStringsFromStringTable();
    er = doUninstallAs(UNINSTALL_UNINSTALL);
    if (er != 0) {
        hr = -1;
    }
LExit:
    er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    return WcaFinalize(er);
}


extern "C" UINT __stdcall DoRollback(MSIHANDLE hInstall) {
    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    HRESULT hr = WcaInitialize(hInstall, "CA: DoRollback");
    UINT er = 0;
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
    DoStopSvc(agentService);
    er = doUninstallAs(UNINSTALL_ROLLBACK);
    if (er != 0) {
        hr = -1;
    }
    {
        std::wstring dir_to_delete;
        dir_to_delete = installdir + L"bin";
        DeleteFilesInDirectory(dir_to_delete.c_str(), L"*.pyc");
        dir_to_delete = installdir + L"embedded2";
        DeleteFilesInDirectory(dir_to_delete.c_str(), L"*.pyc");
        // python 3, on startup, leaves a bunch of __pycache__ directories,
        // so we have to be more aggressive.
        dir_to_delete = installdir + L"embedded3";
        DeleteFilesInDirectory(dir_to_delete.c_str(), L"*.pyc");
        DeleteFilesInDirectory(dir_to_delete.c_str(), L"__pycache__", true);
    }
LExit:
    er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    return WcaFinalize(er);
}

HMODULE hDllModule;

// DllMain - Initialize and cleanup WiX custom action utils.
extern "C" BOOL WINAPI DllMain(
    __in HINSTANCE hInst,
    __in ULONG ulReason,
    __in LPVOID
    )
{
    switch(ulReason)
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


