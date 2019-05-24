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

    ///////////////////////////////////////////////////////////////////////////
    //
    // If domain controller:
    //   If user is present:
    //     if service is present:
    //        this is an upgrade.
    //     if service is not present
    //        this is new install on this machine
    //        dd user has already been created in domain
    //        must have password for registering service
    //   If user is NOT present
    //     if service is present
    //       ERROR how could service be present but user not present?
    //     if service is not present
    //       new install in this domain
    //       must have password for user creation and service installation
    //
    // If NOT a domain controller
    //   if user is present
    //     if the service is present
    //       this is an upgrade, shouldn't need to do anything for user/service
    //     if the service is not present
    //       ERROR why is user created but not service?
    //   if the user is NOT present
    //     if the service is present
    //       This is OK if it's a domain user
    //     if the service is not present
    //       install service, create user
    //       use password if provided, otherwise generate

    if (isDC) {
        if (!ddUserExists && ddServiceExists) {
            WcaLog(LOGMSG_STANDARD, "Invalid configuration; no DD user, but service exists");
            er = ERROR_INSTALL_FAILURE;
            goto LExit;
        }
        if (!ddUserExists || !ddServiceExists) {
            if (!data.present(propertyDDAgentUserPassword)) {
                WcaLog(LOGMSG_STANDARD, "Must supply password for dd-agent-user to create user and/or install service in a domain");
                er = ERROR_INSTALL_FAILURE;
                goto LExit;
            }
        }
    }
    else {
        if (ddUserExists)
        {
            if (data.getDomainPtr() != NULL) {
                // if it's a domain user. We need the password if the service isn't here
                if (!ddServiceExists && !data.present(propertyDDAgentUserPassword))
                {
                    WcaLog(LOGMSG_STANDARD, "Must supply the password to allow service registration");
                    er = ERROR_INSTALL_FAILURE;
                    goto LExit;
                }
            }
            else {
                if (!ddServiceExists) {
                    WcaLog(LOGMSG_STANDARD, "Invalid configuration; DD user exists, but no service exists");
                    er = ERROR_INSTALL_FAILURE;
                    goto LExit;
                }
            }
        }
        if (!ddUserExists && ddServiceExists) {
            WcaLog(LOGMSG_STANDARD, "Invalid configuration; no DD user, but service exists");
            er = ERROR_INSTALL_FAILURE;
            goto LExit;
        }
    }
    // ok.  If we get here, we should be in a sane state (all installation conditions met)
    WcaLog(LOGMSG_STANDARD, "custom action initialization complete.  Processing");
    // first, let's decide if we need to create the dd-agent-user
    if (!ddUserExists) {
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
        DWORD nErr = 0;
        DWORD ret = doCreateUser(data.getUsername(), data.getDomainPtr(), ddAgentUserDescription, passToUse);
        if (ret != 0) {
            WcaLog(LOGMSG_STANDARD, "Failed to create DD user");
            er = ERROR_INSTALL_FAILURE;
            goto LExit;
        }
        // store that we created the user, and store the username so we can
        // delete on rollback/uninstall
        keyRollback.setStringValue(installCreatedDDUser.c_str(), data.getUserPtr());
        keyInstall.setStringValue(installCreatedDDUser.c_str(), data.getUserPtr());
        if (data.getDomainPtr()) {
            keyRollback.setStringValue(installCreatedDDDomain.c_str(), data.getDomainPtr());
            keyInstall.setStringValue(installCreatedDDDomain.c_str(), data.getDomainPtr());
        }
    }
    if(!ddUserExists || !ddServiceExists)
    {
        LOCALGROUP_MEMBERS_INFO_0 lmi0;
        memset(&lmi0, 0, sizeof(LOCALGROUP_MEMBERS_INFO_0));
        DWORD nErr  = 0;
        std::wstring groupname;
        // since we just created the user, fix up all the rights we want
        hr = -1;
        sid = GetSidForUser(NULL, data.getQualifiedUsername().c_str());
        if (!sid) {
            WcaLog(LOGMSG_STANDARD, "Failed to get SID for %s", data.getFullUsernameMbcs().c_str());
            goto LExit;
        }
        if ((hLsa = GetPolicyHandle()) == NULL) {
            WcaLog(LOGMSG_STANDARD, "Failed to get policy handle for %s", data.getFullUsernameMbcs().c_str());
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
        // add the user to the "performance monitor users" group
        lmi0.lgrmi0_sid = sid;
        // need to look up the group name by SID; the group name can be localized
        {
            PSID groupsid = NULL;
            if(!ConvertStringSidToSid(L"S-1-5-32-558", &groupsid)) {
                WcaLog(LOGMSG_STANDARD, "failed to convert sid string to sid; attempting default");
                groupname = L"Performance Monitor Users";
            } else {
                if(!GetNameForSid(NULL, groupsid, groupname)) {
                    WcaLog(LOGMSG_STANDARD, "failed to get group name for sid; using default");
                    groupname = L"Performance Monitor Users";
                }
                LocalFree((LPVOID) groupsid);
            }
        }
        nErr = NetLocalGroupAddMembers(NULL, groupname.c_str(), 0, (LPBYTE)&lmi0, 1);
        if (nErr == NERR_Success) {
            WcaLog(LOGMSG_STANDARD, "Added ddagentuser to Performance Monitor Users");
        }
        else if (nErr == ERROR_MEMBER_IN_GROUP || nErr == ERROR_MEMBER_IN_ALIAS) {
            WcaLog(LOGMSG_STANDARD, "User already in group, continuing %d", nErr);
        }
        else {
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
    er = addDdUserPermsToFile(data, installdir);
    WcaLog(LOGMSG_STANDARD, "%d setting installdir dir perms", er);

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

BOOL DeleteDirectory(const TCHAR* sPath);
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
        dir_to_delete = programdataroot + L"bin";
        DeleteDirectory(dir_to_delete.c_str());
        dir_to_delete = programdataroot + L"embedded2";
        DeleteDirectory(dir_to_delete.c_str());
        dir_to_delete = programdataroot + L"embedded3";
        DeleteDirectory(dir_to_delete.c_str());
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
    if (t == UNINSTALL_UNINSTALL) {
        regkey.createSubKey(strUninstallKeyName.c_str(), installState);
    }
    else {
        regkey.createSubKey(strRollbackKeyName.c_str(), installState);
    }
    // check to see if we created the user, and if so, what the user's name was
    std::wstring installedUser, installedDomain, installedComplete;
    const wchar_t* installedUserPtr = NULL;
    const wchar_t* installedDomainPtr = NULL;
    if (installState.getStringValue(installCreatedDDUser.c_str(), installedUser))
    {
        std::string usershort;
        toMbcs(usershort, installedUser.c_str());
        WcaLog(LOGMSG_STANDARD, "This install installed user %s, will remove", usershort.c_str());
        installedUserPtr = installedUser.c_str();
        if (installState.getStringValue(installCreatedDDDomain.c_str(), installedDomain)) {
            installedDomainPtr = installedDomain.c_str();
            toMbcs(usershort, installedDomainPtr);
            WcaLog(LOGMSG_STANDARD, "Removing user from domain %s", usershort);
            installedComplete = installedDomain + L"\\";
        }
        installedComplete += installedUser;
        willDeleteUser = true;
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
        lmi0.lgrmi0_sid = sid;
        nErr = NetLocalGroupDelMembers(NULL, L"Performance Monitor Users", 0, (LPBYTE)&lmi0, 1);
        if (nErr == NERR_Success) {
            WcaLog(LOGMSG_STANDARD, "removed ddagentuser from Performance Monitor Users");
        }
        else if (nErr == ERROR_NO_SUCH_MEMBER || nErr == ERROR_MEMBER_NOT_IN_ALIAS) {
            WcaLog(LOGMSG_STANDARD, "User wasn't in group, continuing %d", nErr);
        }
        else {
            WcaLog(LOGMSG_STANDARD, "Unexpected error removing user from group %d", nErr);
        }

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

    if (sid) {
        delete[](BYTE *) sid;
    }
    if (hLsa) {
        LsaClose(hLsa);
    }
    return er;

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


