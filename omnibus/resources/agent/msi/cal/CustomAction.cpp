#include "stdafx.h"
int changeServiceConfig(wchar_t *svcName);
extern "C" UINT __stdcall FinalizeInstall(MSIHANDLE hInstall) {
    HRESULT hr = S_OK;
    UINT er = ERROR_SUCCESS;
    CustomActionData data;
    PSID sid = NULL;
    std::wstring propval;
    ddRegKey regkeybase;
    RegKey keyRollback, keyInstall;

    std::wstring waitval;

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
    regkeybase.deleteSubKey(strRollbackKeyName.c_str());
    regkeybase.createSubKey(strRollbackKeyName.c_str(), keyRollback, REG_OPTION_VOLATILE);
    regkeybase.createSubKey(strUninstallKeyName.c_str(), keyInstall);

    // check to see if we're a domain controller.
    WcaLog(LOGMSG_STANDARD, "checking if this is a domain controller");

    
    // ok.  If we get here, we should be in a sane state (all installation conditions met)
    WcaLog(LOGMSG_STANDARD, "custom action initialization complete.  Processing");
    // first, let's decide if we need to create the dd-agent-user
    bool isDC = isDomainController(hInstall);
    if(!isDC)
    {
        sid = GetSidForUser(NULL, L"NT Service\\datadogagent");
        int nErr = AddUserToGroup(sid, L"S-1-5-32-558", L"Performance Monitor Users");
        if (nErr != NERR_Success) {
            WcaLog(LOGMSG_STANDARD, "Unexpected error adding user to group %d", nErr);
            goto LExit;
        }
        nErr = AddUserToGroup(sid, L"S-1-5-32-573", L"Event Log Readers");
        if (nErr != NERR_Success) {
            WcaLog(LOGMSG_STANDARD, "Unexpected error adding user to group %d", nErr);
            goto LExit;
        }
        hr = 0;
    } else {
        WcaLog(LOGMSG_STANDARD, "Machine is a domain controller.  Not adding to groups");
        WcaLog(LOGMSG_STANDARD, "For Event Logs, manually change permissions");
    }

    WcaLog(LOGMSG_STANDARD, "updating service configs");
    er = SetServiceSIDUnrestricted(L"datadogagent");
    if(er == 0){
        WcaLog(LOGMSG_STANDARD, "Changed config for agent");
    } else {
        WcaLog(LOGMSG_STANDARD, "Failed to change config for agent %d", er);
    }
    er = SetServiceSIDUnrestricted(L"datadog-trace-agent");
    if(er == 0){
        WcaLog(LOGMSG_STANDARD, "Changed config for trace agent");
    } else {
        WcaLog(LOGMSG_STANDARD, "Failed to change config for traceagent %d", er);
    }
    const wchar_t* users[] = {
        L"NT Service\\datadogagent",
        L"NT Service\\datadog-trace-agent",
        NULL
    };
    er = addDdUserPermsToFile(users, programdataroot);
    WcaLog(LOGMSG_STANDARD, "%d setting programdata dir perms", er);
    er = addDdUserPermsToFile(users, installdir);
    WcaLog(LOGMSG_STANDARD, "%d setting installdir dir perms", er);

    er = addDdUserPermsToFile(users, logfilename);
    WcaLog(LOGMSG_STANDARD, "%d setting log file perms", er);
    er = addDdUserPermsToFile(users, authtokenfilename);
    WcaLog(LOGMSG_STANDARD, "%d setting token file perms", er);
    er = addDdUserPermsToFile(users, datadogyamlfile);
    WcaLog(LOGMSG_STANDARD, "%d setting datadog.yaml file perms", er);
    er = addDdUserPermsToFile(users, confddir);
    WcaLog(LOGMSG_STANDARD, "%d setting confd dir perms", er);
    er = addDdUserPermsToFile(users, logdir);
    WcaLog(LOGMSG_STANDARD, "%d setting log dir perms", er);

    er = EnableServiceForUser(users[0], traceService);
    if (0 != er) {
        WcaLog(LOGMSG_STANDARD, "Warning, unable to enable trace service for dd user %d", er);
    }
    er = EnableServiceForUser(users[0], processService);
    if (0 != er) {
        WcaLog(LOGMSG_STANDARD, "Warning, unable to enable process service for dd user %d", er);
    }
    er = EnableServiceForUser(users[0], agentService);
    if (0 != er) {
        WcaLog(LOGMSG_STANDARD, "Warning, unable to enable agent service for dd user %d", er);
    }

    if (0 == changeRegistryAcls(users, datadog_acl_key_datadog.c_str())) {
        WcaLog(LOGMSG_STANDARD, "registry perms updated");
    }
    else {
        WcaLog(LOGMSG_STANDARD, "registry perm update failed");
        er = ERROR_INSTALL_FAILURE;
    }
    {
        // attempt to add the symlink.  We're not going to fail in case of failure,
        // so we can ignore return ocde if it's already there.
        std::wstring embedded = installdir + L"\\embedded";
        std::wstring bindir = installdir + L"\\bin";
        BOOL bRet = CreateSymbolicLink(embedded.c_str(), bindir.c_str(), SYMBOLIC_LINK_FLAG_DIRECTORY);
        WcaLog(LOGMSG_STANDARD, "CreateSymbolicLink %d %d", bRet, GetLastError());
    }
LExit:
    if (sid) {
        delete[](BYTE *) sid;
    }
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
    DoStopSvc(hInstall, agentService);
    WcaLog(LOGMSG_STANDARD, "Waiting for prestop to complete");
    Sleep(10000);
    WcaLog(LOGMSG_STANDARD, "Prestop complete");
LExit:
    er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    return WcaFinalize(er);

}


#if 0
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
#endif

extern "C" UINT __stdcall DoRollback(MSIHANDLE hInstall) {
    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    HRESULT hr = WcaInitialize(hInstall, "CA: DoRollback");
    UINT er = 0;
    ExitOnFailure(hr, "Failed to initialize");
    initializeStringsFromStringTable();
    WcaLog(LOGMSG_STANDARD, "Initialized.");

#ifdef _DEBUG
    MessageBoxA(NULL, "DoRollback", "DoRollback", MB_OK);
#endif
    // just clean any pyc files that were created
    {
        std::wstring dir_to_delete;
        dir_to_delete = installdir + L"bin";
        DeleteFilesInDirectory(dir_to_delete.c_str(), L"*.pyc");
        dir_to_delete = installdir + L"embedded2";
        DeleteFilesInDirectory(dir_to_delete.c_str(), L"*.pyc");
        dir_to_delete = installdir + L"embedded3";
        DeleteFilesInDirectory(dir_to_delete.c_str(), L"*.pyc");

        // remove the symlink
        std::wstring embedded = installdir + L"\\embedded";
        RemoveDirectory(embedded.c_str());

        // delete the auth token filename altogether
        DeleteFile(authtokenfilename.c_str());
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


