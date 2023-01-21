#include "stdafx.h"
#include "PropertyReplacer.h"
#include "TargetMachine.h"
#ifndef _CONSOLE
// install-cmd and uninstall-cmd projects are console projects.
// skip the decompressing part for those testing projects.
#include "decompress.h"
#endif
#include <array>
#include <filesystem>
#include <fstream>

bool ShouldUpdateConfig()
{
    std::wifstream inputConfigStream(datadogyamlfile);
    if (!inputConfigStream.is_open())
    {
        WcaLog(LOGMSG_STANDARD, "datadog.yaml cannot be opened - trying to update it");
        return true;
    }

    inputConfigStream.seekg(0, std::ios::end);
    size_t fileSize = inputConfigStream.tellg();
    if (fileSize <= 0)
    {
        WcaLog(LOGMSG_STANDARD, "datadog.yaml is empty - updating");
        return true;
    }
    WcaLog(LOGMSG_STANDARD, "datadog.yaml exists and is not empty - not modifying it");
    return false;
}

bool updateYamlConfig(CustomActionData &customActionData)
{
    // check if datadog.yaml file needs to be updated.
    if (!ShouldUpdateConfig())
    {
        return true;
    }

    // Read example config in memory.
    std::wifstream inputConfigExampleStream(datadogyamlfile + L".example");
    if (!inputConfigExampleStream.is_open())
    {
        WcaLog(LOGMSG_STANDARD, "ERROR: datadog.yaml.example cannot be opened !");
        return false;
    }
    inputConfigExampleStream.seekg(0, std::ios::end);
    size_t fileSize = inputConfigExampleStream.tellg();
    if (fileSize <= 0)
    {
        WcaLog(LOGMSG_STANDARD, "ERROR: datadog.yaml.example is empty !");
        return true;
    }

    std::wstring inputConfig;
    inputConfig.reserve(fileSize);
    inputConfigExampleStream.seekg(0, std::ios::beg);
    inputConfig.assign(std::istreambuf_iterator<wchar_t>(inputConfigExampleStream),
                       std::istreambuf_iterator<wchar_t>());

    std::vector<std::wstring> failedToReplace;
    inputConfig = replace_yaml_properties(
        inputConfig,
        [&customActionData](std::wstring const &propertyName) -> std::optional<std::wstring> {
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

    std::wofstream outputConfigStream(datadogyamlfile);
    outputConfigStream << inputConfig;
    return true;
}

std::optional<std::wstring> GetInstallMethod(const CustomActionData &customActionData)
{
    std::wstring customInstallMethod;
    customActionData.value(L"OVERRIDE_INSTALLATION_METHOD", customInstallMethod);

    if (customInstallMethod.empty())
    {
        WcaLog(LOGMSG_VERBOSE, "No override installation method specified, computing using UILevel");

        std::wstring uiLevelStr;
        customActionData.value(L"UILevel", uiLevelStr);

        std::wstringstream uiLevelStrStream(uiLevelStr);
        int uiLevel = -1;
        uiLevelStrStream >> uiLevel;
        if (uiLevelStrStream.fail())
        {
            WcaLog(LOGMSG_STANDARD, "Could not read UILevel from installer: %S", uiLevelStr.c_str());
            return std::nullopt;
        }

        // 2 = quiet
        // > 2 (typically 5) = UI
        if (uiLevel > 2)
        {
            customInstallMethod = L"windows_msi_gui";
        }
        else
        {
            customInstallMethod = L"windows_msi_quiet";
        }
    }
    return std::optional<std::wstring>(customInstallMethod);
}

bool writeInstallInfo(const CustomActionData &customActionData)
{
    std::optional<std::wstring> installMethod = GetInstallMethod(customActionData);
    if (installMethod)
    {
        WcaLog(LOGMSG_VERBOSE, "Install method: %S", installMethod.value().c_str());
        std::wofstream installInfoOutputStream(installInfoFile);
        installInfoOutputStream << L"---" << std::endl
                                << L"install_method:" << std::endl
                                << L"  tool: " << installMethod.value() << std::endl
                                << L"  tool_version: " << installMethod.value() << std::endl
                                << L"  installer_version: " << installMethod.value() << std::endl;
        return true;
    }

    // Prefer logging error in GetInstallMethod to avoid double logging.
    return false;
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

    if (!canInstall(data, bResetPassword, NULL))
    {
        er = ERROR_INSTALL_FAILURE;
        goto LExit;
    }

    if (data.value(propertyDDAgentUserPassword, providedPassword))
    {
        passToUse = providedPassword.c_str();
    }

    // ok.  If we get here, we should be in a sane state (all installation conditions met)
    WcaLog(LOGMSG_STANDARD, "custom action initialization complete.  Processing");
    // first, let's decide if we need to create the dd-agent-user
    if (!ddUserExists || bResetPassword)
    {
        // that was easy.  Need to create the user.  See if we have a password, or need to
        // generate one
        passbuflen = MAX_PASS_LEN + 2;

        if (!data.value(propertyDDAgentUserPassword, providedPassword))
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

            auto sidResult = GetSidForUser(nullptr, data.FullyQualifiedUsername().c_str());
            if (sidResult.Result != ERROR_SUCCESS)
            {
                WcaLog(LOGMSG_STANDARD, "Failed to lookup account name: %d", GetLastError());
                er = ERROR_INSTALL_FAILURE;
                goto LExit;
            }
            data.Sid(sidResult.Sid);

            // store that we created the user, and store the username so we can
            // delete on rollback/uninstall
            keyRollback.setStringValue(installCreatedDDUser.c_str(), data.FullyQualifiedUsername().c_str());
            keyInstall.setStringValue(installCreatedDDUser.c_str(), data.FullyQualifiedUsername().c_str());
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
        WcaLog(LOGMSG_STANDARD, "Failed to get policy handle for %S", data.FullyQualifiedUsername().c_str());
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

    if (!writeInstallInfo(data))
    {
        WcaLog(LOGMSG_STANDARD, "Failed to update install_info");
        er = ERROR_INSTALL_FAILURE;
        goto LExit;
    }

#ifndef _CONSOLE
    {
        std::array<std::filesystem::path, 2> embeddedArchiveLocations = {
            installdir + L"\\embedded2.7z",
            installdir + L"\\embedded3.7z",
        };

        for (const auto path : embeddedArchiveLocations)
        {
            if (std::filesystem::exists(path))
            {
                WcaLog(LOGMSG_STANDARD, "Found archive %s, decompressing", path.string().c_str());
                if (decompress_archive(path, installdir) != 0)
                {
                    WcaLog(LOGMSG_STANDARD, "Failed to decompress archive %s", path.string().c_str());
                    er = ERROR_INSTALL_FAILURE;
                    goto LExit;
                }
                else
                {
                    // Delete the archive
                    std::filesystem::remove(path);
                }
            }
        }
    }
#endif

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
            auto lastErrStr = GetErrorMessageStrW(lastErr);
            WcaLog(LOGMSG_STANDARD, "CreateSymbolicLink: %S (%d)", lastErrStr.c_str(), lastErr);
        }
        else
        {
            WcaLog(LOGMSG_STANDARD, "CreateSymbolicLink");
        }
    }
    // write out the open source config information
    data.setClosedSourceConfig();
    
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
