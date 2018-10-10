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
extern "C" UINT __stdcall CreateOrUpdateDDUser(MSIHANDLE hInstall) 
{
    HRESULT hr = S_OK;
    UINT er = ERROR_SUCCESS;
    LSA_HANDLE hLsa = NULL;
    PSID sid = NULL;
    DWORD nErr = 0;
    LOCALGROUP_MEMBERS_INFO_0 lmi0;
    memset(&lmi0, 0, sizeof(LOCALGROUP_MEMBERS_INFO_3));

    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    hr = WcaInitialize(hInstall, "CA: CreateOrUpdateDDUser");
    ExitOnFailure(hr, "Failed to initialize");
    logProcCount();
    WcaLog(LOGMSG_STANDARD, "Initialized.");

    er = CreateDDUser(hInstall);
    if (0 != er) {
        hr = -1;
        goto LExit;
    } 
    // if the log file or the auth token already exist, allow the dd-user to 
    // access them

    
    er = addDdUserPermsToFile(logfilename);
    WcaLog(LOGMSG_STANDARD, "%d setting log file perms",er);
    er = addDdUserPermsToFile(authtokenfilename);
    WcaLog(LOGMSG_STANDARD, "%d setting token file perms",er);
    er = addDdUserPermsToFile(datadogyamlfile);
    WcaLog(LOGMSG_STANDARD, "%d setting token file perms",er);
    er = addDdUserPermsToFile(confddir);
    WcaLog(LOGMSG_STANDARD, "%d setting confd dir perms",er);
<<<<<<< HEAD

=======
    MarkInstallStepComplete(strFilePermissionsChanged);
>>>>>>> db/dd-agent-user-65-omnibus
// change the rights on this user
    hr = -1;
    sid = GetSidForUser(NULL, (LPCWSTR)ddAgentUserName.c_str());
    if (!sid) {
        goto LExit;
    }
    if ((hLsa = GetPolicyHandle()) == NULL) {
        goto LExit;
    }

#ifndef _NO_DD_USER_RIGHTS_REMOVAL
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
#endif
    if (!AddPrivileges(sid, hLsa, SE_SERVICE_LOGON_NAME)) {
        WcaLog(LOGMSG_STANDARD, "failed to add service login right");
        goto LExit;
    }
    // add the user to the "performance monitor users" group
    lmi0.lgrmi0_sid = sid;
    nErr = NetLocalGroupAddMembers(NULL, L"Performance Monitor Users", 0, (LPBYTE)&lmi0, 1);
    if(nErr == NERR_Success) {
        WcaLog(LOGMSG_STANDARD, "Added ddagentuser to Performance Monitor Users");

    } else if (nErr == ERROR_MEMBER_IN_GROUP || nErr == ERROR_MEMBER_IN_ALIAS ) {
        WcaLog(LOGMSG_STANDARD, "User already in group, continuing %d", nErr);
    } else {
        WcaLog(LOGMSG_STANDARD, "Unexpected error adding user to group %d", nErr);
        goto LExit;
    }
    hr = 0;
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

extern "C" UINT __stdcall EnableServicesForDDUser(MSIHANDLE hInstall) 
{
    HRESULT hr = S_OK;
    UINT er = ERROR_SUCCESS;

    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    hr = WcaInitialize(hInstall, "CA: EnableServicesForDDUser");
    ExitOnFailure(hr, "Failed to initialize");
    logProcCount();
    WcaLog(LOGMSG_STANDARD, "Initialized.");

    er = EnableServiceForUser(traceService, ddAgentUserName);
    if (0 != er) {
        hr = -1;
        goto LExit;
    } 
    er = EnableServiceForUser(processService, ddAgentUserName);
    if (0 != er) {
        hr = -1;
        goto LExit;
    }
    // need to enable user rights for the datadogagent service (main service)
    // so that it can restart itself
    er = EnableServiceForUser(agentService, ddAgentUserName);
    if (0 != er) {
        hr = -1;
        goto LExit;
    }


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


extern "C" UINT __stdcall VerifyDatadogRegistryPerms(MSIHANDLE hInstall) {
    HRESULT hr = S_OK;
    UINT er = ERROR_SUCCESS;

    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    hr = WcaInitialize(hInstall, "CA: VerifyDDRegPerms");
    ExitOnFailure(hr, "Failed to initialize");
    logProcCount();
    WcaLog(LOGMSG_STANDARD, "Initialized.");
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

extern "C" UINT __stdcall PreStopServices(MSIHANDLE hInstall) {
    HRESULT hr = S_OK;
    UINT er = ERROR_SUCCESS;

    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    hr = WcaInitialize(hInstall, "CA: PreStopServices");
    ExitOnFailure(hr, "Failed to initialize");
    logProcCount();
    WcaLog(LOGMSG_STANDARD, "Initialized.");

    DoStopSvc(hInstall, datadog_service_name);
    WcaLog(LOGMSG_STANDARD, "Waiting for prestop to complete");
    Sleep(10000);
    WcaLog(LOGMSG_STANDARD, "Prestop complete");
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

