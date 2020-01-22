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
typedef enum _uninstall_type {
    UNINSTALL_UNINSTALL,
    UNINSTALL_ROLLBACK
} UNINSTALL_TYPE;

UINT doUninstallAs(MSIHANDLE hInstall, UNINSTALL_TYPE t);
extern "C" UINT __stdcall DoUninstall(MSIHANDLE hInstall) {
    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    HRESULT hr = WcaInitialize(hInstall, "CA: DoUninstall");
    UINT er = 0;
    ExitOnFailure(hr, "Failed to initialize");
     
    WcaLog(LOGMSG_STANDARD, "Initialized.");
    initializeStringsFromStringTable();
    er = doUninstallAs(hInstall, UNINSTALL_UNINSTALL);
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
    er = doUninstallAs(hInstall, UNINSTALL_ROLLBACK);
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
UINT doUninstallAs(MSIHANDLE hInstall, UNINSTALL_TYPE t)
{

    DWORD er = ERROR_SUCCESS;
    CustomActionData data;
    PSID sid = NULL;
    LSA_HANDLE hLsa = NULL;
    std::wstring propval;
    ddRegKey regkey;
    RegKey installState;
    std::wstring waitval;
    DWORD nErr = 0;
    LOCALGROUP_MEMBERS_INFO_0 lmi0;
    memset(&lmi0, 0, sizeof(LOCALGROUP_MEMBERS_INFO_0));
    BOOL willDeleteUser = false;
    BOOL isDC = isDomainController();
    if (t == UNINSTALL_UNINSTALL) {
        regkey.createSubKey(strUninstallKeyName.c_str(), installState);
    }
    else {
        regkey.createSubKey(strRollbackKeyName.c_str(), installState);
    }
    // check to see if we created the user, and if so, what the user's name was
    std::wstring installedUser, installedDomain, installedComplete;
    if (installState.getStringValue(installCreatedDDUser.c_str(), installedUser))
    {
        WcaLog(LOGMSG_STANDARD, "This install installed user %S", installedUser.c_str());
        size_t ndx;
        if((ndx = installedUser.find(L'\\')) != std::string::npos)
        {
            installedUser = installedUser.substr(ndx + 1);
        }
        // username is now stored fully qualified (<domain>\<user>).  However, removal
        // code expects the unqualified name. Split it out here.
        if (installState.getStringValue(installCreatedDDDomain.c_str(), installedDomain)) {
            WcaLog(LOGMSG_STANDARD, "NOT Removing user from domain %S", installedDomain.c_str());
            WcaLog(LOGMSG_STANDARD, "Domain user can be removed.");
            installedComplete = installedDomain + L"\\";
        } else if (isDC) {
            WcaLog(LOGMSG_STANDARD, "NOT Removing user %S from domain controller", installedUser.c_str());
            WcaLog(LOGMSG_STANDARD, "Domain user can be removed.");
    
        } else {
            WcaLog(LOGMSG_STANDARD, "Will delete user %S from local user store", installedUser.c_str());
            willDeleteUser = true;
        }
        installedComplete += installedUser;
        
    }

    if (willDeleteUser)
    {
        sid = GetSidForUser(NULL, installedComplete.c_str());

        // remove dd user from programdata root
        removeUserPermsFromFile(programdataroot, sid);

        // remove dd user from log directory
        removeUserPermsFromFile(logdir, sid);

        // remove dd user from conf directory
        removeUserPermsFromFile(confddir, sid);

        // remove dd user from datadog.yaml
        removeUserPermsFromFile(datadogyamlfile, sid);

        // remove dd user from Performance monitor users
        DelUserFromGroup(sid, L"S-1-5-32-558", L"Performance Monitor Users");
        DelUserFromGroup(sid, L"S-1-5-32-573", L"Event Log Readers");

        // remove dd user right for
        //   SE_SERVICE_LOGON NAME
        //   SE_DENY_REMOVE_INTERACTIVE_LOGON_NAME
        //   SE_DENY_NETWORK_LOGIN_NAME
        //   SE_DENY_INTERACTIVE_LOGIN_NAME
        if ((hLsa = GetPolicyHandle()) != NULL) {
            if (!RemovePrivileges(sid, hLsa, SE_DENY_INTERACTIVE_LOGON_NAME)) {
                WcaLog(LOGMSG_STANDARD, "failed to remove deny interactive login right");
            }

            if (!RemovePrivileges(sid, hLsa, SE_DENY_NETWORK_LOGON_NAME)) {
                WcaLog(LOGMSG_STANDARD, "failed to remove deny network login right");
            }
            if (!RemovePrivileges(sid, hLsa, SE_DENY_REMOTE_INTERACTIVE_LOGON_NAME)) {
                WcaLog(LOGMSG_STANDARD, "failed to remove deny remote interactive login right");
            }
            if (!RemovePrivileges(sid, hLsa, SE_SERVICE_LOGON_NAME)) {
                WcaLog(LOGMSG_STANDARD, "failed to remove service login right");
            }
        }
        // delete the user
        er = DeleteUser(NULL, installedUser.c_str());
        if (0 != er) {
            // don't actually fail on failure.  We're doing an uninstall,
            // and failing will just leave the system in a more confused state
            WcaLog(LOGMSG_STANDARD, "Didn't delete the datadog user %d", er);
        }
    }
    // remove the auth token file altogether

    DeleteFile(authtokenfilename.c_str());
    std::wstring svcsInstalled;
    if (installState.getStringValue(installInstalledServices.c_str(), svcsInstalled))
    {
        // uninstall the services
        uninstallServices(data);
    }
    else {
        // this would have to be the rollback state, during an upgrade.
        // attempt to restart the services
        if (doesServiceExist(agentService)) {
            // attempt to start it back up
            DoStartSvc(agentService);
        }
    }
    std::wstring embedded = installdir + L"\\embedded";
    RemoveDirectory(embedded.c_str());

    if (sid) {
        delete[](BYTE *) sid;
    }
    if (hLsa) {
        LsaClose(hLsa);
    }
    return 0;

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


