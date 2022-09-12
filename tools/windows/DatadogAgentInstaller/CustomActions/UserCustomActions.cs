using System;
using System.Security.Cryptography;
using Datadog.CustomActions.Native;
using Microsoft.Deployment.WindowsInstaller;

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
                string ddAgentUserName = session["DDAGENTUSER_NAME"];
                string userName, domain;
                NativeMethods.SID_NAME_USE nameUse;
                bool userFound = NativeMethods.LookupAccountName(ddAgentUserName,
                    out userName,
                    out domain,
                    out nameUse);
                bool isServiceAccount = false;
                if (userFound)
                {
                    session["DDAGENTUSER_FOUND"] = "true";
                    NativeMethods.NetIsServiceAccount(null, ddAgentUserName, out isServiceAccount);
                }
                else
                {
                    session["DDAGENTUSER_FOUND"] = "false";
                    NativeMethods.ParseUserName(ddAgentUserName, out userName, out domain);
                }

                session["DDAGENTUSER_NAME"] = userName;
                session["DDAGENTUSER_DOMAIN"] = domain;

                string ddAgentUserPassword = session["DDAGENTUSER_PASSWORD"];

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
    }
}
