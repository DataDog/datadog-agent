#include "stdafx.h"

extern "C" UINT __stdcall FinalizeInstall(MSIHANDLE hInstall) {
    HRESULT hr = S_OK;
    UINT er = ERROR_SUCCESS;
    int ddUserExists = 0;
    int ddServiceExists = 0;
    bool isDC = false;
    int passbuflen = 0;
    wchar_t *passbuf = NULL;
    const wchar_t * passToUse = NULL;
    CustomActionData data;
    std::wstring providedPassword;
    PSID sid = NULL;
    LSA_HANDLE hLsa = NULL;
    std::wstring propval;
    ddRegKey regkeybase;
    RegKey keyRollback, keyInstall;
    DWORD nErr = NERR_Success;
    bool bResetPassword = false;


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
    isDC = isDomainController(hInstall);

    // check to see if the supplied dd-agent-user exists
    WcaLog(LOGMSG_STANDARD, "checking to see if the user is already present");
    if ((ddUserExists = doesUserExist(hInstall, data, isDC)) == -1) {
        er = ERROR_INSTALL_FAILURE;
        goto LExit;
    }
    // check to see if the service is already installed
    WcaLog(LOGMSG_STANDARD, "checking to see if the service is installed");
    if ((ddServiceExists = doesServiceExist(hInstall, agentService)) == -1) {
        er = ERROR_INSTALL_FAILURE;
        goto LExit;
    }
    
    // now we have all the information we need to decide if this is a
    // new installation or an upgrade, and what steps need to be taken

    
    if(!canInstall(isDC, ddUserExists, ddServiceExists, data, bResetPassword)){
        er = ERROR_INSTALL_FAILURE;
        goto LExit;
    }
    
    // ok.  If we get here, we should be in a sane state (all installation conditions met)
    WcaLog(LOGMSG_STANDARD, "custom action initialization complete.  Processing");
    // first, let's decide if we need to create the dd-agent-user
    if (!ddUserExists || bResetPassword) {
        // that was easy.  Need to create the user.  See if we have a password, or need to
        // generate one
        passbuflen = MAX_PASS_LEN + 2;

        if (data.value(propertyDDAgentUserPassword, providedPassword)) {
            passToUse = providedPassword.c_str();
        }
        else {
            passbuf = new wchar_t[passbuflen];
            if (!generatePassword(passbuf, passbuflen)) {
                WcaLog(LOGMSG_STANDARD, "failed to generate password");
                er = ERROR_INSTALL_FAILURE;
                goto LExit;
            }
            passToUse = passbuf;
        }
        if (bResetPassword) {
            DWORD ret = doSetUserPassword(data.UnqualifiedUsername(), passToUse);
            if(ret != 0){
                WcaLog(LOGMSG_STANDARD, "Failed to set DD user password");
                er = ERROR_INSTALL_FAILURE;
                goto LExit;
            }
        } else {
            DWORD nErr = 0;
            DWORD ret = doCreateUser(data.UnqualifiedUsername(), ddAgentUserDescription, passToUse);
            if (ret != 0) {
                WcaLog(LOGMSG_STANDARD, "Failed to create DD user");
                er = ERROR_INSTALL_FAILURE;
                goto LExit;
            }
            // store that we created the user, and store the username so we can
            // delete on rollback/uninstall
            keyRollback.setStringValue(installCreatedDDUser.c_str(), data.Username().c_str());
            keyInstall.setStringValue(installCreatedDDUser.c_str(), data.Username().c_str());
            if (data.isUserDomainUser()) {
                keyRollback.setStringValue(installCreatedDDDomain.c_str(), data.Domain().c_str());
                keyInstall.setStringValue(installCreatedDDDomain.c_str(), data.Domain().c_str());
            }
        }
    }
    
    // add all the rights we want to the user (either existing or newly created)

    // set the account privileges regardless; if they're already set the OS will silently
    // ignore the request.    
    hr = -1;
    sid = GetSidForUser(NULL, data.Username().c_str());
    if (!sid) {
        WcaLog(LOGMSG_STANDARD, "Failed to get SID for %S", data.Username().c_str());
        goto LExit;
    }
    if ((hLsa = GetPolicyHandle()) == NULL) {
        WcaLog(LOGMSG_STANDARD, "Failed to get policy handle for %S", data.Username().c_str());
        goto LExit;
    }
    if (!AddPrivileges(sid, hLsa, SE_DENY_INTERACTIVE_LOGON_NAME)) {
        WcaLog(LOGMSG_STANDARD, "failed to add deny interactive login right");
        goto LExit;
    }

    if (!AddPrivileges(sid, hLsa, SE_DENY_NETWORK_LOGON_NAME)) {
        WcaLog(LOGMSG_STANDARD, "failed to add deny network login right");
        goto LExit;
    }
    if (!AddPrivileges(sid, hLsa, SE_DENY_REMOTE_INTERACTIVE_LOGON_NAME)) {
        WcaLog(LOGMSG_STANDARD, "failed to add deny remote interactive login right");
        goto LExit;
    }
    if (!AddPrivileges(sid, hLsa, SE_SERVICE_LOGON_NAME)) {
        WcaLog(LOGMSG_STANDARD, "failed to add service login right");
        goto LExit;
    }
    hr = 0;

    if(!ddUserExists)
    {
        hr = -1;
        nErr = AddUserToGroup(sid, L"S-1-5-32-558", L"Performance Monitor Users");
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
    }
    if (!ddServiceExists) {
        WcaLog(LOGMSG_STANDARD, "attempting to install services");
        if (!passToUse) {
            if (!data.value(propertyDDAgentUserPassword, providedPassword)) {
                // given all the error conditions checked above, this should *never*
                // happen.  But we'll check anyway
                WcaLog(LOGMSG_STANDARD, "Don't have password to register service");
                er = ERROR_INSTALL_FAILURE;
                goto LExit;
            }
            passToUse = providedPassword.c_str();
        }
        int ret = installServices(hInstall, data, passToUse);

        if (ret != 0) {
            WcaLog(LOGMSG_STANDARD, "Failed to create install services");
            er = ERROR_INSTALL_FAILURE;
            goto LExit;
        }
        keyRollback.setStringValue(installInstalledServices.c_str(), L"true");
        keyInstall.setStringValue(installInstalledServices.c_str(), L"true");

    } else {
        WcaLog(LOGMSG_STANDARD, "updating existing service record");
        int ret = verifyServices(hInstall, data);
        if (ret != 0) {
            WcaLog(LOGMSG_STANDARD, "Failed to updated existing services");
            er = ERROR_INSTALL_FAILURE;
            goto LExit;
        }
    }
    er = addDdUserPermsToFile(data, programdataroot);
    WcaLog(LOGMSG_STANDARD, "%d setting programdata dir perms", er);
    er = addDdUserPermsToFile(data, embedded2Dir);
    WcaLog(LOGMSG_STANDARD, "%d setting embedded2Dir dir perms", er);
    er = addDdUserPermsToFile(data, embedded3Dir);
    WcaLog(LOGMSG_STANDARD, "%d setting embedded3Dir dir perms", er);
    er = addDdUserPermsToFile(data, logfilename);
    WcaLog(LOGMSG_STANDARD, "%d setting log file perms", er);
    er = addDdUserPermsToFile(data, authtokenfilename);
    WcaLog(LOGMSG_STANDARD, "%d setting token file perms", er);
    er = addDdUserPermsToFile(data, datadogyamlfile);
    WcaLog(LOGMSG_STANDARD, "%d setting datadog.yaml file perms", er);
    er = addDdUserPermsToFile(data, confddir);
    WcaLog(LOGMSG_STANDARD, "%d setting confd dir perms", er);
    er = addDdUserPermsToFile(data, logdir);
    WcaLog(LOGMSG_STANDARD, "%d setting log dir perms", er);
    
    if (0 == changeRegistryAcls(data, datadog_acl_key_datadog.c_str())) {
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
    if (passbuf) {
        memset(passbuf, 0, sizeof(wchar_t) * passbuflen);
        delete[] passbuf;
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


    er = DoStartSvc(hInstall, agentService);
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
    DoStopSvc(hInstall, agentService);
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
    BOOL isDC = isDomainController(hInstall);
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
        uninstallServices(hInstall, data);
    }
    else {
        // this would have to be the rollback state, during an upgrade.
        // attempt to restart the services
        if (doesServiceExist(hInstall, agentService)) {
            // attempt to start it back up
            DoStartSvc(hInstall, agentService);
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


