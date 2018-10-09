#include "stdafx.h"


/*
 * Rollback is initiated in the event of a failed installation.  Rollback
 * should do the following actions
 *
 * Remove the dd-user IFF this installation added the dd user
 * Remove the secret user IFF this installation added the user
 * Remove the secret user password from the registry IFF it was added by this installation
 *
 * Whether or not those operations were initiated by this installation is indicated
 * by flags set in the registry
 */
void logProcCount();
 extern "C" UINT __stdcall RollbackInstallation(MSIHANDLE hInstall)
{
    HRESULT hr = S_OK;
    UINT er = ERROR_SUCCESS;
    bool bDDUserWasAdded = false;
    bool bDDUserPasswordChanged = false;
    bool bDDUserFilePermsChanged = false;
    bool bDDUserPerfmon = false;
    bool bDDRegPermsChanged = false;
    std::wstring propertystring;
    std::map<std::wstring, bool> params;
    
    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    hr = WcaInitialize(hInstall, "CA: Rollback");
    ExitOnFailure(hr, "Failed to initialize");
    logProcCount();
    WcaLog(LOGMSG_STANDARD, "Rollback Initialized.");
    
    // check and see what was done during the install so far

    bDDUserWasAdded = WasInstallStepCompleted(strDdUserCreated);
    bDDUserPasswordChanged = WasInstallStepCompleted(strDdUserPasswordChanged);
    bDDUserFilePermsChanged = WasInstallStepCompleted(strFilePermissionsChanged);

    bDDRegPermsChanged = WasInstallStepCompleted(strChangedRegistryPermissions);

    if (bDDUserWasAdded) {
        WcaLog(LOGMSG_STANDARD, "dd-agent-user created by this install, undoing");
        doRemoveDDUser();
    }
    else {
        WcaLog(LOGMSG_STANDARD, "dd-agent-user not created by this install, not undoing");
    }


    WcaLog(LOGMSG_STANDARD, "Custom action rollback complete");
LExit:
    er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    return WcaFinalize(er);


}
