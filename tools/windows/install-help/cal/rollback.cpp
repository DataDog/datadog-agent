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
 extern "C" UINT __stdcall RollbackInstallation(MSIHANDLE hInstall)
{
    HRESULT hr = S_OK;
    UINT er = ERROR_SUCCESS;
    bool bDDUserWasAdded = false;
    bool bDDUserPasswordChanged = false;
    bool bDDUserFilePermsChanged = false;
    bool bDDUserPerfmon = false;
    bool bDDRegPermsChanged = false;
   
    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    hr = WcaInitialize(hInstall, "CA: Rollback");
    ExitOnFailure(hr, "Failed to initialize");
    WcaLog(LOGMSG_STANDARD, "Rollback Initialized.");



    WcaLog(LOGMSG_STANDARD, "Custom action rollback complete");
LExit:
    er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    return WcaFinalize(er);


}
