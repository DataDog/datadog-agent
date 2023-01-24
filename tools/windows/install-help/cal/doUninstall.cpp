// ReSharper disable CommentTypo
// ReSharper disable IdentifierTypo
#include "stdafx.h"
#include "TargetMachine.h"
#include <array>

extern std::wstring versionhistoryfilename;

void cleanupFolders()
{
    std::error_code ec;
    const std::filesystem::path installPath(installdir);

    // embedded is a symlink to bin created by us in the customaction, remove it here
    std::filesystem::remove(installPath / "embedded", ec);
    if (ec)
    {
        WcaLog(LOGMSG_STANDARD, "Could not remove embedded folder: %s", ec.message().c_str());
    }

    // Nuke the embedded2/3 folders since we don't support patching the Python installation
    // and it's not tracked by the MSI installer anymore
    for (const auto &folder : {"embedded2", "embedded3"})
    {
        std::filesystem::path folderPath = installPath / folder;
        if (!exists(folderPath))
        {
            continue;
        }

        // Ensure no file is read-only
        // Bug: We have to do this because of a bug in Microsoft's STL remove_all function
        // See https://github.com/microsoft/STL/issues/1511
        // It has been fixed starting with VS 2022 17.0 Preview 3
        for (const auto &direntry : std::filesystem::recursive_directory_iterator(folderPath))
        {
            // Remove read-only if set
            permissions(direntry, std::filesystem::perms::all, std::filesystem::perm_options::replace, ec);
            if (ec)
            {
                WcaLog(LOGMSG_STANDARD, "Could not update permissions for %s: %s", direntry.path().c_str(),
                       ec.message().c_str());
            }
        }

        remove_all(folderPath, ec);
        if (ec)
        {
            WcaLog(LOGMSG_STANDARD, "Could not delete folder %S: %s", folderPath.c_str(), ec.message().c_str());
        }
    }

    // Remove the installdir only if it's empty
    std::filesystem::remove(installPath, ec);
    if (ec)
    {
        WcaLog(LOGMSG_STANDARD, "Could not delete folder %S: %s", installPath.c_str(), ec.message().c_str());
    }
}

UINT doUninstallAs(UNINSTALL_TYPE t)
{

    DWORD er = ERROR_SUCCESS;
    LSA_HANDLE hLsa = NULL;
    std::wstring propval;
    ddRegKey regkey;
    RegKey installState;
    std::wstring waitval;
    DWORD nErr = 0;
    LOCALGROUP_MEMBERS_INFO_0 lmi0;
    memset(&lmi0, 0, sizeof(LOCALGROUP_MEMBERS_INFO_0));
    BOOL willDeleteUser = false;
    TargetMachine machine;

    if (t == UNINSTALL_UNINSTALL)
    {
        regkey.createSubKey(strUninstallKeyName.c_str(), installState);
        //
        // Make best effort to delete versionhistory.json file on uninstallation. We only attempt
        // to delete the file from the default location. If customer changed the default location,
        // the file will not be deleted.
        //
        (void)DeleteFileW(versionhistoryfilename.c_str());
    }
    else
    {
        regkey.createSubKey(strRollbackKeyName.c_str(), installState);
    }
    // check to see if we created the user, and if so, what the user's name was
    std::wstring installedUser, installedDomain, installedComplete;
    if (installState.getStringValue(installCreatedDDUser.c_str(), installedUser))
    {
        WcaLog(LOGMSG_STANDARD, "This install installed user %S", installedUser.c_str());
        size_t ndx;
        if ((ndx = installedUser.find(L'\\')) != std::string::npos)
        {
            installedUser = installedUser.substr(ndx + 1);
        }
        // username is now stored fully qualified (<domain>\<user>).  However, removal
        // code expects the unqualified name. Split it out here.
        if (installState.getStringValue(installCreatedDDDomain.c_str(), installedDomain))
        {
            WcaLog(LOGMSG_STANDARD, "NOT Removing user from domain %S", installedDomain.c_str());
            WcaLog(LOGMSG_STANDARD, "Domain user can be removed.");
            installedComplete = installedDomain + L"\\";
        }
        else if (machine.IsDomainController())
        {
            WcaLog(LOGMSG_STANDARD, "NOT Removing user %S from domain controller", installedUser.c_str());
            WcaLog(LOGMSG_STANDARD, "Domain user can be removed.");
        }
        else
        {
            WcaLog(LOGMSG_STANDARD, "Will delete user %S from local user store", installedUser.c_str());
            willDeleteUser = true;
        }
        installedComplete += installedUser;
    }

    if (willDeleteUser)
    {
        auto sidResult = GetSidForUser(nullptr, installedComplete.c_str());

        // Do not try to do anything if we don't find the user.
        if (sidResult.Result == ERROR_SUCCESS)
        {
            // remove dd user from programdata root
            removeUserPermsFromFile(programdataroot, sidResult.Sid.get());

            // remove dd user from log directory
            removeUserPermsFromFile(logdir, sidResult.Sid.get());

            // remove dd user from conf directory
            removeUserPermsFromFile(confddir, sidResult.Sid.get());

            // remove dd user from datadog.yaml
            removeUserPermsFromFile(datadogyamlfile, sidResult.Sid.get());

            // remove dd user from Performance monitor users
            DelUserFromGroup(sidResult.Sid.get(), L"S-1-5-32-558", L"Performance Monitor Users");
            DelUserFromGroup(sidResult.Sid.get(), L"S-1-5-32-573", L"Event Log Readers");

            // remove dd user right for
            //   SE_SERVICE_LOGON NAME
            //   SE_DENY_REMOVE_INTERACTIVE_LOGON_NAME
            //   SE_DENY_NETWORK_LOGIN_NAME
            //   SE_DENY_INTERACTIVE_LOGIN_NAME
            if ((hLsa = GetPolicyHandle()) != NULL)
            {
                if (!RemovePrivileges(sidResult.Sid.get(), hLsa, SE_DENY_INTERACTIVE_LOGON_NAME))
                {
                    WcaLog(LOGMSG_STANDARD, "failed to remove deny interactive login right");
                }

                if (!RemovePrivileges(sidResult.Sid.get(), hLsa, SE_DENY_NETWORK_LOGON_NAME))
                {
                    WcaLog(LOGMSG_STANDARD, "failed to remove deny network login right");
                }
                if (!RemovePrivileges(sidResult.Sid.get(), hLsa, SE_DENY_REMOTE_INTERACTIVE_LOGON_NAME))
                {
                    WcaLog(LOGMSG_STANDARD, "failed to remove deny remote interactive login right");
                }
                if (!RemovePrivileges(sidResult.Sid.get(), hLsa, SE_SERVICE_LOGON_NAME))
                {
                    WcaLog(LOGMSG_STANDARD, "failed to remove service login right");
                }
            }
            // delete the user
            er = DeleteUser(NULL, installedUser.c_str());
            if (0 != er)
            {
                // don't actually fail on failure.  We're doing an uninstall,
                // and failing will just leave the system in a more confused state
                WcaLog(LOGMSG_STANDARD, "Didn't delete the datadog user %d", er);
                er = 0;
            }
            else
            {
                // delete the home directory that was left behind
                DeleteHomeDirectory(installedUser, sidResult.Sid.get());
            }
        }
    }
    // remove the auth token file altogether
    DeleteFile(authtokenfilename.c_str());
    std::wstring svcsInstalled;
    if (installState.getStringValue(installInstalledServices.c_str(), svcsInstalled))
    {
        // uninstall the services
        uninstallServices();
    }
    else
    {
        // this would have to be the rollback state, during an upgrade.
        // attempt to restart the services
        if (doesServiceExist(agentService))
        {
            // attempt to start it back up
            DoStartSvc(agentService);
        }
    }
    cleanupFolders();
    if (t == UNINSTALL_UNINSTALL)
    {
        if (regkey.deleteSubKey(strUninstallKeyName.c_str()))
        {
            WcaLog(LOGMSG_STANDARD, "Deleted registry keys");
        }
        else
        {
            WcaLog(LOGMSG_STANDARD, "Failed to delete registry keys %d", GetLastError());
        }
        if (!regkey.deleteValue(keyInstalledUser.c_str()))
        {
            WcaLog(LOGMSG_STANDARD, "deleted installed user key");
        }
        else
        {
            WcaLog(LOGMSG_STANDARD, "failed to delete installed user key");
        }
        if (!regkey.deleteValue(keyInstalledDomain.c_str()))
        {
            WcaLog(LOGMSG_STANDARD, "deleted installed domain key");
        }
        else
        {
            WcaLog(LOGMSG_STANDARD, "failed to delete installed domain key");
        }
        if (!regkey.deleteValue(keyClosedSourceEnabled.c_str()))
        {
            WcaLog(LOGMSG_STANDARD, "deleted closed source enabled key");
        }
        else
        {
            WcaLog(LOGMSG_STANDARD, "failed to delete closed source enabled key");
        }
    }

    if (hLsa)
    {
        LsaClose(hLsa);
    }
    return 0;
}
