#include "stdafx.h"

//! RemoveDDAgentUser
/*!
 \param hInstall handle to the currently running MSI
 
 This function removes all of the permissions that were added during an install
 that created 'ddagentuser', removes the user, and uninstalls the service
 from the service control manager

 This is a deferred custom action.  It should be executed immediately after the
 RemoveExistingProducts.

 The ddagentuser uninstalls didn't remove the user or service on upgrade,
 leaving them in place.  Must remove the ddagentuser (it's not needed any more)
 and remove the service registration so that it doesn't interfere with the
 new service registration
  */
extern "C" UINT __stdcall RemoveDDAgentUser(MSIHANDLE hInstall)
{
    // that's helpful.  WcaInitialize Log header silently limited to 32 chars
    HRESULT hr = WcaInitialize(hInstall, "CA: RemoveDDUser");
    UINT er = 0;
    ExitOnFailure(hr, "Failed to initialize");
     
    WcaLog(LOGMSG_STANDARD, "Initialized.");
    initializeStringsFromStringTable();
    DoStopSvc(hInstall, agentService);
    er = doDDUninstallAs(hInstall, UNINSTALL_UNINSTALL);
    if (er != 0) {
        hr = -1;
    }
LExit:
    er = SUCCEEDED(hr) ? ERROR_SUCCESS : ERROR_INSTALL_FAILURE;
    return WcaFinalize(er);
}