#include "stdafx.h"
#include "PropertyReplacer.h"
#include "TargetMachine.h"
#include <fstream>

bool updateYamlConfig(CustomActionData &customActionData)
{
    std::wstring inputConfig;

    // Read config in memory. The config should be small enough
    // and we control its source - so it's fine to allocate up front.
    {
        std::wifstream inputConfigStream(datadogyamlfile);

        inputConfigStream.seekg(0, std::ios::end);
        size_t fileSize = inputConfigStream.tellg();
        if (fileSize <= 0)
        {
            WcaLog(LOGMSG_STANDARD, "ERROR: datadog.yaml file empty !");
            return false;
        }
        inputConfig.reserve(fileSize);
        inputConfigStream.seekg(0, std::ios::beg);

        inputConfig.assign(std::istreambuf_iterator<wchar_t>(inputConfigStream), std::istreambuf_iterator<wchar_t>());
    }

    // If we find an API key entry in the yaml file, don't do anything
    std::wregex re(L"^api_key:(.*)");
    std::match_results<std::wstring::const_iterator> results;
    if (std::regex_search(inputConfig, results, re))
    {
        auto api_key = results[1].str();
        api_key.erase(api_key.begin(),
                      std::find_if(api_key.begin(), api_key.end(), [](int ch) { return !std::isspace(ch); }));
        if (api_key.length() > 0)
        {
            WcaLog(LOGMSG_STANDARD, "API key already present in configuration - not modifying it");
            return true;
        }
    }

    std::vector<std::wstring> failedToReplace;
    inputConfig =
        replace_yaml_properties(inputConfig, [&customActionData](std::wstring const &propertyName) -> std::optional<std::wstring> {
            std::wstring propertyValue;
            if (customActionData.value(propertyName, propertyValue))
            {
                return propertyValue;
            }
            return std::nullopt;
        },
        &failedToReplace);

    for (auto v : failedToReplace)
    {
        WcaLog(LOGMSG_STANDARD, "Failed to replace %S in datadog.yaml file", v.c_str());
    }

    {
        std::wofstream inputConfigStream(datadogyamlfile);
        inputConfigStream << inputConfig;
    }
    return true;
}


UINT doFinalizeInstall(CustomActionData &data)
{
    HRESULT hr = S_OK;
    UINT er = ERROR_SUCCESS;

    bool ddUserExists = false;
    int ddServiceExists = 0;
    int passbuflen = 0;
    wchar_t *passbuf = NULL;
    const wchar_t *passToUse = NULL;
    std::wstring providedPassword;
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

    // check to see if the service is already installed
    WcaLog(LOGMSG_STANDARD, "checking to see if the service is installed");
    if ((ddServiceExists = doesServiceExist(agentService)) == -1)
    {
        er = ERROR_INSTALL_FAILURE;
        goto LExit;
    }

    // now we have all the information we need to decide if this is a
    // new installation or an upgrade, and what steps need to be taken
    ddUserExists = data.DoesUserExist();

    if (!canInstall(data.GetTargetMachine()->IsDomainController(), ddUserExists, ddServiceExists, data, bResetPassword))
    {
        er = ERROR_INSTALL_FAILURE;
        goto LExit;
    }

    // ok.  If we get here, we should be in a sane state (all installation conditions met)
    WcaLog(LOGMSG_STANDARD, "custom action initialization complete.  Processing");
    // first, let's decide if we need to create the dd-agent-user
    if (!ddUserExists || bResetPassword)
    {
        // that was easy.  Need to create the user.  See if we have a password, or need to
        // generate one
        passbuflen = MAX_PASS_LEN + 2;

        if (data.value(propertyDDAgentUserPassword, providedPassword))
        {
            passToUse = providedPassword.c_str();
        }
        else
        {
            passbuf = new wchar_t[passbuflen];
            if (!generatePassword(passbuf, passbuflen))
            {
                WcaLog(LOGMSG_STANDARD, "failed to generate password");
                er = ERROR_INSTALL_FAILURE;
                goto LExit;
            }
            passToUse = passbuf;
        }
        if (bResetPassword)
        {
            DWORD ret = doSetUserPassword(data.UnqualifiedUsername(), passToUse);
            if (ret != 0)
            {
                WcaLog(LOGMSG_STANDARD, "Failed to set DD user password");
                er = ERROR_INSTALL_FAILURE;
                goto LExit;
            }
        }
        else
        {
            DWORD ret = doCreateUser(data.UnqualifiedUsername(), ddAgentUserDescription, passToUse);
            if (ret != 0)
            {
                WcaLog(LOGMSG_STANDARD, "Failed to create DD user");
                er = ERROR_INSTALL_FAILURE;
                goto LExit;
            }

            auto sidResult = GetSidForUser(nullptr, data.Username().c_str());
            if (sidResult.Result != ERROR_SUCCESS)
            {
                WcaLog(LOGMSG_STANDARD, "Failed to lookup account name: %d", GetLastError());
                er = ERROR_INSTALL_FAILURE;
                goto LExit;
            }
            data.Sid(sidResult.Sid);

            // store that we created the user, and store the username so we can
            // delete on rollback/uninstall
            keyRollback.setStringValue(installCreatedDDUser.c_str(), data.Username().c_str());
            keyInstall.setStringValue(installCreatedDDUser.c_str(), data.Username().c_str());
            if (data.isUserDomainUser())
            {
                keyRollback.setStringValue(installCreatedDDDomain.c_str(), data.Domain().c_str());
                keyInstall.setStringValue(installCreatedDDDomain.c_str(), data.Domain().c_str());
            }
        }
    }

    // add all the rights we want to the user (either existing or newly created)
    // set the account privileges regardless; if they're already set the OS will silently
    // ignore the request.
    hr = -1;
    if ((hLsa = GetPolicyHandle()) == NULL)
    {
        WcaLog(LOGMSG_STANDARD, "Failed to get policy handle for %S", data.Username().c_str());
        goto LExit;
    }
    if (!AddPrivileges(data.Sid(), hLsa, SE_DENY_INTERACTIVE_LOGON_NAME))
    {
        WcaLog(LOGMSG_STANDARD, "failed to add deny interactive login right");
        goto LExit;
    }

    if (!AddPrivileges(data.Sid(), hLsa, SE_DENY_NETWORK_LOGON_NAME))
    {
        WcaLog(LOGMSG_STANDARD, "failed to add deny network login right");
        goto LExit;
    }
    if (!AddPrivileges(data.Sid(), hLsa, SE_DENY_REMOTE_INTERACTIVE_LOGON_NAME))
    {
        WcaLog(LOGMSG_STANDARD, "failed to add deny remote interactive login right");
        goto LExit;
    }
    if (!AddPrivileges(data.Sid(), hLsa, SE_SERVICE_LOGON_NAME))
    {
        WcaLog(LOGMSG_STANDARD, "failed to add service login right");
        goto LExit;
    }
    hr = 0;

    if (!data.GetTargetMachine()->IsReadOnlyDomainController())
    {
        er = AddUserToGroup(data.Sid(), L"S-1-5-32-558", L"Performance Monitor Users");
        if (er != NERR_Success)
        {
            WcaLog(LOGMSG_STANDARD, "Unexpected error adding user to group %d", er);
            goto LExit;
        }
        er = AddUserToGroup(data.Sid(), L"S-1-5-32-573", L"Event Log Readers");
        if (er != NERR_Success)
        {
            WcaLog(LOGMSG_STANDARD, "Unexpected error adding user to group %d", er);
            goto LExit;
        }
    }

    if (!ddServiceExists)
    {
        WcaLog(LOGMSG_STANDARD, "attempting to install services");
        if (!passToUse)
        {
            if (!data.value(propertyDDAgentUserPassword, providedPassword))
            {
                // given all the error conditions checked above, this should *never*
                // happen.  But we'll check anyway
                WcaLog(LOGMSG_STANDARD, "Don't have password to register service");
                er = ERROR_INSTALL_FAILURE;
                goto LExit;
            }
            passToUse = providedPassword.c_str();
        }
        int ret = installServices(data, data.Sid(), passToUse);

        if (ret != 0)
        {
            WcaLog(LOGMSG_STANDARD, "Failed to create install services");
            er = ERROR_INSTALL_FAILURE;
            goto LExit;
        }
        keyRollback.setStringValue(installInstalledServices.c_str(), L"true");
        keyInstall.setStringValue(installInstalledServices.c_str(), L"true");
    }
    else
    {
        WcaLog(LOGMSG_STANDARD, "updating existing service record");
        int ret = verifyServices(data);
        if (ret != 0)
        {
            WcaLog(LOGMSG_STANDARD, "Failed to updated existing services");
            er = ERROR_INSTALL_FAILURE;
            goto LExit;
        }
    }

    if (!updateYamlConfig(data))
    {
        WcaLog(LOGMSG_STANDARD, "Failed to update datadog.yaml");
        er = ERROR_INSTALL_FAILURE;
        goto LExit;
    }

    er = addDdUserPermsToFile(data.Sid(), programdataroot);
    WcaLog(LOGMSG_STANDARD, "%d setting programdata dir perms", er);
    er = addDdUserPermsToFile(data.Sid(), embedded2Dir);
    WcaLog(LOGMSG_STANDARD, "%d setting embedded2Dir dir perms", er);
    er = addDdUserPermsToFile(data.Sid(), embedded3Dir);
    WcaLog(LOGMSG_STANDARD, "%d setting embedded3Dir dir perms", er);
    er = addDdUserPermsToFile(data.Sid(), logfilename);
    WcaLog(LOGMSG_STANDARD, "%d setting log file perms", er);
    er = addDdUserPermsToFile(data.Sid(), authtokenfilename);
    WcaLog(LOGMSG_STANDARD, "%d setting token file perms", er);
    er = addDdUserPermsToFile(data.Sid(), datadogyamlfile);
    WcaLog(LOGMSG_STANDARD, "%d setting datadog.yaml file perms", er);
    er = addDdUserPermsToFile(data.Sid(), confddir);
    WcaLog(LOGMSG_STANDARD, "%d setting confd dir perms", er);
    er = addDdUserPermsToFile(data.Sid(), logdir);
    WcaLog(LOGMSG_STANDARD, "%d setting log dir perms", er);

    if (0 == changeRegistryAcls(data.Sid(), datadog_acl_key_datadog.c_str()))
    {
        WcaLog(LOGMSG_STANDARD, "registry perms updated");
    }
    else
    {
        WcaLog(LOGMSG_STANDARD, "registry perm update failed");
        er = ERROR_INSTALL_FAILURE;
    }
    {
        // attempt to add the symlink.  We're not going to fail in case of failure,
        // so we can ignore return ocde if it's already there.
        std::wstring embedded = installdir + L"\\embedded";
        std::wstring bindir = installdir + L"\\bin";
        BOOL bRet = CreateSymbolicLink(embedded.c_str(), bindir.c_str(), SYMBOLIC_LINK_FLAG_DIRECTORY);
        if (!bRet)
        {
            DWORD lastErr = GetLastError();
            std::string lastErrStr = GetErrorMessageStr(lastErr);
            WcaLog(LOGMSG_STANDARD, "CreateSymbolicLink: %s (%d)", lastErrStr.c_str(), lastErr);
        }
        else
        {
            WcaLog(LOGMSG_STANDARD, "CreateSymbolicLink");
        }
    }

    // write out the username & domain we used.  Even write it out if we didn't create it,
    // it's needed on xDCs where we may not have created the user -and- is necessary on upgrade
    // from previous install that didn't write this key
    regkeybase.setStringValue(keyInstalledUser.c_str(), data.UnqualifiedUsername().c_str());
    regkeybase.setStringValue(keyInstalledDomain.c_str(), data.Domain().c_str());

LExit:
    if (passbuf)
    {
        memset(passbuf, 0, sizeof(wchar_t) * passbuflen);
        delete[] passbuf;
    }
    if (er == ERROR_SUCCESS)
    {
        er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    }
    return er;
}
