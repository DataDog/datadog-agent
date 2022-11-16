using System;
using System.DirectoryServices.ActiveDirectory;
using System.Security.Cryptography;
using System.Security.Principal;
using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Native;
using Microsoft.Deployment.WindowsInstaller;
using static Datadog.CustomActions.Native.NativeMethods;

namespace Datadog.CustomActions
{
    public class UserCustomActions
    {
        public static string GetRandomPassword(int length)
        {
            byte[] rgb = new byte[length];
            RNGCryptoServiceProvider rngCrypt = new RNGCryptoServiceProvider();
            rngCrypt.GetBytes(rgb);
            return Convert.ToBase64String(rgb);
        }

        private static string GetDefaultDomainPart()
        {
            // We default to creating a local account if the domain
            // part is not specified in DDAGENTUSER_NAME.
            // However, domain controllers do not have local accounts, so we must
            // default to a domain account.
            // We still want to default to local accounts for domain clients
            // though, so we must also check if this computer is a domain controller
            // for this domain.
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

        private static ActionResult ReadRegistryProperties(ISession session)
        {
            // This CA runs (only once) in either the InstallUISequence or the InstallExecuteSequence.
            // For UI installs it ensures that the agent user dialog is pre-populated with registry
            // creds if applicable.
            try
            {
                // DDAGENTUSER_NAME
                //
                // The user account can be provided to the installer by
                // * The registry
                // * The command line
                // * The agent user dialog
                // The user account domain and name are stored separetely in the registry
                // but are passed together on the command line and the agent user dialog.
                // This function will combine the registry properties if they exist.
                // Preference is given to creds provided on the command line and the agent user dialog.
                if (string.IsNullOrEmpty(session["DDAGENTUSER_NAME"]))
                {
                    try
                    {
                        var domain = Microsoft.Win32.Registry.LocalMachine.OpenSubKey(@"Software\Datadog\Datadog Agent")
                            .GetValue("installedDomain")
                            .ToString();
                        var user = Microsoft.Win32.Registry.LocalMachine.OpenSubKey(@"Software\Datadog\Datadog Agent")
                            .GetValue("installedUser")
                            .ToString();
                        if (!string.IsNullOrEmpty(domain) && !string.IsNullOrEmpty(user)) {
                            session["DDAGENTUSER_NAME"] = $"{domain}\\{user}";
                            session.Log($"Found creds in registry {session["DDAGENTUSER_NAME"]}");
                        }
                    }
                    catch { }
                }
                else
                {
                    session.Log($"User provided DDAGENTUSER_NAME {session["DDAGENTUSER_NAME"]}");
                }

                // PROJECTLOCATION
                //
                if (string.IsNullOrEmpty(session["PROJECTLOCATION"]))
                {
                    try
                    {
                        session["PROJECTLOCATION"] = Microsoft.Win32.Registry.LocalMachine.OpenSubKey(@"Software\Datadog\Datadog Agent")
                            .GetValue("InstallPath")
                            .ToString();
                        session.Log($"Found PROJECTLOCATION in registry {session["PROJECTLOCATION"]}");
                    }
                    catch {  }
                }
                else
                {
                    session.Log($"User provided PROJECTLOCATION {session["PROJECTLOCATION"]}");
                }

                // APPLICATIONDATADIRECTORY
                //
                if (string.IsNullOrEmpty(session["APPLICATIONDATADIRECTORY"]))
                {
                    try
                    {
                        session["APPLICATIONDATADIRECTORY"] = Microsoft.Win32.Registry.LocalMachine.OpenSubKey(@"Software\Datadog\Datadog Agent")
                            .GetValue("ConfigRoot")
                            .ToString();
                        session.Log($"Found APPLICATIONDATADIRECTORY in registry {session["APPLICATIONDATADIRECTORY"]}");
                    }
                    catch {  }
                }
                else
                {
                    session.Log($"User provided APPLICATIONDATADIRECTORY {session["APPLICATIONDATADIRECTORY"]}");
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
                if (!string.IsNullOrEmpty(session["DDAGENTUSER_FQ_NAME"]))
                {
                  // This function has already executed succesfully
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
                session.Log($"Installing with DDAGENTUSER_USERNAME={userName} and DDAGENTUSER_DOMAIN={domain}");
                session["DDAGENTUSER_USERNAME"] = userName;
                session["DDAGENTUSER_DOMAIN"] = domain;
                session["DDAGENTUSER_FQ_NAME"] = $"{domain}\\{userName}";

                var ddAgentUserPassword = session["DDAGENTUSER_PASSWORD"];

                if (!userFound && string.IsNullOrEmpty(ddAgentUserPassword) && !isServiceAccount)
                {
                    ddAgentUserPassword = GetRandomPassword(128);
                }

                if (!string.IsNullOrEmpty(ddAgentUserPassword) && isServiceAccount)
                {
                    ddAgentUserPassword = null;
                }

                session["DDAGENTUSER_PASSWORD"] = ddAgentUserPassword;
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
                    var ddAgentUserName = $"{session.Property("DDAGENTUSER_FQ_NAME")}";
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

                securityIdentifier.AddToGroup("Performance Monitor Users");
                securityIdentifier.AddToGroup("Event Log Readers");

                securityIdentifier.AddPrivilege(AccountRightsConstants.SeDenyInteractiveLogonRight);
                securityIdentifier.AddPrivilege(AccountRightsConstants.SeDenyNetworkLogonRight);
                securityIdentifier.AddPrivilege(AccountRightsConstants.SeDenyRemoteInteractiveLogonRight);
                securityIdentifier.AddPrivilege(AccountRightsConstants.SeServiceLogonRight);

                /*
                var key = Microsoft.Win32.Registry.LocalMachine.CreateSubKey("SOFTWARE\\Datadog\\Datadog Agent");
                if (key != null)
                {
                    key.GetAccessControl()
                        .AddAccessRule(new RegistryAccessRule(
                            securityIdentifier,
                            RegistryRights.WriteKey |
                            RegistryRights.ReadKey |
                            RegistryRights.Delete |
                            RegistryRights.FullControl,
                            AccessControlType.Allow));
                }
                else
                {
                    session.Log($"{nameof(ConfigureUser)}: Could not set registry ACLs.");
                    return ActionResult.Failure;
                }
                */
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
    }
}
