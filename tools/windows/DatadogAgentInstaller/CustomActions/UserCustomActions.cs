using System;
using System.DirectoryServices;
using System.Security.Cryptography;
using System.Security.Principal;
using System.Text;
using System.Windows.Forms;
using CustomActions.Extensions;
using CustomActions.Native;
using Microsoft.Deployment.WindowsInstaller;

namespace CustomActions
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

#if false
            try
            {
                NTAccount f = new NTAccount(domain, userName);
                SecurityIdentifier s = (SecurityIdentifier)f.Translate(typeof(SecurityIdentifier));
                String sidString = s.ToString();
            }
            catch (IdentityNotMappedException)
            {
                // User not found
            }

            DirectoryEntry ad = new DirectoryEntry("WinNT://" + Environment.MachineName + ",computer");
            var user = ad.Children.Find(userName);
            if (user == null)
            {
                DirectoryEntry newUser = ad.Children.Add(userName, "user");
                newUser.Invoke("SetPassword", ddAgentUserPassword);
                newUser.Invoke("Put", "Description", "Test User from .NET");
                newUser.CommitChanges();
            }

            DirectoryEntry grp;

            grp = ad.Children.Find("Guests", "group");
            if (grp != null) { grp.Invoke("Add", new object[] { user.Path.ToString() }); }
#endif
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
