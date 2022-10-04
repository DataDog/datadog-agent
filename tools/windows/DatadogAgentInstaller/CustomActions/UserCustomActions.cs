using System;
using System.Diagnostics;
using System.Security.AccessControl;
using System.Security.Cryptography;
using System.Security.Principal;
using Datadog.CustomActions.Extensions;
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

        [CustomAction]
        public static ActionResult ProcessDdAgentUserCredentials(Session session)
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
                    session.Log($"{nameof(ProcessDdAgentUserCredentials)}: Found {userName} in {domain} as {nameUse}");
                    NetIsServiceAccount(null, ddAgentUserName, out isServiceAccount);
                }
                else
                {
                    session["DDAGENTUSER_FOUND"] = "false";
                    session.Log($"{nameof(ProcessDdAgentUserCredentials)}: User {ddAgentUserName} doesn't exist.");
                    ParseUserName(ddAgentUserName, out userName, out domain);
                }

                session.Log($"Installing with DDAGENTUSER_NAME={userName} and DDAGENTUSER_DOMAIN={domain}");
                session["DDAGENTUSER_NAME"] = userName;
                session["DDAGENTUSER_DOMAIN"] = domain;

                var ddAgentUserPassword = session["DDAGENTUSER_PASSWORD"];

                if (userFound && string.IsNullOrEmpty(ddAgentUserPassword) && !isServiceAccount)
                {
                    // Impossible to use an existing user that is not a service account without a password
                    session.Log($"{nameof(ProcessDdAgentUserCredentials)}: Provide a password for the user {ddAgentUserName}");
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
                session.Log($"{nameof(ProcessDdAgentUserCredentials)}: Error processing ddAgentUser credentials: {e}");
                return ActionResult.Failure;
            }
            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult ConfigureUser(Session session)
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
                    session.Log($"{nameof(ConfigureUser)}: Could not find user {ddAgentUserName}.");
                    return ActionResult.Failure;
                }
            }
            else
            {
                securityIdentifier = new SecurityIdentifier(session.Property("DDAGENTUSER_SID"));
            }

            AddUserToGroup(securityIdentifier, "Performance Monitor Users");
            AddUserToGroup(securityIdentifier, "Event Log Readers");

            AddPrivilege(securityIdentifier, "SeDenyInteractiveLogonRight");
            AddPrivilege(securityIdentifier, "SeDenyNetworkLogonRight");
            AddPrivilege(securityIdentifier, "SeDenyRemoteInteractiveLogonRight");
            AddPrivilege(securityIdentifier, "SeServiceLogonRight");

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
    }
}
