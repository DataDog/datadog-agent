using System;
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

        private static ActionResult ProcessDdAgentUserCredentials(ISession session)
        {
            try
            {
                var ddAgentUserName = session["DDAGENTUSER_NAME"];
                if (string.IsNullOrEmpty(ddAgentUserName))
                {
                    ddAgentUserName = $"{Environment.MachineName}\\ddagentuser";
                }
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
                }
                else
                {
                    session["DDAGENTUSER_FOUND"] = "false";
                    session.Log($"User {ddAgentUserName} doesn't exist.");
                    ParseUserName(ddAgentUserName, out userName, out domain);
                }

                session.Log($"Installing with DDAGENTUSER_NAME={userName} and DDAGENTUSER_DOMAIN={domain}");
                session["DDAGENTUSER_NAME"] = userName;
                session["DDAGENTUSER_DOMAIN"] = domain;

                var ddAgentUserPassword = session["DDAGENTUSER_PASSWORD"];

                if (userFound && string.IsNullOrEmpty(ddAgentUserPassword) && !isServiceAccount)
                {
                    // Impossible to use an existing user that is not a service account without a password
                    session.Log($"Provide a password for the user {ddAgentUserName}");
                    return ActionResult.Failure;
                }

                if (string.IsNullOrEmpty(ddAgentUserPassword) && !isServiceAccount)
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
                    var ddAgentUserName = $"{session.Property("DDAGENTUSER_DOMAIN")}\\{session.Property("DDAGENTUSER_NAME")}";
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
