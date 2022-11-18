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
    /// <summary>
    /// Fetch and process registry value(s) and return a string to be assigned to a WIX property.
    /// </summary>
    using GetRegistryPropertyHandler = Func<ISession, string>;

    public class UserCustomActions
    {
        public static string GetRandomPassword(int length)
        {
            byte[] rgb = new byte[length];
            RNGCryptoServiceProvider rngCrypt = new RNGCryptoServiceProvider();
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
        private static void RegistryValueProperty(ISession session, string propertyName, Microsoft.Win32.RegistryKey registryKey, string registryValue)
        {
            RegistryProperty(session, propertyName,
                GetRegistryPropertyHandler =>
                {
                    return registryKey.GetValue(registryValue).ToString();
                });
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
                using (var subkey = Microsoft.Win32.Registry.LocalMachine.OpenSubKey(@"Software\Datadog\Datadog Agent"))
                {
                    if (subkey != null)
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
                session.Log($"Installing with DDAGENTUSER_PROCESSED_NAME={userName} and DDAGENTUSER_PROCESSED_DOMAIN={domain}");
                // Create new DDAGENTUSER_PROCESSED_NAME property so we don't modify the property containing
                // the user provided value DDAGENTUSER_NAME
                session["DDAGENTUSER_PROCESSED_NAME"] = userName;
                session["DDAGENTUSER_PROCESSED_DOMAIN"] = domain;
                session["DDAGENTUSER_PROCESSED_FQ_NAME"] = $"{domain}\\{userName}";

                var ddAgentUserPassword = session["DDAGENTUSER_PASSWORD"];

                if (!userFound && string.IsNullOrEmpty(ddAgentUserPassword) && !isServiceAccount)
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
