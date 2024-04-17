using System;
using System.Diagnostics;
using System.Security.Principal;
using Datadog.CustomActions.Interfaces;
using Microsoft.Deployment.WindowsInstaller;

namespace Datadog.CustomActions
{
    public class PrerequisitesCustomActions
    {
        const string Error = "The Datadog Agent installer must be run by a user that is a member of the Administrator group.";

        [CustomAction]
        public static ActionResult EnsureAdminCaller(Session session)
        {
            if (!new WindowsPrincipal(WindowsIdentity.GetCurrent()).IsInRole(WindowsBuiltInRole.Administrator))
            {
                ((ISession)new SessionWrapper(session)).Log(Error);
                if (int.Parse(session["UILevel"]) > 3)
                {
                    try
                    {
                        // Skip the fatal error dialog and run the installer again as an administrator
                        session["SKIP_ERROR_DIALOG"] = "1";

                        var startInfo = new ProcessStartInfo
                        {
                            UseShellExecute = true,
                            WorkingDirectory = Environment.CurrentDirectory,
                            FileName = "msiexec.exe",
                            Arguments = "/i \"" + session["OriginalDatabase"] + "\"",
                            Verb = "runas"
                        };

                        Process.Start(startInfo);
                    }
                    catch
                    {
                        // ignored
                    }
                }

                return ActionResult.Failure;
            }

            return ActionResult.Success;
        }
    }
}
