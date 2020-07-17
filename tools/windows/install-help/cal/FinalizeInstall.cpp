#include "stdafx.h"

UINT doFinalizeInstall(CustomActionData &data)
{
    HRESULT hr = S_OK;
    UINT er = ERROR_SUCCESS;

    int ddUserExists = 0;
    int ddServiceExists = 0;
    bool isDC = false;
    int passbuflen = 0;
    wchar_t *passbuf = NULL;
    const wchar_t * passToUse = NULL;
    
    std::wstring providedPassword;
    PSID sid = NULL;
    LSA_HANDLE hLsa = NULL;
    std::wstring propval;
    ddRegKey regkeybase;
    RegKey keyRollback, keyInstall;
    DWORD nErr = NERR_Success;
    bool bResetPassword = false;


    std::wstring waitval;
    regkeybase.deleteSubKey(strRollbackKeyName.c_str());
    regkeybase.createSubKey(strRollbackKeyName.c_str(), keyRollback, REG_OPTION_VOLATILE);
    regkeybase.createSubKey(strUninstallKeyName.c_str(), keyInstall);

    // check to see if we're a domain controller.
    WcaLog(LOGMSG_STANDARD, "checking if this is a domain controller");
    isDC = isDomainController();

    // check to see if the supplied dd-agent-user exists
    WcaLog(LOGMSG_STANDARD, "checking to see if the user is already present");
    if ((ddUserExists = doesUserExist(data, isDC)) == -1) {
        er = ERROR_INSTALL_FAILURE;
        goto LExit;
    }
    // check to see if the service is already installed
    WcaLog(LOGMSG_STANDARD, "checking to see if the service is installed");
    if ((ddServiceExists = doesServiceExist(agentService)) == -1) {
        er = ERROR_INSTALL_FAILURE;
        goto LExit;
    }

    // now we have all the information we need to decide if this is a
    // new installation or an upgrade, and what steps need to be taken


    if (!canInstall(isDC, ddUserExists, ddServiceExists, data, bResetPassword)) {
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
            if (ret != 0) {
                WcaLog(LOGMSG_STANDARD, "Failed to set DD user password");
                er = ERROR_INSTALL_FAILURE;
                goto LExit;
            }
        }
        else {
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

    if (!ddUserExists)
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
        int ret = installServices(data, passToUse);

        if (ret != 0) {
            WcaLog(LOGMSG_STANDARD, "Failed to create install services");
            er = ERROR_INSTALL_FAILURE;
            goto LExit;
        }
        keyRollback.setStringValue(installInstalledServices.c_str(), L"true");
        keyInstall.setStringValue(installInstalledServices.c_str(), L"true");

    }
    else {
        WcaLog(LOGMSG_STANDARD, "updating existing service record");
        int ret = verifyServices(data);
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
    return er;
}
