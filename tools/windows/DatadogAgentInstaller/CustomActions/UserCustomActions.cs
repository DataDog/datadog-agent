using System;
using System.Collections.Generic;
using System.DirectoryServices.ActiveDirectory;
using System.IO;
using System.Security.AccessControl;
using System.Security.Cryptography;
using System.Security.Principal;
using System.Windows.Forms;
using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Native;
using Microsoft.Deployment.WindowsInstaller;
using Microsoft.Win32;
using static Datadog.CustomActions.Native.NativeMethods;

namespace Datadog.CustomActions
{
    // Fetch and process registry value(s) and return a string to be assigned to a WIX property.
    using GetRegistryPropertyHandler = Func<ISession, string>;

    public class UserCustomActions
    {
        public static string GetRandomPassword(int length)
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
        private static string GetDefaultDomainPart()
        {
            try
            {
                var serverInfo = NetServerGetInfo<SERVER_INFO_101>();
                if ((serverInfo.Type & ServerTypes.DomainCtrl) == ServerTypes.DomainCtrl
                    || (serverInfo.Type & ServerTypes.BackupDomainCtrl) == ServerTypes.BackupDomainCtrl)
                {
                    // Computer is a DC, default to domain name
                    var joinedDomain = Domain.GetComputerDomain();
                    return joinedDomain.Name;
                }
                // Computer is not a DC, default to machine name
            }
            catch (ActiveDirectoryObjectNotFoundException)
            {
                // not joined to a domain, use the machine name
            }
            return Environment.MachineName;
        }

        /// <summary>
        /// If the WIX property <c>propertyName</c> does not have a value, assign it the value returned by <c>handler</c>.
        /// This gives precedence to properties provided on the command line over the registry values.
        /// </summary>
        private static void RegistryProperty(ISession session, string propertyName, GetRegistryPropertyHandler handler)
        {
            if (string.IsNullOrEmpty(session[propertyName]))
            {
                try
                {
                    var propertyVal = handler(session);
                    if (!string.IsNullOrEmpty(propertyVal))
                    {
                        session[propertyName] = propertyVal;
                        session.Log($"Found {propertyName} in registry {session[propertyName]}");
                    }
                }
                catch (Exception e)
                {
                    session.Log($"Exception processing registry value for {propertyName}: {e}");
                }
            }
            else
            {
                session.Log($"User provided {propertyName} {session[propertyName]}");
            }
        }

        /// <summary>
        /// Convenience wrapper of <c>RegistryProperty</c> for properties that have an exact 1:1 mapping to a registry value
        /// and don't require additional processing.
        /// </summary>
        private static void RegistryValueProperty(ISession session, string propertyName, RegistryKey registryKey, string registryValue)
        {
            RegistryProperty(session, propertyName,
                GetRegistryPropertyHandler => registryKey.GetValue(registryValue)?.ToString());
        }

        /// <summary>
        /// Assigns WIX properties that were not provided by the user to their registry values.
        /// </summary>
        /// <remarks>
        /// Custom Action that runs (only once) in either the InstallUISequence or the InstallExecuteSequence.
        /// </remarks>
        private static ActionResult ReadRegistryProperties(ISession session)
        {
            try
            {
                using (var subkey = Registry.LocalMachine.OpenSubKey(@"Software\Datadog\Datadog Agent"))
                {
                    if (subkey != null)
                    {
                        // DDAGENTUSER_NAME
                        //
                        // The user account can be provided to the installer by
                        // * The registry
                        // * The command line
                        // * The agent user dialog
                        // The user account domain and name are stored separately in the registry
                        // but are passed together on the command line and the agent user dialog.
                        // This function will combine the registry properties if they exist.
                        // Preference is given to creds provided on the command line and the agent user dialog.
                        // For UI installs it ensures that the agent user dialog is pre-populated.
                        RegistryProperty(session, "DDAGENTUSER_NAME",
                            GetRegistryPropertyHandler =>
                            {
                                var domain = subkey.GetValue("installedDomain").ToString();
                                var user = subkey.GetValue("installedUser").ToString();
                                if (!string.IsNullOrEmpty(domain) && !string.IsNullOrEmpty(user))
                                {
                                    return $"{domain}\\{user}";
                                }
                                return string.Empty;
                            });

                        // PROJECTLOCATION
                        RegistryValueProperty(session, "PROJECTLOCATION", subkey, "InstallPath");

                        // APPLICATIONDATADIRECTORY
                        RegistryValueProperty(session, "APPLICATIONDATADIRECTORY", subkey, "ConfigRoot");
                    }
                }
            }
            catch (Exception e)
            {
                session.Log($"Error processing registry properties: {e}");
                return ActionResult.Failure;
            }
            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult ReadRegistryProperties(Session session)
        {
            return ReadRegistryProperties(new SessionWrapper(session));
        }

        private static ActionResult ProcessDdAgentUserCredentials(ISession session)
        {
            try
            {
                if (!string.IsNullOrEmpty(session["DDAGENTUSER_PROCESSED_FQ_NAME"]))
                {
                  // This function has already executed successfully
                  return ActionResult.Success;
                }

                var ddAgentUserName = session["DDAGENTUSER_NAME"];

                if (string.IsNullOrEmpty(ddAgentUserName))
                {
                    // Creds are not in registry and user did not pass a value, use default account name
                    ddAgentUserName = $"{GetDefaultDomainPart()}\\ddagentuser";
                    session.Log($"No creds provided, using default {ddAgentUserName}");
                }

                // Check if user exists, and parse the full account name
                var userFound = LookupAccountName(ddAgentUserName,
                    out var userName,
                    out var domain,
                    out var securityIdentifier,
                    out var nameUse);
                var isServiceAccount = false;
                if (userFound)
                {
                    session["DDAGENTUSER_FOUND"] = "true";
                    session["DDAGENTUSER_SID"] = securityIdentifier.ToString();
                    session.Log($"Found {userName} in {domain} as {nameUse}");
                    NetIsServiceAccount(null, ddAgentUserName, out isServiceAccount);
                    session.Log($"Is {userName} in {domain} a service account: {isServiceAccount}");
                }
                else
                {
                    session["DDAGENTUSER_FOUND"] = "false";
                    session.Log($"User {ddAgentUserName} doesn't exist.");
                    ParseUserName(ddAgentUserName, out userName, out domain);
                }

                if (string.IsNullOrEmpty(domain))
                {
                    domain = GetDefaultDomainPart();
                }
                session.Log($"Installing with DDAGENTUSER_PROCESSED_NAME={userName} and DDAGENTUSER_PROCESSED_DOMAIN={domain}");
                // Create new DDAGENTUSER_PROCESSED_NAME property so we don't modify the property containing
                // the user provided value DDAGENTUSER_NAME
                session["DDAGENTUSER_PROCESSED_NAME"] = userName;
                session["DDAGENTUSER_PROCESSED_DOMAIN"] = domain;
                session["DDAGENTUSER_PROCESSED_FQ_NAME"] = $"{domain}\\{userName}";

                var ddAgentUserPassword = session["DDAGENTUSER_PASSWORD"];

                if (!isServiceAccount && string.IsNullOrEmpty(ddAgentUserPassword))
                {
                    ddAgentUserPassword = GetRandomPassword(128);
                }

                if (!string.IsNullOrEmpty(ddAgentUserPassword) && isServiceAccount)
                {
                    ddAgentUserPassword = null;
                }

                session["DDAGENTUSER_PROCESSED_PASSWORD"] = ddAgentUserPassword;
            }
            catch (Exception e)
            {
                session.Log($"Error processing ddAgentUser credentials: {e}");
                return ActionResult.Failure;
            }
            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult ProcessDdAgentUserCredentials(Session session)
        {
            return ProcessDdAgentUserCredentials(new SessionWrapper(session));
        }

        private static ActionResult ConfigureUser(ISession session)
        {
            try
            {
                SecurityIdentifier securityIdentifier;
                if (string.IsNullOrEmpty(session.Property("DDAGENTUSER_SID")))
                {
                    var ddAgentUserName = $"{session.Property("DDAGENTUSER_PROCESSED_FQ_NAME")}";
                    var userFound = LookupAccountName(ddAgentUserName,
                        out _,
                        out _,
                        out securityIdentifier,
                        out _);
                    if (!userFound)
                    {
                        session.Log($"Could not find user {ddAgentUserName}.");
                        return ActionResult.Failure;
                    }
                }
                else
                {
                    securityIdentifier = new SecurityIdentifier(session.Property("DDAGENTUSER_SID"));
                }

                securityIdentifier.AddToGroup(WellKnownSidType.BuiltinPerformanceMonitoringUsersSid);
                securityIdentifier.AddToGroup(new SecurityIdentifier("S-1-5-32-573")); // Builtin\Event Log Readers

                securityIdentifier.AddPrivilege(AccountRightsConstants.SeDenyInteractiveLogonRight);
                securityIdentifier.AddPrivilege(AccountRightsConstants.SeDenyNetworkLogonRight);
                securityIdentifier.AddPrivilege(AccountRightsConstants.SeDenyRemoteInteractiveLogonRight);
                securityIdentifier.AddPrivilege(AccountRightsConstants.SeServiceLogonRight);

                // Necessary to allow the ddagentuser to read the registry
                var key = Registry.LocalMachine.CreateSubKey("SOFTWARE\\Datadog\\Datadog Agent");
                if (key != null)
                {
                    var registrySecurity = new RegistrySecurity();
                    registrySecurity.AddAccessRule(new RegistryAccessRule(
                        new SecurityIdentifier("SY"),
                        RegistryRights.FullControl,
                        AccessControlType.Allow));
                    registrySecurity.AddAccessRule(new RegistryAccessRule(
                        new SecurityIdentifier("BA"),
                        RegistryRights.FullControl,
                        AccessControlType.Allow));
                    registrySecurity.AddAccessRule(new RegistryAccessRule(
                        securityIdentifier,
                        RegistryRights.FullControl,
                        AccessControlType.Allow));
                    registrySecurity.SetAccessRuleProtection(false, true);
                    key.SetAccessControl(registrySecurity);
                }
                else
                {
                    session.Log($"{nameof(ConfigureUser)}: Could not set registry ACLs.");
                    return ActionResult.Failure;
                }

                var files = new List<string>
                {
                    session.Property("APPLICATIONDATADIRECTORY"),
                    Path.Combine(session.Property("APPLICATIONDATADIRECTORY"), "logs"),
                    Path.Combine(session.Property("APPLICATIONDATADIRECTORY"), "logs\\agent.log"),
                    Path.Combine(session.Property("APPLICATIONDATADIRECTORY"), "conf.d"),
                    Path.Combine(session.Property("APPLICATIONDATADIRECTORY"), "auth_token"),
                    Path.Combine(session.Property("APPLICATIONDATADIRECTORY"), "datadog.yaml"),

                    Path.Combine(session.Property("PROJECTLOCATION"), "embedded2"),
                    Path.Combine(session.Property("PROJECTLOCATION"), "embedded3"),

                };
                foreach (var filePath in files)
                {
                    if (!Directory.Exists(filePath) && !File.Exists(filePath))
                    {
                        if (filePath.Contains("embedded3"))
                        {
                            throw new InvalidOperationException($"The file {filePath} doesn't exist, but it should");
                        }
                        session.Log($"{filePath} does not exists, skipping changing ACLs.");
                        continue;
                    }

                    FileSystemSecurity fileSystemSecurity;
                    string sddl;
                    try
                    {
                        if (Directory.Exists(filePath))
                        {
                            fileSystemSecurity = Directory.GetAccessControl(filePath, AccessControlSections.All);
                            sddl = $"D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;WD;;;BU)(A;OICI;FA;;;{securityIdentifier.Value})";
                        }
                        else
                        {
                            fileSystemSecurity = File.GetAccessControl(filePath, AccessControlSections.All);
                            sddl = $"D:PAI(A;;FA;;;SY)(A;;FA;;;BA)(A;;WD;;;BU)(A;;FA;;;{securityIdentifier.Value})";
                        }
                    }
                    catch (Exception e)
                    {
                        session.Log($"Failed to get ACLs on {filePath}: {e}");
                        throw;
                    }

                    session.Log($"{filePath} current ACLs: {fileSystemSecurity.GetSecurityDescriptorSddlForm(AccessControlSections.All)}");

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
                        if (Directory.Exists(filePath))
                        {
                            Directory.SetAccessControl(filePath, (DirectorySecurity)fileSystemSecurity);
                        }
                        else
                        {
                            File.SetAccessControl(filePath, (FileSecurity)fileSystemSecurity);
                        }
                    }
                    catch (Exception e)
                    {
                        try
                        {
                            // Try again but without owner/group
                            fileSystemSecurity.SetSecurityDescriptorSddlForm(sddl);
                            if (Directory.Exists(filePath))
                            {
                                Directory.SetAccessControl(filePath, (DirectorySecurity)fileSystemSecurity);
                            }
                            else
                            {
                                File.SetAccessControl(filePath, (FileSecurity)fileSystemSecurity);
                            }
                        }
                        catch (Exception)
                        {
                            session.Log($"Failed to set ACLs on {filePath}: {e}");
                            throw;
                        }
                    }

                    try
                    {
                        if (Directory.Exists(filePath))
                        {
                            fileSystemSecurity = Directory.GetAccessControl(filePath);
                        }
                        else
                        {
                            fileSystemSecurity = File.GetAccessControl(filePath);
                        }

                        session.Log($"{filePath} new ACLs: {fileSystemSecurity.GetSecurityDescriptorSddlForm(AccessControlSections.All)}");
                    }
                    catch (Exception e)
                    {
                        session.Log($"Failed to get ACLs on {filePath}: {e}");
                    }
                }

                return ActionResult.Success;
            }
            catch (Exception e)
            {
                session.Log($"Failed to configure user: {e}");
                return ActionResult.Failure;
            }
        }

        [CustomAction]
        public static ActionResult ConfigureUser(Session session)
        {
            return ConfigureUser(new SessionWrapper(session));
        }

        private static ActionResult OpenMsiLog(ISession session)
        {
            // The MessageBoxs are unlikely to ever show.
            // In testing I couldn't hit the "failed" ones. For permissions errors,
            // Notepad still opens, and Notepad shows its own error dialog box.
            // The log file should always exist because we set the MsiLogging
            // property, so unless that changes we won't see the "no log file"
            // MessageBoxs either.
            // The log file can't be deleted/renamed while the installer is running
            // because the installer has a handle to it.
            // We use MessageBoxIcon.Warning rather than MessageBoxIcon.Error
            // to match the WiX built-in error dialogs.
            var wixLogLocation = string.Empty;
            var messageBoxTitle = "Datadog Agent Setup";
            try
            {
                wixLogLocation = session["MsiLogFileLocation"];
                if (!string.IsNullOrEmpty(wixLogLocation))
                {
                    var proc = System.Diagnostics.Process.Start(wixLogLocation);
                    if (proc == null)
                    {
                        // Did not start a process
                        MessageBox.Show($"Failed to open log file: {wixLogLocation}",
                            messageBoxTitle,
                            MessageBoxButtons.OK,
                            MessageBoxIcon.Warning);
                    }
                }
                else
                {
                    // Log file path property is empty
                    MessageBox.Show("There is no log file. Please pass the /l or /log options to the installer to create a log file.",
                        messageBoxTitle,
                        MessageBoxButtons.OK,
                        MessageBoxIcon.Warning);
                }
            }
            catch (Exception e)
            {
                if (!string.IsNullOrEmpty(wixLogLocation))
                {
                    MessageBox.Show($"Failed to open log file: {wixLogLocation}\n{e.Message}",
                        messageBoxTitle,
                        MessageBoxButtons.OK,
                        MessageBoxIcon.Warning);
                }
                else
                {
                    // Log file path property is empty
                    MessageBox.Show("There is no log file. Please pass the /l or /log options to the installer to create a log file.",
                        messageBoxTitle,
                        MessageBoxButtons.OK,
                        MessageBoxIcon.Warning);
                }
                return ActionResult.Failure;
            }
            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult OpenMsiLog(Session session)
        {
            return OpenMsiLog(new SessionWrapper(session));
        }
    }
}
