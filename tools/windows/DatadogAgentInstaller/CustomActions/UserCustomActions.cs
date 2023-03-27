using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Diagnostics;
using System.DirectoryServices.ActiveDirectory;
using System.IO;
using System.Security.AccessControl;
using System.Security.Cryptography;
using System.Security.Principal;
using System.Windows.Forms;
using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Microsoft.Deployment.WindowsInstaller;

namespace Datadog.CustomActions
{
    public class UserCustomActions
    {
        private readonly ISession _session;
        private readonly INativeMethods _nativeMethods;
        private readonly IRegistryServices _registryServices;
        private readonly IDirectoryServices _directoryServices;
        private readonly IFileServices _fileServices;
        private readonly IServiceController _serviceController;

        public UserCustomActions(
            ISession session,
            INativeMethods nativeMethods,
            IRegistryServices registryServices,
            IDirectoryServices directoryServices,
            IFileServices fileServices,
            IServiceController serviceController)
        {
            _session = session;
            _nativeMethods = nativeMethods;
            _registryServices = registryServices;
            _directoryServices = directoryServices;
            _fileServices = fileServices;
            _serviceController = serviceController;
        }

        public UserCustomActions(ISession session)
        : this(
            session,
            new Win32NativeMethods(),
            new RegistryServices(),
            new DirectoryServices(),
            new FileServices(),
            new ServiceController()
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
            // This message is displayed to the customer in a dialog box. Ensure the text is well formatted.
            string errorDialogMessage = null;

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
                var datadogAgentServiceExists = _serviceController.ServiceExists("datadogagent");

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
                        errorDialogMessage = "The name provided is not a user account. Please supply a user account name in the format domain\\username.";
                        throw new InvalidOperationException(errorDialogMessage);
                    }
                    _session["DDAGENTUSER_FOUND"] = "true";
                    _session["DDAGENTUSER_SID"] = securityIdentifier.ToString();
                    isServiceAccount = _nativeMethods.IsServiceAccount(securityIdentifier);
                    isDomainAccount = _nativeMethods.IsDomainAccount(securityIdentifier);
                    _session.Log($"\"{domain}\\{userName}\" ({securityIdentifier.Value}, {nameUse}) is a {(isDomainAccount ? "domain" : "local")} {(isServiceAccount ? "service " : string.Empty)}account");

                    if (string.IsNullOrEmpty(ddAgentUserPassword) &&
                        !isServiceAccount)
                    {
                        if (isDomainController &&
                            !datadogAgentServiceExists)
                        {
                            errorDialogMessage = "A password was not provided. Passwords are required for non-service accounts on Domain Controllers.";
                            throw new InvalidOperationException(errorDialogMessage);
                        }

                        if (isDomainAccount &&
                            !datadogAgentServiceExists)
                        {
                            errorDialogMessage = "A password was not provided. Passwords are required for domain accounts.";
                            throw new InvalidOperationException(errorDialogMessage);
                        }
                    }
                }
                else
                {
                    _session["DDAGENTUSER_FOUND"] = "false";
                    _session.Log($"User {ddAgentUserName} doesn't exist.");

                    if (isDomainController)
                    {
                        errorDialogMessage = "The account does not exist. The account must already exist when installing on Domain Controllers.";
                        throw new InvalidOperationException(errorDialogMessage);
                    }

                    ParseUserName(ddAgentUserName, out userName, out domain);
                }

                if (string.IsNullOrEmpty(userName))
                {
                    // If userName is empty at this point, then it is likely that the input is malformed
                    errorDialogMessage = $"Unable to parse account name from {ddAgentUserName}. Please ensure the account name follows the format domain\\username.";
                    throw new Exception(errorDialogMessage);
                }

                if (string.IsNullOrEmpty(domain))
                {
                    // This case is hit if user specifies a username without a domain part and it does not exist
                    _session.Log("domain part is empty, using default");
                    domain = GetDefaultDomainPart();
                }

                // We are trying to create a user in a domain on a non-domain controller.
                // This must run *after* checking that the domain is not empty.
                if (!userFound &&
                    domain != Environment.MachineName)
                {
                    errorDialogMessage = "The account does not exist. Domain accounts must already exist when installing on Domain Clients.";
                    throw new InvalidOperationException(errorDialogMessage);
                }

                _session.Log($"Installing with DDAGENTUSER_PROCESSED_NAME={userName} and DDAGENTUSER_PROCESSED_DOMAIN={domain}");
                // Create new DDAGENTUSER_PROCESSED_NAME property so we don't modify the property containing
                // the user provided value DDAGENTUSER_NAME
                _session["DDAGENTUSER_PROCESSED_NAME"] = userName;
                _session["DDAGENTUSER_PROCESSED_DOMAIN"] = domain;
                _session["DDAGENTUSER_PROCESSED_FQ_NAME"] = $"{domain}\\{userName}";

                if (!isServiceAccount &&
                    !isDomainAccount  &&
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
            catch (Exception e)
            {
                _session.Log($"Error processing ddAgentUser credentials: {e}");
                if (string.IsNullOrEmpty(errorDialogMessage))
                {
                    errorDialogMessage = $"An unexpected error occurred while parsing the account name: {e.Message}";
                }

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
            if (calledFromUIControl)
            {
                _session["DDAgentUser_Valid"] = "True";
            }
            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult ProcessDdAgentUserCredentials(Session session)
        {
            return new UserCustomActions(new SessionWrapper(session)).ProcessDdAgentUserCredentials(calledFromUIControl: false);
        }

        [CustomAction]
        public static ActionResult ProcessDdAgentUserCredentialsUI(Session session)
        {
            return new UserCustomActions(new SessionWrapper(session)).ProcessDdAgentUserCredentials(calledFromUIControl: true);
        }

        private ActionResult AddUser()
        {
            try
            {
                var userFound = _session.Property("DDAGENTUSER_FOUND");
                var userSid = _session.Property("DDAGENTUSER_SID");
                var userName = _session.Property("DDAGENTUSER_PROCESSED_NAME");
                var userPassword = _session.Property("DDAGENTUSER_PROCESSED_PASSWORD");
                if (userFound != "true" && string.IsNullOrEmpty(userSid))
                {
                    _session.Log($"Creating user {userName}");
                    var ret = _nativeMethods.AddUser(userName, userPassword);
                    if (ret != 0)
                    {
                        throw new Win32Exception(ret);
                    }
                }
                else
                {
                    _session.Log($"{userName} already exists, not creating");
                }
            }
            catch (Exception e)
            {
                _session.Log($"Failed to create user: {e}");
                return ActionResult.Failure;
            }

            return ActionResult.Success;
        }

        public ActionResult ConfigureUser()
        {
            try
            {
                if (AddUser() != ActionResult.Success)
                {
                    return ActionResult.Failure;
                }

                var ddAgentUserName = $"{_session.Property("DDAGENTUSER_PROCESSED_FQ_NAME")}";
                var userFound = _nativeMethods.LookupAccountName(ddAgentUserName,
                    out _,
                    out _,
                    out var securityIdentifier,
                    out _);
                if (!userFound)
                {
                    throw new Exception($"Could not find user {ddAgentUserName}.");
                }
                
                var resetPassword = _session.Property("DDAGENTUSER_RESET_PASSWORD");
                var ddagentuserPassword = _session.Property("DDAGENTUSER_PROCESSED_PASSWORD");
                var ddagentuser = _session.Property("DDAGENTUSER_PROCESSED_NAME");
                if (!string.IsNullOrEmpty(resetPassword))
                {
                    _session.Log($"Resetting {ddagentuser} password.");
                    if (string.IsNullOrEmpty(ddagentuserPassword))
                    {
                        throw new InvalidOperationException("Asked to reset password, but password was not provided");
                    }
                    _nativeMethods.SetUserPassword(ddagentuser, ddagentuserPassword);
                }

                {
                    using var actionRecord = new Record(
                        "ConfigureUser",
                        $"Configuring service account {ddagentuser}",
                        ""
                    );
                    _session.Message(InstallMessage.ActionStart, actionRecord);
                }

                _nativeMethods.AddToGroup(securityIdentifier, WellKnownSidType.BuiltinPerformanceMonitoringUsersSid);
                _nativeMethods.AddToGroup(securityIdentifier, new SecurityIdentifier("S-1-5-32-573")); // Builtin\Event Log Readers

                _nativeMethods.AddPrivilege(securityIdentifier, AccountRightsConstants.SeDenyInteractiveLogonRight);
                _nativeMethods.AddPrivilege(securityIdentifier, AccountRightsConstants.SeDenyNetworkLogonRight);
                _nativeMethods.AddPrivilege(securityIdentifier, AccountRightsConstants.SeDenyRemoteInteractiveLogonRight);
                _nativeMethods.AddPrivilege(securityIdentifier, AccountRightsConstants.SeServiceLogonRight);

                // Necessary to allow the ddagentuser to read the registry
                {
                    using var actionRecord = new Record(
                        "ConfigureUser",
                        "Configuring registry permissions",
                        ""
                    );
                    _session.Message(InstallMessage.ActionStart, actionRecord);
                }
                var key = _registryServices.CreateRegistryKey(Registries.LocalMachine, "SOFTWARE\\Datadog\\Datadog Agent");
                if (key != null)
                {
                    var registrySecurity = new RegistrySecurity();
                    // Allow system and admins to access registry, standard privs
                    registrySecurity.AddAccessRule(new RegistryAccessRule(
                        new SecurityIdentifier("SY"),
                        RegistryRights.FullControl,
                        AccessControlType.Allow));
                    registrySecurity.AddAccessRule(new RegistryAccessRule(
                        new SecurityIdentifier("BA"),
                        RegistryRights.FullControl,
                        AccessControlType.Allow));
                    // Give ddagentuser full access, important so it can read settings
                    // TODO: Switch to readonly
                    registrySecurity.AddAccessRule(new RegistryAccessRule(
                        securityIdentifier,
                        RegistryRights.FullControl,
                        AccessControlType.Allow));
                    registrySecurity.SetAccessRuleProtection(false, true);
                    key.SetAccessControl(registrySecurity);
                }
                else
                {
                    throw new Exception("Could not set registry ACLs.");
                }

                {
                    using var actionRecord = new Record(
                        "ConfigureUser",
                        "Configuring file permissions",
                        ""
                    );
                    _session.Message(InstallMessage.ActionStart, actionRecord);
                }
                var files = new List<string>
                {
                    _session.Property("APPLICATIONDATADIRECTORY"),
                    Path.Combine(_session.Property("APPLICATIONDATADIRECTORY"), "logs"),
                    Path.Combine(_session.Property("APPLICATIONDATADIRECTORY"), "logs\\agent.log"),
                    Path.Combine(_session.Property("APPLICATIONDATADIRECTORY"), "conf.d"),
                    Path.Combine(_session.Property("APPLICATIONDATADIRECTORY"), "auth_token"),
                    Path.Combine(_session.Property("APPLICATIONDATADIRECTORY"), "datadog.yaml"),
                    Path.Combine(_session.Property("APPLICATIONDATADIRECTORY"), "system-probe.yaml"),
                    Path.Combine(_session.Property("PROJECTLOCATION"), "embedded2"),
                    Path.Combine(_session.Property("PROJECTLOCATION"), "embedded3"),

                };
                foreach (var filePath in files)
                {
                    if (!_directoryServices.Exists(filePath) && !_fileServices.Exists(filePath))
                    {
                        if (filePath.Contains("embedded3"))
                        {
                            throw new InvalidOperationException($"The file {filePath} doesn't exist, but it should");
                        }
                        _session.Log($"{filePath} does not exists, skipping changing ACLs.");
                        continue;
                    }

                    FileSystemSecurity fileSystemSecurity;
                    string sddl;
                    try
                    {
                        if (_directoryServices.Exists(filePath))
                        {
                            fileSystemSecurity = _directoryServices.GetAccessControl(filePath, AccessControlSections.All);
                            sddl = $"D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;WD;;;BU)(A;OICI;FA;;;{securityIdentifier.Value})";
                        }
                        else
                        {
                            fileSystemSecurity = _fileServices.GetAccessControl(filePath, AccessControlSections.All);
                            sddl = $"D:PAI(A;;FA;;;SY)(A;;FA;;;BA)(A;;WD;;;BU)(A;;FA;;;{securityIdentifier.Value})";
                        }
                    }
                    catch (Exception e)
                    {
                        _session.Log($"Failed to get ACLs on {filePath}: {e}");
                        throw;
                    }

                    _session.Log($"{filePath} current ACLs: {fileSystemSecurity.GetSecurityDescriptorSddlForm(AccessControlSections.All)}");

                    // Set owner and group only if necessary
                    if (fileSystemSecurity.GetSecurityDescriptorSddlForm(AccessControlSections.Owner) != "O:SY" ||
                        fileSystemSecurity.GetSecurityDescriptorSddlForm(AccessControlSections.Group) != "G:SY")
                    {
                        fileSystemSecurity.SetSecurityDescriptorSddlForm($"O:SYG:SY{sddl}");
                    }
                    else
                    {
                        fileSystemSecurity.SetSecurityDescriptorSddlForm(sddl);
                    }

                    try
                    {
                        if (_directoryServices.Exists(filePath))
                        {
                            _directoryServices.SetAccessControl(filePath, (DirectorySecurity)fileSystemSecurity);
                        }
                        else
                        {
                            _fileServices.SetAccessControl(filePath, (FileSecurity)fileSystemSecurity);
                        }
                    }
                    catch (Exception e)
                    {
                        try
                        {
                            // Try again but without owner/group
                            fileSystemSecurity.SetSecurityDescriptorSddlForm(sddl);
                            if (_directoryServices.Exists(filePath))
                            {
                                _directoryServices.SetAccessControl(filePath, (DirectorySecurity)fileSystemSecurity);
                            }
                            else
                            {
                                _fileServices.SetAccessControl(filePath, (FileSecurity)fileSystemSecurity);
                            }
                        }
                        catch (Exception)
                        {
                            _session.Log($"Failed to set ACLs on {filePath}: {e}");
                            throw;
                        }
                    }

                    try
                    {
                        if (_directoryServices.Exists(filePath))
                        {
                            fileSystemSecurity = _directoryServices.GetAccessControl(filePath);
                        }
                        else
                        {
                            fileSystemSecurity = _fileServices.GetAccessControl(filePath);
                        }

                        _session.Log($"{filePath} new ACLs: {fileSystemSecurity.GetSecurityDescriptorSddlForm(AccessControlSections.All)}");
                    }
                    catch (Exception e)
                    {
                        _session.Log($"Failed to get ACLs on {filePath}: {e}");
                    }
                }

                return ActionResult.Success;
            }
            catch (Exception e)
            {
                _session.Log($"Failed to configure user: {e}");
                return ActionResult.Failure;
            }
        }

        [CustomAction]
        public static ActionResult ConfigureUser(Session session)
        {
            return new UserCustomActions(new SessionWrapper(session)).ConfigureUser();
        }
    }
}
