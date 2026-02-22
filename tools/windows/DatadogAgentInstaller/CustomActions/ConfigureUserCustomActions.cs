using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Datadog.CustomActions.Rollback;
using WixToolset.Dtf.WindowsInstaller;
using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.IO;
using System.Linq;
using System.Security.AccessControl;
using System.Security.Principal;

namespace Datadog.CustomActions
{
    public class ConfigureUserCustomActions
    {
        private readonly ISession _session;
        private readonly INativeMethods _nativeMethods;
        private readonly IRegistryServices _registryServices;
        private readonly IFileSystemServices _fileSystemServices;
        private readonly IServiceController _serviceController;

        private readonly RollbackDataStore _rollbackDataStore;

        private SecurityIdentifier _ddAgentUserSID;
        private SecurityIdentifier _previousDdAgentUserSID;

        public ConfigureUserCustomActions(
            ISession session,
            string rollbackDataName,
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

            _rollbackDataStore = new RollbackDataStore(_session, rollbackDataName, _fileSystemServices, _serviceController);
        }

        public ConfigureUserCustomActions(ISession session, string rollbackDataName)
            : this(
                session,
                rollbackDataName,
                new Win32NativeMethods(),
                new RegistryServices(),
                new FileSystemServices(),
                new ServiceController()
            )
        {
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
            try
            {
                if (_nativeMethods.IsReadOnlyDomainController())
                {
                    _session.Log("Host is a Read-Only Domain controller, user cannot be added to groups by the installer." +
                                 " Install will continue, agent may not function properly if user has not been added to these groups.");
                    return;
                }
            }
            catch (Exception e)
            {
                // On error assume the host is not a read-only domain controller
                // If the host is actually a read-only domain controller then the following operations will fail
                _session.Log($"Error determining if host is a read-only domain controller, continuing assuming it is not: {e}");
                _session.Log("If the host is actually a read-only domain controller, ensure the LanmanServer/Server service is running.");
            }

            _nativeMethods.AddToGroup(_ddAgentUserSID, WellKnownSidType.BuiltinPerformanceMonitoringUsersSid);
            // Required for using ETW - we would not need this right if the Agent was running as virtual service account (as they have
            // the same rights as LocalService, which can use ETW by default.
            // See https://www.geoffchappell.com/studies/windows/km/ntoskrnl/api/etw/secure/index.htm
            //   "By default, only the administrator of the computer, users in the Performance Log Users group, and services running as LocalSystem,
            //    *LocalService*, NetworkService can control trace sessions and provide and consume event data.
            //    Only users with administrative privileges and services running as LocalSystem can start and control an NT Kernel Logger session."
            _nativeMethods.AddToGroup(_ddAgentUserSID, WellKnownSidType.BuiltinPerformanceLoggingUsersSid);
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
            fileSystemSecurity.SetOwner(new SecurityIdentifier(
                WellKnownSidType.LocalSystemSid, null));
            fileSystemSecurity.SetGroup(new SecurityIdentifier(
                WellKnownSidType.LocalSystemSid, null));
            UpdateAndLogAccessControl(_session.Property("APPLICATIONDATADIRECTORY"), fileSystemSecurity);
        }

        /// <summary>
        /// Reset the permissions for select files and directories in the agent configuration directory APPLICATIONDATADIRECTORY.
        /// - Enable ACE inheritance
        /// - Remove redundant explicit ACEs for the agent user that may have been left by previous installer versions
        /// - If changing the agent user, remove the previous agent user as owner/group of any files and directories
        /// </summary>
        /// <remarks>
        /// This mirrors the file/directories that previous versions of the installer would directly apply an ACE
        /// for the ddagentuser to. Now that ddagentuser ACE is inherited from the configuration directory, we just need
        /// to ensure that inheritance is enabled (previous versions of the installer marked the DACLs protected).
        ///
        /// We select specific subdirs/files instead of enumerating the entire configuration directory so that
        /// we don't trample over any subdirs/files that a customer may have added. In particular, in case a
        /// customer stores the secret_backend_command in the configuration directory, we don't want to enable
        /// inheritance on this file and cause it to inherit ACEs that will cause the agent to refuse to execute it.
        /// </remarks>
        private void ResetConfigurationPermissions()
        {
            var paths = _session.PathsWithAgentAccess();
            // add dirs recursively only if they exist (EnumerateFileSystemEntries throws an exception if they don't)
            foreach (var dir in paths.Where(_fileSystemServices.IsDirectory).ToArray())
            {
                paths.AddRange(Directory.EnumerateFileSystemEntries(dir, "*.*", SearchOption.AllDirectories));
            }

            foreach (var filePath in paths)
            {
                if (!_fileSystemServices.Exists(filePath))
                {
                    continue;
                }
                var fileSystemSecurity =
                    _fileSystemServices.GetAccessControl(filePath, AccessControlSections.All);
                var changed = false;

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
            foreach (var filePath in _session.PathsWithAgentAccess())
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

        private void AddDatadogUserToDataFolder()
        {
            var dataDirectory = _session.Property("APPLICATIONDATADIRECTORY");

            FileSystemSecurity fileSystemSecurity;
            try
            {
                fileSystemSecurity = _fileSystemServices.GetAccessControl(dataDirectory, AccessControlSections.All);
            }
            catch (Exception e)
            {
                _session.Log($"Failed to get ACLs on {dataDirectory}: {e}");
                throw;
            }
            // ddagentuser Read and execute permissions, enable child inheritance of this ACE
            fileSystemSecurity.AddAccessRule(new FileSystemAccessRule(
                _ddAgentUserSID,
                FileSystemRights.ReadAndExecute | FileSystemRights.Synchronize,
                InheritanceFlags.ContainerInherit | InheritanceFlags.ObjectInherit,
                PropagationFlags.None,
                AccessControlType.Allow));

            // datadog write on this folder
            // This allows creating new files/folders, but not deleting or modifying permissions.
            fileSystemSecurity.AddAccessRule(new FileSystemAccessRule(
                _ddAgentUserSID,
                FileSystemRights.WriteData | FileSystemRights.AppendData | FileSystemRights.WriteAttributes | FileSystemRights.WriteExtendedAttributes | FileSystemRights.Synchronize,
                InheritanceFlags.None,
                PropagationFlags.None,
                AccessControlType.Allow));

            // add full control to CREATOR OWNER
            // Grants FullControl to any files/directories created by the Agent user
            // Marked InherityOnly so it applies only to children and not this directory
            fileSystemSecurity.AddAccessRule(new FileSystemAccessRule(
                new SecurityIdentifier(WellKnownSidType.CreatorOwnerSid, null),
                FileSystemRights.FullControl,
                InheritanceFlags.ContainerInherit | InheritanceFlags.ObjectInherit,
                PropagationFlags.InheritOnly,
                AccessControlType.Allow));
            try
            {
                UpdateAndLogAccessControl(dataDirectory, fileSystemSecurity);
            }
            catch (Exception e)
            {
                _session.Log($"Failed to set ACLs on {dataDirectory}: {e}");
                throw;
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

                // Let's make sure the SE_DACL_AUTO_INHERITED flag is correctly set
                // Resetting it on %PROJECTLOCATION% will propagate to the subfolders
                RestoreDaclRollbackCustomAction.RestoreAutoInheritedFlag(_session.Property("PROJECTLOCATION"));

                SetBaseInheritablePermissions();

                ResetConfigurationPermissions();

                if (_ddAgentUserSID != new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null))
                {
                    AddDatadogUserToDataFolder();
                    GrantAgentAccessPermissions();
                }
            }
            catch (Exception e)
            {
                _session.Log($"Error configuring file permissions: {e}");
                throw;
            }
        }

        /// <summary>
        /// Returns the name (key) of the secret used to store the Agent password in the LSA secret store
        /// </summary>
        public static string AgentPasswordPrivateDataKey()
        {
            // use L$ prefix to indicate the secret is "Local" level secret, this prevents
            // the secret from being accessed remotely.
            // https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-lsad/483f1b6e-7b14-4341-9ab2-9b99c01f896e
            // https://learn.microsoft.com/en-us/windows/win32/secmgmt/private-data-object
            // The above pages aren't super clear on which users can access local secrets, but the following pages
            // show that only Administrators have POLICY_GET_PRIVATE_INFORMATION by default, and this can
            // be confirmed empirically.
            // https://learn.microsoft.com/en-us/windows/win32/secmgmt/policy-object-access-rights
            // https://learn.microsoft.com/en-us/windows/win32/secmgmt/policy-object-protection
            // https://learn.microsoft.com/en-us/windows/win32/secmgmt/private-data-object-initial-protection
            // Machine private data objects, 'M$', are not readable (through the API) even by LocalSystem.
            // https://learn.microsoft.com/en-us/windows/win32/secmgmt/private-data-object
            var secretType = "L$";
            return $"{secretType}datadog_ddagentuser_password";
        }

        private void ConfigureUserPassword()
        {
            var ddagentuserPassword = _session.Property("DDAGENTUSER_PROCESSED_PASSWORD");
            var ddagentuser = _session.Property("DDAGENTUSER_PROCESSED_NAME");
            var resetPassword = _session.Property("DDAGENTUSER_RESET_PASSWORD");
            if (!string.IsNullOrEmpty(resetPassword))
            {
                _session.Log($"Resetting {ddagentuser} password.");
                if (string.IsNullOrEmpty(ddagentuserPassword))
                {
                    throw new InvalidOperationException("Asked to reset password, but password was not provided");
                }

                _nativeMethods.SetUserPassword(ddagentuser, ddagentuserPassword);
            }

            // Store the password in LSA secret store so that it can be used during Fleet Automation remote upgrades
            // This is the same place that Windows Service Manager stores service account passwords, so
            // this isn't introducing any NEW security risks.
            // https://docs.microsoft.com/en-us/windows/win32/services/service-accounts
            // https://learn.microsoft.com/en-us/windows/win32/secmgmt/storing-private-data
            var keyName = AgentPasswordPrivateDataKey();
            var isServiceAccount = _session.Property("DDAGENTUSER_IS_SERVICE_ACCOUNT") == "true";
            if (isServiceAccount)
            {
                // If ddagentuser is a service account, it has no password, so remove any previous entries from the LSA
                _session.Log("Agent user is a service account, removing password from LSA secret store");
                try
                {
                    _nativeMethods.RemoveSecret(keyName);
                }
                catch (Exception e)
                {
                    // Don't fail if we fail to remove the secret.
                    // ProcessDDAgentUserCredentials will appropriately clear the password property for service accounts
                    // so it being left behind shouldn't affect future installs and may be removed then.
                    _session.Log($"Failed to remove agent secret: {e}");
                }
            }
            else if (!string.IsNullOrEmpty(ddagentuserPassword))
            {
                // NOTE: The Agent installer allows upgrades without re-providing the password, so the
                //       password property may be empty and we don't want to overwrite the secret store
                //       with an empty password.
                //       ProcessDDAgentUserCredentials should read the password from the LSA secret store
                //       if it wasn't provided on the command line so this may no longer be a possibility.
                _session.Log("Agent user has a password, storing in LSA secret store");
                _nativeMethods.StoreSecret(keyName, ddagentuserPassword);
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

                _previousDdAgentUserSID = InstallStateCustomActions.GetPreviousAgentUser(_session, _registryServices, _nativeMethods);

                var ddagentuser = _session.Property("DDAGENTUSER_PROCESSED_NAME");
                ConfigureUserPassword();

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

        public static ActionResult ConfigureUser(Session session)
        {
            return new ConfigureUserCustomActions(new SessionWrapper(session), "ConfigureUser").ConfigureUser();
        }

        public ActionResult ConfigureUserRollback()
        {
            RunRollbackDataRestore();
            return ActionResult.Success;
        }

        public static ActionResult ConfigureUserRollback(Session session)
        {
            return new ConfigureUserCustomActions(new SessionWrapper(session), "ConfigureUser").ConfigureUserRollback();
        }


        /// <summary>
        /// Remove an explicit access ACE for the ddagentuser for @sid from @filePath
        /// </summary>
        /// <param name="sid"></param>
        /// <param name="filePath"></param>
        private void RemoveAgentAccess(SecurityIdentifier sid, string filePath)
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

        private void RunRollbackDataRestore()
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
            }
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
                    foreach (var filePath in _session.PathsWithAgentAccess().Where(_fileSystemServices.Exists))
                    {
                        try
                        {
                            RemoveAgentAccess(securityIdentifier, filePath);
                        }
                        catch (Exception e)
                        {
                            _session.Log($"Failed to remove {ddAgentUserName} from {filePath}: {e}");
                        }
                    }
                    //remove datadog access to root folder and restore to base permissions
                    SetBaseInheritablePermissions();
                }

                // Remove password from LSA secret store
                var upgrading = !string.IsNullOrEmpty(_session.Property("UPGRADINGPRODUCTCODE"));
                var fleetAutomation = _session.Property("FLEET_INSTALL") == "1";
                if (upgrading || fleetAutomation)
                {
                    // If this is an upgrade, we don't want to remove the password from the LSA secret store
                    // because the new version of the Agent will need it. Technically it should already have it
                    // because it's fetched in ProcessDDAgentUserCredentials, which runs before the existing
                    // products are removed, but we don't want to lose it on rollback.
                    // TODO(WINA-1357): rollback the password if the upgrade fails
                    // If this is a Fleet Automation upgrade (uninstall->install workflow), we don't want to remove
                    // the password from the LSA secret store because the new version of the Agent will need it.
                    _session.Log($"Upgrade detected, not removing password from LSA secret store");
                }
                else
                {
                    _session.Log("Uninstall detected, removing password from LSA secret store");
                    try
                    {
                        var keyName = AgentPasswordPrivateDataKey();
                        _nativeMethods.RemoveSecret(keyName);
                    }
                    catch (Exception e)
                    {
                        // Don't fail if we fail to remove the secret.
                        // ProcessDDAgentUserCredentials will appropriately clear the password property for service accounts
                        // so it being left behind shouldn't affect future installs and may be removed then.
                        _session.Log($"Failed to remove agent secret: {e}");
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
            finally
            {
                _rollbackDataStore.Store();
            }

            return ActionResult.Success;
        }

        public static ActionResult UninstallUser(Session session)
        {
            return new ConfigureUserCustomActions(new SessionWrapper(session), "UninstallUser").UninstallUser();
        }

        public ActionResult UninstallUserRollback()
        {
            RunRollbackDataRestore();
            return ActionResult.Success;
        }

        public static ActionResult UninstallUserRollback(Session session)
        {
            return new ConfigureUserCustomActions(new SessionWrapper(session), "UninstallUser").UninstallUserRollback();
        }
    }
}
