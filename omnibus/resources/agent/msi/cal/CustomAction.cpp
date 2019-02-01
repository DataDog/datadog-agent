#include "stdafx.h"


int CreateUser(std::wstring& name, std::wstring& comment, bool writePassToReg = false);
DWORD DeleteUser(std::wstring& name);
DWORD DeleteSecretsRegKey();

/* define _NO_SECRET_USER_RIGHTS_REMOVAL to test with the datadog_secretuser
   retaining interactive, network, remote login rights
   */
//#define _NO_SECRET_USER_RIGHTS_REMOVAL

/* define _NO_DD_USER_RIGHTS_REMOVAL to test with the ddagentuser
   retaining interactive, network, remote login rights
   */
//#define _NO_DD_USER_RIGHTS_REMOVAL

static int proccount = 0;

void logProcCount() {
    WcaLog(LOGMSG_STANDARD, "ProcCount %d", ++proccount);
}

#ifdef CA_CMD_TEST
#define LOGMSG_STANDARD 0
void WcaLog(int type, const char * fmt...)
{
    va_list args;
    va_start(args, fmt);
    vprintf(fmt, args);
    va_end(args);
    printf("\n");
}
#else

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
    ddRegKey regkey;
    std::wstring waitval;

    // first, get the necessary initialization data
    // need the dd-agent-username (if provided)
    // need the dd-agent-password (if provided)
    hr = WcaInitialize(hInstall, "CA: FinalizeInstall");
    ExitOnFailure(hr, "Failed to initialize");
    WcaLog(LOGMSG_STANDARD, "Initialized.");
    if(!data.init(hInstall)){
        WcaLog(LOGMSG_STANDARD, "Failed to load custom action property data");
        er = ERROR_INSTALL_FAILURE;
        goto LExit;
    }

    // check to see if the supplied dd-agent-user exists
    if ((ddUserExists = doesUserExist(hInstall, data)) == -1) {
        er = ERROR_INSTALL_FAILURE;
        goto LExit;
    }
    // check to see if the service is already installed
    if ((ddServiceExists = doesServiceExist(hInstall, agentService)) == -1) {
        er = ERROR_INSTALL_FAILURE;
        goto LExit;
    }
    // check to see if we're a domain controller.
    isDC = isDomainController(hInstall);

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
    //       ERROR how could service be present but not user?
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
        if (ddUserExists && !ddServiceExists) {
            WcaLog(LOGMSG_STANDARD, "Invalid configuration; DD user exists, but no service exists");
            er = ERROR_INSTALL_FAILURE;
            goto LExit;
        }
        if (!ddUserExists && ddServiceExists) {
            WcaLog(LOGMSG_STANDARD, "Invalid configuration; no DD user, but service exists");
            er = ERROR_INSTALL_FAILURE;
            goto LExit;
        }
    }
    // ok.  If we get here, we should be in a sane state (all installation conditions met)

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
        LOCALGROUP_MEMBERS_INFO_0 lmi0;
        memset(&lmi0, 0, sizeof(LOCALGROUP_MEMBERS_INFO_0));
        DWORD nErr = 0;
        DWORD ret = doCreateUser(data.getUsername(), data.getDomainPtr(), ddAgentUserDescription, passToUse);
        if (ret != 0) {
            WcaLog(LOGMSG_STANDARD, "Failed to create DD user");
            er = ERROR_INSTALL_FAILURE;
            goto LExit;
        }
        // since we just created the user, fix up all the rights we want
        hr = -1;
        sid = GetSidForUser(data.getDomainPtr(), data.getUserPtr());
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
        nErr = NetLocalGroupAddMembers(NULL, L"Performance Monitor Users", 0, (LPBYTE)&lmi0, 1);
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
    }
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
    er = addDdUserPermsToFile(data, programdataroot);
    WcaLog(LOGMSG_STANDARD, "%d setting programdata dir perms", er);

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
extern "C" UINT __stdcall RemoveDDUser(MSIHANDLE hInstall)
{
    HRESULT hr = S_OK;
    UINT er = ERROR_SUCCESS;
    PSID sid = NULL;
    LSA_HANDLE hLsa = NULL;

    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    hr = WcaInitialize(hInstall, "CA: DeleteDDUser");
    ExitOnFailure(hr, "Failed to initialize");
    logProcCount();
    WcaLog(LOGMSG_STANDARD, "Initialized.");
    // change the rights on this user
    sid = GetSidForUser(NULL, (LPCWSTR)ddAgentUserName.c_str());
    if (!sid) {
        goto LExit;
    }
    if ((hLsa = GetPolicyHandle()) == NULL) {
        goto LExit;
    }

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

    er = doRemoveDDUser();
    
LExit:
    if (sid) {
        delete[](BYTE *) sid;
    }
    if (hLsa) {
        LsaClose(hLsa);
    }
    er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    return WcaFinalize(er);


}


#if 0

extern "C" UINT __stdcall VerifyDatadogRegistryPerms(MSIHANDLE hInstall) {
    HRESULT hr = S_OK;
    UINT er = ERROR_SUCCESS;

    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    hr = WcaInitialize(hInstall, "CA: VerifyDDRegPerms");
    ExitOnFailure(hr, "Failed to initialize");
    logProcCount();
    WcaLog(LOGMSG_STANDARD, "Initialized.");
    // get the ddagent user name
    if(!loadDdAgentUserName(hInstall))
    {
        WcaLog(LOGMSG_STANDARD, "DDAGENT username not supplied, using default");
    }
    // make sure the key is there
    LSTATUS status = 0;
    HKEY hKey;
    status = RegCreateKeyExW(HKEY_LOCAL_MACHINE,
        datadog_key_root.c_str(),
        0, // reserved is zero
        NULL, // class is null
        0, // no options
        KEY_ALL_ACCESS,
        NULL, // default security descriptor (we'll change this later)
        &hKey,
        NULL); // don't care about disposition... 
    if (ERROR_SUCCESS != status) {
        WcaLog(LOGMSG_STANDARD, "Couldn't create/open datadog reg key %d", GetLastError());
        hr = -1;
        goto LExit;
    }
    RegCloseKey(hKey);
    
    WcaLog(LOGMSG_STANDARD, "Reg key created, setting perms");
    if(0 == changeRegistryAcls(datadog_acl_key_datadog.c_str())) {
        WcaLog(LOGMSG_STANDARD, "registry perms updated");
        hr = S_OK;
        MarkInstallStepComplete(strChangedRegistryPermissions);
    } else {
        WcaLog(LOGMSG_STANDARD, "registry perm update failed");
        hr = -1;
    }


LExit:
    er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    return WcaFinalize(er);

}
#endif
extern "C" UINT __stdcall PreStopServices(MSIHANDLE hInstall) {
    HRESULT hr = S_OK;
    UINT er = ERROR_SUCCESS;

    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    hr = WcaInitialize(hInstall, "CA: PreStopServices");
    ExitOnFailure(hr, "Failed to initialize");
    logProcCount();
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
    logProcCount();
    WcaLog(LOGMSG_STANDARD, "Initialized.");

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

extern "C" UINT __stdcall DoUninstall(MSIHANDLE hInstall) {
    HRESULT hr = S_OK;
    DWORD er = ERROR_SUCCESS;

    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    hr = WcaInitialize(hInstall, "CA: DoUninstall");
    ExitOnFailure(hr, "Failed to initialize");
    logProcCount();
    WcaLog(LOGMSG_STANDARD, "Initialized.");

LExit:
    er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    return WcaFinalize(er);

}


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
        break;

    case DLL_PROCESS_DETACH:
        WcaGlobalFinalize();
        break;
    }

    return TRUE;
}
#endif // CA_CMD_TEST

