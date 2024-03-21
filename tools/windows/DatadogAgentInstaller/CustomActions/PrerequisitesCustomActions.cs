using System;
using System.Security.Principal;
using System.Windows.Forms;
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
                        MessageBox.Show(Error, @"Privileges exception",
                            MessageBoxButtons.OK);
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
