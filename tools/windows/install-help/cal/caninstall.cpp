#include "stdafx.h"

/**
 *  canInstall determines if the install can proceed based on the current
 * configuration of the machine, and whether we have enough information
 * to continue
 * 
 * @param isDC  whether or not this machine has been detected to be a DC
 * 
 * @param ddUserExists whether or not the specified ddagent user exists
 * 
 * @param ddServiceExists whether or not the datadog agent service is already configured on the system
 * 
 * @param data custom action data passed into the custom action by way of properties from the core install
 * 
 * @param bResetPassword on return, set to true if the password needs to be reset based on configuration,
 *                       otherwise false.
 */
bool canInstall(BOOL isDC, int ddUserExists, int ddServiceExists, const CustomActionData &data, bool &bResetPassword)
{
    bResetPassword = false;
    ///////////////////////////////////////////////////////////////////////////
    //
    // If domain controller:
    //   If user is present:
    //     if service is present:
    //        (1) this is an upgrade.
    //     if service is not present
    //        (2) this is new install on this machine
    //        dd user has already been created in domain
    //        must have password for registering service
    //   If user is NOT present
    //     if service is present
    //       (3) ERROR how could service be present but user not present?
    //     if service is not present
    //       (4) new install in this domain
    //       must have password for user creation and service installation
    //
    // If NOT a domain controller
    //   if user is present
    //     if the service is present
    //       (5) this is an upgrade, shouldn't need to do anything for user/service
    //     if the service is not present
    //       (6) No longer an error due to incident response. Allow user to be present,
    //           but must reset password
    //   if the user is NOT present
    //     if the service is present
    //       (7) error, should never be in this state.
    //     if the service is not present
    //       (8) install service, create user
    //       use password if provided, otherwise generate
    if (isDC) {
        if (!ddUserExists && ddServiceExists) {
            // case (3) above
            WcaLog(LOGMSG_STANDARD, "Invalid configuration; no DD user, but service exists");
            return false;
        }
        if (!ddUserExists || !ddServiceExists) {
            // case (4) and case (2)
            if (!data.present(propertyDDAgentUserPassword)) {
                // error case of case 2 & 4.  Must have the password to create the user in the domain,
                // because it must be reused by other domain controllers in domain.
                // must have the password to install the service for an existing user
                WcaLog(LOGMSG_STANDARD, "Must supply password for dd-agent-user to create user and/or install service in a domain");
                return false;
            }
        }
        if(!ddUserExists &&
            (_wcsicmp(data.Domain().c_str(), domainname.c_str())))
        {
            // on a domain controller, we can only create a user in this controller's domain.
            // check and reject an attempt to create a user not in this domain
            WcaLog(LOGMSG_STANDARD, "Can't create a user that's not in this domain: %S (asked for %S)",
                domainname.c_str(), data.Domain().c_str());
                return false;
        }
    }
    else {
        if(!ddUserExists && data.isUserDomainUser()) {
            WcaLog(LOGMSG_STANDARD, "Can't create a domain user when not on a domain controller");
            WcaLog(LOGMSG_STANDARD, "Install Datadog Agent on the domain controller for the %S domain", data.Domain().c_str());
            return false;
        }
        if (ddUserExists)
        {
            if (data.isUserDomainUser()) {
                // if it's a domain user. We need the password if the service isn't here
                if (!ddServiceExists && !data.present(propertyDDAgentUserPassword))
                {
                    // really an error case of (2). Even though we're not in a domain, if
                    // they supplied a domain user, we have to use it, which means we need
                    // the password
                    WcaLog(LOGMSG_STANDARD, "Must supply the password to allow service registration");
                    return false;
                }
            }
            else {
                if (!ddServiceExists) {
                    // case (6)
                    WcaLog(LOGMSG_STANDARD, "dd user exists %S, but not service.  Continuing", data.Username().c_str());
                    bResetPassword = true;
                }
            }
        }
        if (!ddUserExists && ddServiceExists) {
            // error case of (7)
            WcaLog(LOGMSG_STANDARD, "Invalid configuration; no DD user, but service exists");
            return false;
        }
    }
    // case (1), case (2) if password provided, case (4) if password provided
    // case (5), case (6) but reset password, case (8) are all success.
    return true;
}