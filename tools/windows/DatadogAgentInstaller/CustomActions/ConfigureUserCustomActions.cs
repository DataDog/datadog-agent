using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.DirectoryServices.ActiveDirectory;
using System.IO;
using System.Security.AccessControl;
using System.Security.Cryptography;
using System.Security.Principal;
using Newtonsoft.Json;
using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Microsoft.Deployment.WindowsInstaller;
using Datadog.CustomActions.RollbackData;

namespace Datadog.CustomActions
{
    public class ConfigureUserCustomActions
    {
        private readonly ISession _session;
        private readonly INativeMethods _nativeMethods;
        private readonly IRegistryServices _registryServices;
        private readonly IFileSystemServices _fileSystemServices;
        private readonly IServiceController _serviceController;

        private RollbackDataStore _rollbackDataStore;

        private SecurityIdentifier _ddAgentUserSID;
        private SecurityIdentifier _previousDdAgentUserSID;

        public ConfigureUserCustomActions(
            ISession session,
            string rollbackdataname,
            INativeMethods nativeMethods,
            IRegistryServices registryServices,
            IFileSystemServices fileSystemServices,
            IServiceController serviceController)
        {
            _session = session;
            _nativeMethods = nativeMethods;
            _registryServices = registryServices;
            _fileSystemServices = fileSystemServices;
            _serviceController = serviceController;

            _rollbackDataStore = new RollbackDataStore(_session, rollbackdataname, _fileSystemServices);
        }

        public ConfigureUserCustomActions(ISession session, string rollbackdataname)
            : this(
                session,
                rollbackdataname
                new Win32NativeMethods(),
                new RegistryServices(),
                new FileSystemServices(),
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

        /// <summary>
        /// Add ddagentuser to groups
        /// </summary>
        private void ConfigureUserGroups()
        {
            _nativeMethods.AddToGroup(_ddAgentUserSID, WellKnownSidType.BuiltinPerformanceMonitoringUsersSid);
            // Builtin\Event Log Readers
            _nativeMethods.AddToGroup(_ddAgentUserSID, new SecurityIdentifier("S-1-5-32-573"));
        }

        /// <summary>
        /// User Rights Assignment for ddagentuser
        /// https://learn.microsoft.com/en-us/windows/security/threat-protection/security-policy-settings/user-rights-assignment
        /// </summary>
        private void ConfigureUserAccountRights()
        {
            _nativeMethods.AddPrivilege(_ddAgentUserSID, AccountRightsConstants.SeDenyInteractiveLogonRight);
            _nativeMethods.AddPrivilege(_ddAgentUserSID, AccountRightsConstants.SeDenyNetworkLogonRight);
            _nativeMethods.AddPrivilege(_ddAgentUserSID, AccountRightsConstants.SeDenyRemoteInteractiveLogonRight);
            _nativeMethods.AddPrivilege(_ddAgentUserSID, AccountRightsConstants.SeServiceLogonRight);
        }

        private void ConfigureRegistryPermissions()
        {
            // Necessary to allow the ddagentuser to read the registry
            var key = _registryServices.CreateRegistryKey(Registries.LocalMachine,
                Constants.DatadogAgentRegistryKey);
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
                    _ddAgentUserSID,
                    RegistryRights.FullControl,
                    AccessControlType.Allow));
                registrySecurity.SetAccessRuleProtection(false, true);
                key.SetAccessControl(registrySecurity);
            }
            else
            {
                throw new Exception("Could not set registry ACLs.");
            }
        }

        /// <summary>
        /// set base permissions on APPLICATIONDATADIRECTORY, restrict access to admins only.
        /// This clears the ACL, any custom permissions added by customers will be removed.
        /// Any non-inherited ACE added to children of APPLICATIONDATADIRECTORY will be persisted.
        /// </summary>
        private void SetBaseInheritablePermissions()
        {
            FileSystemSecurity fileSystemSecurity = new DirectorySecurity();
            // disable inheritance, discard inherited rules
            fileSystemSecurity.SetAccessRuleProtection(true, false);
            // Administrators FullControl
            fileSystemSecurity.AddAccessRule(new FileSystemAccessRule(
                new SecurityIdentifier(WellKnownSidType.BuiltinAdministratorsSid, null),
                FileSystemRights.FullControl,
                InheritanceFlags.ContainerInherit | InheritanceFlags.ObjectInherit,
                PropagationFlags.None,
                AccessControlType.Allow));
            // SYSTEM FullControl
            fileSystemSecurity.AddAccessRule(new FileSystemAccessRule(
                new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null),
                FileSystemRights.FullControl,
                InheritanceFlags.ContainerInherit | InheritanceFlags.ObjectInherit,
                PropagationFlags.None,
                AccessControlType.Allow));
            UpdateAndLogAccessControl(_session.Property("APPLICATIONDATADIRECTORY"), fileSystemSecurity);
        }

        private SecurityIdentifier GetPreviousAgentUser()
        {
            try
            {
                using var subkey =
                    _registryServices.OpenRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey);
                var domain = subkey.GetValue("installedDomain")?.ToString();
                var user = subkey.GetValue("installedUser")?.ToString();
                if (string.IsNullOrEmpty(domain) || string.IsNullOrEmpty(user))
                {
                    throw new Exception("Agent user information is not in registry");
                }

                var name = $"{domain}\\{user}";
                _session.Log($"Found agent user information in registry {name}");
                var userFound = _nativeMethods.LookupAccountName(name,
                    out _,
                    out _,
                    out var securityIdentifier,
                    out _);
                if (!userFound || securityIdentifier == null)
                {
                    throw new Exception($"Could not find account for user {name}.");
                }

                _session.Log($"Found previous agent user {name} ({securityIdentifier})");
                return securityIdentifier;
            }
            catch (Exception e)
            {
                _session.Log($"Could not find previous agent user: {e}");
            }

            return null;
        }

        /// <summary>
        /// Recursively enables inheritance on files in APPLICATIONDATADIRECTORY
        /// Removes any redundant explicit ACEs for the agent user
        /// If changing the agent user, removes the previous agent user as owner of any files in APPLICATIONDATADIRECTORY
        /// </summary>
        private void EnablePermissionInheritance()
        {
            // Ensure that inheritance is enabled on all files/folders in the config directory.
            // This is important so that @SetBaseInheritablePermissions are applied and the agent
            // user has access where it needs it.
            // If changing agent user, remove old user as owner/group from all files/folders
            foreach (var filePath in Directory.EnumerateFileSystemEntries(
                         _session.Property("APPLICATIONDATADIRECTORY"),
                         "*.*", SearchOption.AllDirectories))
            {
                FileSystemSecurity fileSystemSecurity =
                    _fileSystemServices.GetAccessControl(filePath, AccessControlSections.All);
                bool changed = false;

                // If changing agent user, remove old user as owner/group from all files/folders
                if (_previousDdAgentUserSID != null && _previousDdAgentUserSID != _ddAgentUserSID)
                {
                    changed |= RemoveOwnerGroup(filePath, fileSystemSecurity, _previousDdAgentUserSID);
                }

                // Ensure that inheritance is enabled on all files/folders in the config directory
                if (fileSystemSecurity.AreAccessRulesProtected)
                {
                    // enable inheritance
                    fileSystemSecurity.SetAccessRuleProtection(false, true);
                    changed = true;

                    changed |= RemoveRedundantExplicitAccess(filePath, fileSystemSecurity);
                }

                if (changed)
                {
                    UpdateAndLogAccessControl(filePath, fileSystemSecurity);
                }
            }
        }

        private void UpdateAndLogAccessControl(string filePath, FileSystemSecurity fileSystemSecurity)
        {
            var oldfs =
                _fileSystemServices.GetAccessControl(filePath, AccessControlSections.All);
            _session.Log(
                $"{filePath} current ACLs: {oldfs.GetSecurityDescriptorSddlForm(AccessControlSections.All)}");

            _rollbackDataStore.Add(new FilePermissionRollbackData(filePath, _fileSystemServices));
            _fileSystemServices.SetAccessControl(filePath, fileSystemSecurity);

            var newfs = _fileSystemServices.GetAccessControl(filePath, AccessControlSections.All);
            _session.Log(
                $"{filePath} new ACLs: {newfs.GetSecurityDescriptorSddlForm(AccessControlSections.All)}");
        }

        private bool RemoveOwnerGroup(string filePath, FileSystemSecurity fileSystemSecurity, SecurityIdentifier sid)
        {
            var changed = false;
            var owner = (SecurityIdentifier)fileSystemSecurity.GetOwner(typeof(SecurityIdentifier));
            if ((SecurityIdentifier)owner == sid)
            {
                _session.Log($"{filePath} setting owner to SYSTEM");
                fileSystemSecurity.SetOwner(new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null));
                changed = true;
            }

            var group = fileSystemSecurity.GetGroup(typeof(SecurityIdentifier));
            if ((SecurityIdentifier)group == sid)
            {
                _session.Log($"{filePath} setting group to SYSTEM");
                fileSystemSecurity.SetGroup(new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null));
                changed = true;
            }

            return changed;
        }

        /// <summary>
        /// Remove explicit access rules added by the pre-7.47 installer that now have inherited analogues
        /// </summary>
        private bool RemoveRedundantExplicitAccess(string filePath, FileSystemSecurity fileSystemSecurity)
        {
            var changed = false;
            // Remove explicit access rules added by the pre-7.47 installer that will now have inherited analogues
            foreach (var sid in new SecurityIdentifier[]
                     {
                         new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null),
                         new SecurityIdentifier(WellKnownSidType.BuiltinAdministratorsSid, null),
                         _ddAgentUserSID,
                         _previousDdAgentUserSID
                     })
            {
                if (sid == null)
                {
                    continue;
                }

                if (fileSystemSecurity.RemoveAccessRule(new FileSystemAccessRule(
                        sid,
                        FileSystemRights.FullControl,
                        InheritanceFlags.ContainerInherit | InheritanceFlags.ObjectInherit,
                        PropagationFlags.None,
                        AccessControlType.Allow)))
                {
                    _session.Log($"{filePath} removing explicit access rule for {sid}");
                    changed = true;
                }
            }

            // Use AccessRuleFactory instead of a FileSystemAccessRule constructor because they all
            // automatically add FileSystemRights.Synchronize which was not included by the pre-7.47 installer.
            if (fileSystemSecurity.RemoveAccessRule(
                    (FileSystemAccessRule)fileSystemSecurity.AccessRuleFactory(
                        new SecurityIdentifier(WellKnownSidType.BuiltinUsersSid, null),
                        (int)FileSystemRights.ChangePermissions,
                        false,
                        InheritanceFlags.ContainerInherit | InheritanceFlags.ObjectInherit,
                        PropagationFlags.None,
                        AccessControlType.Allow)))
            {
                _session.Log($"{filePath} removing explicit access rule for Users");
                changed = true;
            }

            return changed;
        }

        /// <summary>
        /// Add explicit ACE for the agent user
        /// Remove explicit ACE for old agent user if changing the agent user
        /// </summary>
        private void GrantAgentAccessPermissions()
        {
            // add ddagentuser FullControl to select places
            foreach (var filePath in PathsWithAgentAccess())
            {
                if (!_fileSystemServices.Exists(filePath))
                {
                    if (filePath.Contains("embedded3"))
                    {
                        throw new InvalidOperationException($"The file {filePath} doesn't exist, but it should");
                    }

                    _session.Log($"{filePath} does not exists, skipping changing ACLs.");
                    continue;
                }

                FileSystemSecurity fileSystemSecurity;
                try
                {
                    fileSystemSecurity = _fileSystemServices.GetAccessControl(filePath, AccessControlSections.All);
                }
                catch (Exception e)
                {
                    _session.Log($"Failed to get ACLs on {filePath}: {e}");
                    throw;
                }

                // if changing user during change/repair make sure to remove the rule for the old user
                // if this is an upgrade assume the removing installer properly removes its permissions.
                if (_previousDdAgentUserSID != null && _previousDdAgentUserSID != _ddAgentUserSID &&
                    string.IsNullOrEmpty(_session.Property("WIX_UPGRADE_DETECTED")))
                {
                    if (fileSystemSecurity.RemoveAccessRule(new FileSystemAccessRule(
                            _previousDdAgentUserSID,
                            FileSystemRights.FullControl,
                            InheritanceFlags.ContainerInherit | InheritanceFlags.ObjectInherit,
                            PropagationFlags.None,
                            AccessControlType.Allow)))
                    {
                        _session.Log($"{filePath} removing explicit access rule for {_previousDdAgentUserSID}");
                    }
                }

                if (_fileSystemServices.IsDirectory(filePath))
                {
                    // ddagentuser FullControl, enable child inheritance of this ACE
                    fileSystemSecurity.AddAccessRule(new FileSystemAccessRule(
                        _ddAgentUserSID,
                        FileSystemRights.FullControl,
                        InheritanceFlags.ContainerInherit | InheritanceFlags.ObjectInherit,
                        PropagationFlags.None,
                        AccessControlType.Allow));
                }
                else if (_fileSystemServices.IsFile(filePath))
                {
                    // ddagentuser FullControl
                    fileSystemSecurity.AddAccessRule(new FileSystemAccessRule(
                        _ddAgentUserSID,
                        FileSystemRights.FullControl,
                        AccessControlType.Allow));
                }

                try
                {
                    UpdateAndLogAccessControl(filePath, fileSystemSecurity);
                }
                catch (Exception e)
                {
                    _session.Log($"Failed to set ACLs on {filePath}: {e}");
                    throw;
                }
            }
        }

        private void ConfigureFilePermissions()
        {
            try
            {
                try
                {
                    // SeRestorePrivilege is required to set file owner to a different user
                    _nativeMethods.EnablePrivilege("SeRestorePrivilege");
                }
                catch (Exception e)
                {
                    _session.Log(
                        $"Failed to enable SeRestorePrivilege. Some file permissions may not be able to be set/rolled back: {e}");
                }

                SetBaseInheritablePermissions();

                EnablePermissionInheritance();

                if (_ddAgentUserSID != new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null))
                {
                    GrantAgentAccessPermissions();
                }
            }
            catch (Exception e)
            {
                _session.Log($"Error configuring file permissions: {e}");
                throw;
            }
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
                    out _ddAgentUserSID,
                    out _);
                if (!userFound || _ddAgentUserSID == null)
                {
                    throw new Exception($"Could not find user {ddAgentUserName}.");
                }

                _previousDdAgentUserSID = GetPreviousAgentUser();

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
                ConfigureUserGroups();
                ConfigureUserAccountRights();

                {
                    using var actionRecord = new Record(
                        "ConfigureUser",
                        "Configuring registry permissions",
                        ""
                    );
                    _session.Message(InstallMessage.ActionStart, actionRecord);
                }
                ConfigureRegistryPermissions();

                {
                    using var actionRecord = new Record(
                        "ConfigureUser",
                        "Configuring file permissions",
                        ""
                    );
                    _session.Message(InstallMessage.ActionStart, actionRecord);
                }
                ConfigureFilePermissions();

                return ActionResult.Success;
            }
            catch (Exception e)
            {
                _session.Log($"Failed to configure user: {e}");
                return ActionResult.Failure;
            }
            finally
            {
                _rollbackDataStore.Store();
            }
        }

        [CustomAction]
        public static ActionResult ConfigureUser(Session session)
        {
            return new ConfigureUserCustomActions(new SessionWrapper(session), "ConfigureUser").ConfigureUser();
        }

        public ActionResult ConfigureUserRollback()
        {
            try
            {
                try
                {
                    // SeRestorePrivilege is required to set file owner to a different user
                    _nativeMethods.EnablePrivilege("SeRestorePrivilege");
                }
                catch (Exception e)
                {
                    _session.Log(
                        $"Failed to enable SeRestorePrivilege. Some file permissions may not be able to be set/rolled back: {e}");
                }

                _rollbackDataStore.Restore();
            }
            catch (Exception e)
            {
                _session.Log($"Failed to rollback user configuration: {e}");
                return ActionResult.Failure;
            }

            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult ConfigureUserRollback(Session session)
        {
            return new ConfigureUserCustomActions(new SessionWrapper(session), "ConfigureUser").ConfigureUserRollback();
        }

        private List<string> PathsWithAgentAccess()
        {
            return new List<string>
            {
                // agent needs to be able to write logs/
                // agent GUI needs to be able to edit config
                // agent needs to be able to write to run/
                // agent needs to be able to create auth_token
                _session.Property("APPLICATIONDATADIRECTORY"),
                // allow agent to write __pycache__
                Path.Combine(_session.Property("PROJECTLOCATION"), "embedded2"),
                Path.Combine(_session.Property("PROJECTLOCATION"), "embedded3"),
            };
        }

        /// <summary>
        /// Remove an explicit access ACE for the ddagentuser for @sid from @filePath
        /// </summary>
        /// <param name="sid"></param>
        /// <param name="filePath"></param>
        private void removeAgentAccess(SecurityIdentifier sid, string filePath)
        {
            var fileSystemSecurity = _fileSystemServices.GetAccessControl(filePath, AccessControlSections.All);
            _session.Log(
                $"{filePath} current ACLs: {fileSystemSecurity.GetSecurityDescriptorSddlForm(AccessControlSections.All)}");

            if (!fileSystemSecurity.RemoveAccessRule(new FileSystemAccessRule(
                    sid,
                    FileSystemRights.FullControl,
                    InheritanceFlags.ContainerInherit | InheritanceFlags.ObjectInherit,
                    PropagationFlags.None,
                    AccessControlType.Allow)))
            {
                return;
            }

            UpdateAndLogAccessControl(filePath, fileSystemSecurity);
        }

        public ActionResult UninstallUser()
        {
            try
            {
                // lookup sid for ddagentuser
                var ddAgentUserName = $"{_session.Property("DDAGENTUSER_NAME")}";
                var userFound = _nativeMethods.LookupAccountName(ddAgentUserName,
                    out _,
                    out _,
                    out var securityIdentifier,
                    out _);
                if (!userFound || securityIdentifier == null)
                {
                    _session.Log($"Could not find user {ddAgentUserName}");
                    return ActionResult.Success;
                }

                // Remove explicit ACE for ddagentuser
                {
                    using var actionRecord = new Record(
                        "UninstallUser",
                        "Removing file access",
                        ""
                    );
                    _session.Message(InstallMessage.ActionStart, actionRecord);
                }
                if (securityIdentifier != new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null))
                {
                    _session.Log($"Removing file access for {ddAgentUserName} ({securityIdentifier})");
                    foreach (var filePath in PathsWithAgentAccess())
                    {
                        try
                        {
                            if (_fileSystemServices.Exists(filePath))
                            {
                                removeAgentAccess(securityIdentifier, filePath);
                            }
                        }
                        catch (Exception e)
                        {
                            _session.Log($"Failed to remove {ddAgentUserName} from {filePath}: {e}");
                        }
                    }
                }

                // We intentionally do NOT delete the ddagentuser account.
                // For domain accounts, the account may still be in use elsewhere and we can't delete accounts from domain clients.
                // For local accounts, sometimes even after uninstall the ddagentuser user profile is still loaded
                // and Windows does not provide a way to remove it without a reboot.
            }
            catch (Exception e)
            {
                _session.Log($"Failed to uninstall user: {e}");
                return ActionResult.Failure;
            }

            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult UninstallUser(Session session)
        {
            return new ConfigureUserCustomActions(new SessionWrapper(session), "UninstallUser").UninstallUser();
        }
    }
}
