using System;
using System.DirectoryServices.ActiveDirectory;
using System.Security.Cryptography;
using System.Security.Principal;
using System.Windows.Forms;
using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Microsoft.Deployment.WindowsInstaller;

namespace Datadog.CustomActions
{
    // ExceptionWithDialogMessage is a custom exception type that contains a message that can be displayed to the user in a dialog box.
    // Use to distinguish an expected error with a message that should be displayed to the user from an unexpected exception.
    // This message is displayed to the customer in a dialog box. Ensure the text is well formatted.
    public class ExceptionWithDialogMessage : InvalidOperationException
    {
        public ExceptionWithDialogMessage(string message) : base(message)
        {
        }
    }

    public class ProcessUserCustomActions
    {
        private readonly ISession _session;
        private readonly INativeMethods _nativeMethods;
        private readonly IServiceController _serviceController;
        private readonly IRegistryServices _registryServices;

        public ProcessUserCustomActions(
            ISession session,
            INativeMethods nativeMethods,
            IServiceController serviceController,
            IRegistryServices registryServices)
        {
            _session = session;
            _nativeMethods = nativeMethods;
            _serviceController = serviceController;
            _registryServices = registryServices;
        }

        public ProcessUserCustomActions(ISession session)
            : this(
                session,
                new Win32NativeMethods(),
                new ServiceController(),
                new RegistryServices()
            )
        {
        }

        private static string GetRandomPassword(int length)
        {
            var rgb = new byte[length];
            var rngCrypt = new RNGCryptoServiceProvider();
            rngCrypt.GetBytes(rgb);
            return Convert.ToBase64String(rgb);
        }

        /// <summary>
        /// Determine the default 'domain' part of a user account name when one is not provided by the user.
        /// </summary>
        ///
        /// <remarks>
        /// We default to creating a local account if the domain
        /// part is not specified in DDAGENTUSER_NAME.
        /// However, domain controllers do not have local accounts, so we must
        /// default to a domain account.
        /// We still want to default to local accounts for domain clients
        /// though, so it is not enough to check if the computer is domain joined,
        /// we must specifically check if this computer is a domain controller.
        /// </remarks>
        private string GetDefaultDomainPart()
        {
            if (!_nativeMethods.IsDomainController())
            {
                return Environment.MachineName;
            }

            try
            {
                return _nativeMethods.GetComputerDomain();
            }
            catch (ActiveDirectoryObjectNotFoundException)
            {
                // Computer is not joined to a domain, it can't be a DC
            }

            // Computer is not a DC, default to machine name (NetBIOS name)
            return Environment.MachineName;
        }

        /// <summary>
        /// Returns true if we will treat <c>name</c> as an alias for the local machine name.
        /// </summary>
        /// <remarks>
        /// Comparisons are case-insensitive.
        /// </remarks>
        private bool NameIsLocalMachine(string name)
        {
            name = name.ToLower();
            if (name == Environment.MachineName.ToLower())
            {
                return true;
            }

            // Windows runas does not support the following names, but we can try to.
            if (name == ".")
            {
                return true;
            }

            // Windows runas and logon screen do not support the following names, but we can try to.
            if (_nativeMethods.GetComputerName(COMPUTER_NAME_FORMAT.ComputerNameDnsHostname, out var hostname))
            {
                if (name == hostname.ToLower())
                {
                    return true;
                }
            }

            if (_nativeMethods.GetComputerName(COMPUTER_NAME_FORMAT.ComputerNameDnsFullyQualified, out var fqdn))
            {
                if (name == fqdn.ToLower())
                {
                    return true;
                }
            }

            return false;
        }

        /// <summary>
        /// Returns true if <c>name</c> should be replaced by GetDefaultDomainPart()
        /// </summary>
        /// <remarks>
        /// Comparisons are case-insensitive.
        /// </remarks>
        private bool NameUsesDefaultPart(string name)
        {
            if (name == ".")
            {
                return true;
            }

            return false;
        }

        /// <summary>
        /// Gets the domain and user parts from an account name.
        /// </summary>
        /// <remarks>
        /// Windows has varying support for name syntax accross the OS, this function may normalize the domain
        /// part to the machine name (NetBIOS name) or the domain name.
        /// See NameUsesDefaultPart and NameIsLocalMachine for details on the supported aliases.
        /// For example,
        ///   * on regular hosts, .\user => machinename\user
        ///   * on domain controllers, .\user => domain\user
        /// </remarks>
        private void ParseUserName(string account, out string userName, out string domain)
        {
            // We do not use CredUIParseUserName because it does not handle some cases nicely.
            // e.g. CredUIParseUserName(host.ddev.net\user) returns userName=.ddev.net domain=host.ddev.net
            // e.g. CredUIParseUserName(.\user) returns userName=.\user domain=
            if (account.Contains("\\"))
            {
                var parts = account.Split('\\');
                domain = parts[0];
                userName = parts[1];
                if (NameUsesDefaultPart(domain))
                {
                    domain = GetDefaultDomainPart();
                }
                else if (NameIsLocalMachine(domain))
                {
                    domain = Environment.MachineName;
                }

                return;
            }

            // If no \\, then full string is username
            userName = account;
            domain = "";
        }

        /// <summary>
        /// Wrapper for the LookupAccountName Windows API that also supports additional syntax for the domain part of the name.
        /// See ParseUserName for details on the supported names.
        /// </summary>
        private bool LookupAccountWithExtendedDomainSyntax(
            string account,
            out string userName,
            out string domain,
            out SecurityIdentifier securityIdentifier,
            out SID_NAME_USE nameUse)
        {
            // Provide the account name to Windows as is first, see if Windows can handle it.
            var userFound = _nativeMethods.LookupAccountName(account,
                out userName,
                out domain,
                out securityIdentifier,
                out nameUse);
            if (!userFound)
            {
                // The first LookupAccountName failed, this could be because the user does not exist,
                // or it could be because the domain part of the name is invalid.
                ParseUserName(account, out var tmpUser, out var tmpDomain);
                // Try LookupAccountName again but using a fixed domain part.
                account = $"{tmpDomain}\\{tmpUser}";
                _session.Log($"User not found, trying again with fixed domain part: {account}");
                userFound = _nativeMethods.LookupAccountName(account,
                    out userName,
                    out domain,
                    out securityIdentifier,
                    out nameUse);
            }

            return userFound;
        }


        /// <summary>
        /// Throws an exception if the agent user is the same as the current user.
        /// </summary>
        /// <remarks>
        /// Since the installer modifies the user account, if the current user is provided for the ddagentuser the account will be locked out.
        /// If a customer does this by mistake, they will have to use a different account to log in to the machine and fix the account.
        /// To avoid this, we disallow using the current user as the ddagentuser unless
        ///   - The user is a service account (e.g. LocalSystem)
        ///   - The agent is already installed and the agent user is not being changed
        /// If a customer must use the current user, they can pass ALLOW_CURRENT_USER=true to the installer on the command line.
        /// </remarks>
        private void TestAgentUserIsNotCurrentUser(SecurityIdentifier agentUser, bool isServiceAccount)
        {
            var allowInstallCurrentUser = _session.Property("ALLOW_CURRENT_USER")?.ToLower() == "true";
            string currentUserName;
            SecurityIdentifier currentUserSID;
            try
            {
                _nativeMethods.GetCurrentUser(out currentUserName, out currentUserSID);
            }
            catch (Exception e)
            {
                _session.Log($"Unable to get current user SID: {e}");
                return;
            }
            _session.Log($"Currently logged in user: {currentUserName} ({currentUserSID})");

            // If the user is a service account (e.g. LocalSystem) then it's ok to use the same account
            if (isServiceAccount)
            {
                return;
            }

            // good, agent user and current user are different
            if (!currentUserSID.Equals(agentUser))
            {
                return;
            }

            var _previousDdAgentUserSID = InstallStateCustomActions.GetPreviousAgentUser(_session, _registryServices, _nativeMethods);
            if (_previousDdAgentUserSID != null)
            {
                _session.Log($"Previous agent user: {_previousDdAgentUserSID}");
                // If the previous agent user is the same as the new agentUser, then we are upgrading/changing/repairing and the user has already
                // opted in to allow it to be the same as the current user.
                if (_previousDdAgentUserSID.Equals(agentUser))
                {
                    _session.Log("The account provided is the same as the currently logged in user and the previous agent user, allowing install to continue.");
                    return;
                }
            }

            if (allowInstallCurrentUser)
            {
                _session.Log(
                    "The account provided is the same as the current user, but the user has opted in to allow this.");
                return;
            }

            throw new ExceptionWithDialogMessage("The account provided is the same as the currently logged in user. Please supply a different account for the Datadog Agent.");
        }

        /// <summary>
        /// Throws an exception if the password is required but not provided.
        /// </summary>
        private void TestIfPasswordIsRequiredAndProvidedForExistingAccount(string ddAgentUserPassword, bool isDomainController,
            bool isServiceAccount, bool isDomainAccount, bool datadogAgentServiceExists)
        {
            var passwordProvided = !string.IsNullOrEmpty(ddAgentUserPassword);

            // If password is provided or the account is a service account (no password), we're good
            if (passwordProvided || isServiceAccount)
            {
                return;
            }

            if (!passwordProvided && isDomainController &&
                !datadogAgentServiceExists)
            {
                throw new ExceptionWithDialogMessage(
                    "A password was not provided. Passwords are required for non-service accounts on Domain Controllers.");
            }

            if (!passwordProvided && isDomainAccount &&
                !datadogAgentServiceExists)
            {
                throw new InvalidOperationException(
                    "A password was not provided. Passwords are required for domain accounts.");
            }
        }

        private ActionResult HandleProcessDdAgentUserCredentialsException(Exception e, string errorDialogMessage, bool calledFromUIControl)
        {
            _session.Log($"Error processing ddAgentUser credentials: {e}");
            if (calledFromUIControl)
            {
                // When called from InstallUISequence we must return success for the modal dialog to show,
                // otherwise the installer exits. The control that called this action should check the
                // DDAgentUser_Valid property to determine if this function succeeded or failed.
                // Error information is contained in the ErrorModal_ErrorMessage property.
                // MsiProcessMessage doesn't work here so we must use our own custom error popup.
                _session["ErrorModal_ErrorMessage"] = errorDialogMessage;
                _session["DDAgentUser_Valid"] = "False";
                return ActionResult.Success;
            }

            // Send an error message, the installer may display an error popup depending on the UILevel.
            // https://learn.microsoft.com/en-us/windows/win32/msi/user-interface-levels
            {
                using var actionRecord = new Record
                {
                    FormatString = errorDialogMessage
                };
                _session.Message(InstallMessage.Error
                                 | (InstallMessage)((int)MessageBoxButtons.OK | (int)MessageBoxIcon.Warning),
                    actionRecord);
            }
            // When called from InstallExecuteSequence we want to fail on error
            return ActionResult.Failure;
        }

        /// <summary>
        /// Processes the DDAGENTUSER_NAME and DDAGENTUSER_PASSWORD properties into formats that can be
        /// consumed by other custom actions. Also does some basic error handling/checking on the property values.
        /// </summary>
        /// <param name="calledFromUIControl"></param>
        /// <returns></returns>
        /// <remarks>
        /// This function must support being called multiple times during the install, as the user can back/next the
        /// UI multiple times.
        ///
        /// When calledFromUIControl is true: sets property DDAgentUser_Valid="True" on success, on error, stores error information in the ErrorModal_ErrorMessage property.
        ///
        /// When calledFromUIControl is false (during InstallExecuteSequence), sends an InstallMessage.Error message.
        /// The installer may display an error popup depending on the UILevel.
        /// https://learn.microsoft.com/en-us/windows/win32/msi/user-interface-levels
        /// </remarks>
        public ActionResult ProcessDdAgentUserCredentials(bool calledFromUIControl = false)
        {
            try
            {
                if (calledFromUIControl)
                {
                    // reset output properties
                    _session["ErrorModal_ErrorMessage"] = "";
                    _session["DDAgentUser_Valid"] = "False";
                }

                var ddAgentUserName = _session.Property("DDAGENTUSER_NAME");
                var ddAgentUserPassword = _session.Property("DDAGENTUSER_PASSWORD");
                var isDomainController = _nativeMethods.IsDomainController();
                var isReadOnlyDomainController = _nativeMethods.IsReadOnlyDomainController();
                var datadogAgentServiceExists = _serviceController.ServiceExists(Constants.AgentServiceName);

                // LocalSystem is not supported by LookupAccountName as it is a pseudo account,
                // do the conversion here for user's convenience.
                if (ddAgentUserName == "LocalSystem")
                {
                    ddAgentUserName = "NT AUTHORITY\\SYSTEM";
                }
                else if (ddAgentUserName == "LocalService")
                {
                    ddAgentUserName = "NT AUTHORITY\\LOCAL SERVICE";
                }
                else if (ddAgentUserName == "NetworkService")
                {
                    ddAgentUserName = "NT AUTHORITY\\NETWORK SERVICE";
                }

                if (string.IsNullOrEmpty(ddAgentUserName))
                {
                    if (isDomainController)
                    {
                        // require user to provide a username on domain controllers so that the customer is explicit
                        // about the username/password that will be created on their domain if it does not exist.
                        throw new ExceptionWithDialogMessage("A username was not provided. A username is a required when installing on Domain Controllers.");
                    }

                    // Creds are not in registry and user did not pass a value, use default account name
                    ddAgentUserName = $"{GetDefaultDomainPart()}\\ddagentuser";
                    _session.Log($"No creds provided, using default {ddAgentUserName}");
                }

                // Check if user exists, and parse the full account name
                var userFound = LookupAccountWithExtendedDomainSyntax(
                    ddAgentUserName,
                    out var userName,
                    out var domain,
                    out var securityIdentifier,
                    out var nameUse);
                var isServiceAccount = false;
                var isDomainAccount = false;
                if (userFound)
                {
                    _session.Log($"Found {userName} in {domain} as {nameUse}");
                    // Ensure name belongs to a user account or special accounts like SYSTEM, and not to a domain, computer or group.
                    if (nameUse != SID_NAME_USE.SidTypeUser && nameUse != SID_NAME_USE.SidTypeWellKnownGroup)
                    {
                        throw new ExceptionWithDialogMessage("The name provided is not a user account. Please supply a user account name in the format domain\\username.");
                    }

                    _session["DDAGENTUSER_FOUND"] = "true";
                    _session["DDAGENTUSER_SID"] = securityIdentifier.ToString();
                    isServiceAccount = _nativeMethods.IsServiceAccount(securityIdentifier);
                    isDomainAccount = _nativeMethods.IsDomainAccount(securityIdentifier);
                    _session.Log(
                        $"\"{domain}\\{userName}\" ({securityIdentifier.Value}, {nameUse}) is a {(isDomainAccount ? "domain" : "local")} {(isServiceAccount ? "service " : string.Empty)}account");

                    TestAgentUserIsNotCurrentUser(securityIdentifier, isServiceAccount);
                    TestIfPasswordIsRequiredAndProvidedForExistingAccount(ddAgentUserPassword, isDomainController, isServiceAccount, isDomainAccount, datadogAgentServiceExists);
                }
                else
                {
                    _session["DDAGENTUSER_FOUND"] = "false";
                    _session["DDAGENTUSER_SID"] = null;
                    _session.Log($"User {ddAgentUserName} doesn't exist.");

                    ParseUserName(ddAgentUserName, out userName, out domain);
                }

                if (string.IsNullOrEmpty(userName))
                {
                    // If userName is empty at this point, then it is likely that the input is malformed
                    throw new ExceptionWithDialogMessage($"Unable to parse account name from {ddAgentUserName}. Please ensure the account name follows the format domain\\username.");
                }

                if (string.IsNullOrEmpty(domain))
                {
                    // This case is hit if user specifies a username without a domain part and it does not exist
                    _session.Log("domain part is empty, using default");
                    domain = GetDefaultDomainPart();
                }

                // User does not exist and we cannot create user account from RODC
                if (!userFound && isReadOnlyDomainController)
                {
                    throw new ExceptionWithDialogMessage("The account does not exist. Domain accounts must already exist when installing on Read-Only Domain Controllers.");
                }

                // We are trying to create a user in a domain on a non-domain controller.
                // This must run *after* checking that the domain is not empty.
                if (!userFound &&
                    !isDomainController &&
                    domain != Environment.MachineName)
                {
                    throw new ExceptionWithDialogMessage("The account does not exist. Domain accounts must already exist when installing on Domain Clients.");
                }

                _session.Log(
                    $"Installing with DDAGENTUSER_PROCESSED_NAME={userName} and DDAGENTUSER_PROCESSED_DOMAIN={domain}");
                // Create new DDAGENTUSER_PROCESSED_NAME property so we don't modify the property containing
                // the user provided value DDAGENTUSER_NAME
                _session["DDAGENTUSER_PROCESSED_NAME"] = userName;
                _session["DDAGENTUSER_PROCESSED_DOMAIN"] = domain;
                _session["DDAGENTUSER_PROCESSED_FQ_NAME"] = $"{domain}\\{userName}";

                _session["DDAGENTUSER_RESET_PASSWORD"] = null;
                if (!userFound &&
                    isDomainController &&
                    string.IsNullOrEmpty(ddAgentUserPassword))
                {
                    // require user to provide a password on domain controllers so that the customer is explicit
                    // about the username/password that will be created on their domain if it does not exist.
                    throw new ExceptionWithDialogMessage("A password was not provided. A password is a required when installing on Domain Controllers.");
                }

                if (!isServiceAccount &&
                    !isDomainAccount &&
                    string.IsNullOrEmpty(ddAgentUserPassword))
                {
                    _session.Log("Generating a random password");
                    _session["DDAGENTUSER_RESET_PASSWORD"] = "yes";
                    ddAgentUserPassword = GetRandomPassword(128);
                }
                else if (isServiceAccount && !string.IsNullOrEmpty(ddAgentUserPassword))
                {
                    _session.Log("Ignoring provided password because account is a service account");
                    ddAgentUserPassword = null;
                }

                _session["DDAGENTUSER_PROCESSED_PASSWORD"] = ddAgentUserPassword;
            }
            catch (ExceptionWithDialogMessage e)
            {
                return HandleProcessDdAgentUserCredentialsException(e, e.Message, calledFromUIControl);
            }
            catch (Exception e)
            {
                var errorDialogMessage = $"An unexpected error occurred while parsing the account name: {e.Message}";
                return HandleProcessDdAgentUserCredentialsException(e, errorDialogMessage, calledFromUIControl);
            }

            if (calledFromUIControl)
            {
                _session["DDAgentUser_Valid"] = "True";
            }

            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult ProcessDdAgentUserCredentials(Session session)
        {
            return new ProcessUserCustomActions(new SessionWrapper(session)).ProcessDdAgentUserCredentials(
                calledFromUIControl: false);
        }

        [CustomAction]
        public static ActionResult ProcessDdAgentUserCredentialsUI(Session session)
        {
            return new ProcessUserCustomActions(new SessionWrapper(session)).ProcessDdAgentUserCredentials(
                calledFromUIControl: true);
        }
    }
}
