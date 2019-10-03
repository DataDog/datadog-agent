#include "stdafx.h"

//! doUninstallAs
/*
 \param hInstall  handle to the currently running MSI
 \param UNINSTALL_TYPE  type of uninstall being performed (rollback or uninstall)
 \return 0 on success, or specific error code.
 */

UINT doDDUninstallAs(MSIHANDLE hInstall, UNINSTALL_TYPE t)
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
            er = 0;
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
    return er;

}