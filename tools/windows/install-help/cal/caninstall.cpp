#include "stdafx.h"
#include "TargetMachine.h"

bool canInstall(const CustomActionData &data, bool &bResetPassword, std::wstring *resultMessage)
{
    bool ddUserExists = false;
    bool ddServiceExists = false;
    bool isDC = false;
    bool isReadOnlyDC = false;
    bool isNtAuthority = false;
    bool isServiceAccount = false;
    bool isUserDomainUser = false;
    bool haveUserPassword = false;
    std::wstring userDomain;
    std::wstring computerDomain;

    // ddUserExists
    ddUserExists = data.DoesUserExist();

    // isServiceAccount
    isServiceAccount = data.IsServiceAccount();

    // ddServiceExists
    {
        auto result = doesServiceExist(agentService);
        if (-1 == result)
        {
            // error, call it false?
            ddServiceExists = false;
        }
        else if (0 == result)
        {
            ddServiceExists = false;
        }
        else if (1 == result)
        {
            ddServiceExists = true;
        }
    }

    // isNtAuthority
    if (ddUserExists)
    {
        const auto ntAuthoritySid = WellKnownSID::NTAuthority();
        if (!ntAuthoritySid.has_value())
        {
            WcaLog(LOGMSG_STANDARD, "Cannot check user SID against NT AUTHORITY: memory allocation failed");
        }
        else
        {
            isNtAuthority = EqualPrefixSid(data.Sid(), ntAuthoritySid.value().get());
        }
    }

    // isUserDomainUser
    isUserDomainUser = data.isUserDomainUser();

    // haveUserPassword
    haveUserPassword = data.present(propertyDDAgentUserPassword);

    // isDC
    isDC = data.GetTargetMachine()->IsDomainController();

    // isReadOnlyDC
    isReadOnlyDC = data.GetTargetMachine()->IsReadOnlyDomainController();

    // userDomain
    userDomain = data.Domain();

    // computerDomain
    computerDomain = data.GetTargetMachine()->JoinedDomainName().c_str();

    return canInstall(isDC, isReadOnlyDC, ddUserExists, isServiceAccount, isNtAuthority, isUserDomainUser,
                      haveUserPassword, userDomain, computerDomain, ddServiceExists, bResetPassword, resultMessage);
}

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
 * @param bResetPassword on return, set to true if the password needs to be reset based on configuration,
 *                       otherwise false.
 */
bool canInstall(bool isDC, bool isReadOnlyDC, bool ddUserExists, bool isServiceAccount, bool isNtAuthority,
                bool isUserDomainUser, bool haveUserPassword, std::wstring userDomain, std::wstring computerDomain,
                bool ddServiceExists, bool &bResetPassword, std::wstring *resultMessage)
{
    std::wstring errorMessage;
    bResetPassword = false;
    bool bRet = true;

    ///////////////////////////////////////////////////////////////////////////
    //
    // If domain controller:
    //   If user is present:
    //     if service is present:
    //        (1) this is an upgrade.
    //     if service is not present
    //        (2) this is a new install on this machine
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
    if (isDC)
    {
        if (!ddUserExists && isReadOnlyDC)
        {
            WcaLog(LOGMSG_STANDARD,
                   "(Configuration Error) Can't create user on RODC; install on a writable domain controller first");
            errorMessage = L"User does not exist and cannot be created from a read-only Domain Controller (RODC). "
                           L"Please create the user from a writeable domain controller first.";
            bRet = false;
        }
        if (!ddUserExists && ddServiceExists)
        {
            // case (3) above
            WcaLog(LOGMSG_STANDARD, "(Configuration Error) Invalid configuration; no DD user, but service exists");
            errorMessage =
                L"The agent service exists but the user account does not. Please contact support for assistance.";
            bRet = false;
        }
        if ((!ddUserExists || !ddServiceExists) && !isServiceAccount)
        {
            // case (4) and case (2)
            if (!haveUserPassword && !isNtAuthority)
            {
                // error case of case 2 & 4.  Must have the password to create the user in the domain,
                // because it must be reused by other domain controllers in domain.
                // must have the password to install the service for an existing user
                WcaLog(LOGMSG_STANDARD, "(Configuration Error)  Must supply password for dd-agent-user to create user "
                                        "and/or install service in a domain");
                if (ddUserExists)
                {
                    errorMessage = L"A password was not provided for the existing user account. A password is required "
                                   L"to create the agent services.";
                }
                else
                {
                    // TODO: This contradicts the online docs. Check which is correct.
                    errorMessage = L"A password is required for creating domain accounts. Please provide a password "
                                   L"for the user account.";
                }
                bRet = false;
            }
        }

        if (!ddUserExists && isDC && _wcsicmp(userDomain.c_str(), computerDomain.c_str()) != 0)
        {
            // on a domain controller, we can only create a user in this controller's domain.
            // check and reject an attempt to create a user not in this domain

            WcaLog(LOGMSG_STANDARD,
                   "(Configuration Error) Can't create a user that's not in this Domain Controller's domain.");
            errorMessage = L"The user account does not exist and cannot be created from this domain controller because "
                           L"the domain name provided for the user does not match the domain name managed by this "
                           L"Domain Controller. Please create the user account or provide an existing user account.";
            bRet = false;
        }
    }
    else
    {
        if (!ddUserExists && isUserDomainUser)
        {
            WcaLog(LOGMSG_STANDARD, "(Configuration Error) Can't create a domain user when not on a domain controller");
            WcaLog(LOGMSG_STANDARD,
                   "(Configuration Error) Install Datadog Agent on the domain controller for the %S domain",
                   userDomain.c_str());
            errorMessage = L"The user account does not exist and cannot be created because this computer is not a "
                           L"domain controller. Please create the user account or provide an existing user account.";
            bRet = false;
        }
        if (ddUserExists)
        {
            if (isUserDomainUser)
            {
                // if it's a domain user. We need the password if the service isn't here
                if (!ddServiceExists && !haveUserPassword && !isServiceAccount)
                {
                    // really an error case of (2). Even though we're not in a domain, if
                    // they supplied a domain user, we have to use it, which means we need
                    // the password
                    WcaLog(LOGMSG_STANDARD,
                           "(Configuration Error) Must supply the password to allow service registration");
                    errorMessage = L"A password was not provided for the existing user account. A password is required "
                                   L"to create the agent services.";
                    bRet = false;
                }
            }
            else
            {
                if (!ddServiceExists)
                {
                    // case (6)
                    WcaLog(LOGMSG_STANDARD, "dd user exists, but not service.  Continuing");
                    if (!isNtAuthority) // Don't reset password for NT AUTHORITY\* users
                    {
                        bResetPassword = true;
                    }
                }
            }
        }
        if (!ddUserExists && ddServiceExists)
        {
            // error case of (7)
            WcaLog(LOGMSG_STANDARD, "(Configuration Error) Invalid configuration; no DD user, but service exists");
            errorMessage =
                L"The agent service exists but the user account does not. Please contact support for assistance.";
            bRet = false;
        }
    }
    // case (1), case (2) if password provided, case (4) if password provided
    // case (5), case (6) but reset password, case (8) are all success.
    if (NULL != resultMessage)
    {
        *resultMessage = errorMessage;
    }
    return bRet;
}
