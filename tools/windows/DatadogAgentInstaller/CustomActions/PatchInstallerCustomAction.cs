using System;
using Datadog.CustomActions.Interfaces;
using Microsoft.Deployment.WindowsInstaller;
using Microsoft.Win32;

namespace Datadog.CustomActions
{
    public class PatchInstallerCustomAction
    {
        /// <summary>
        /// Patch the previous install to make the upgrade work.
        /// </summary>
        /// <remarks>
        /// This function is used to make any changes necessary
        /// to the system during an upgrade to ensure it goes smoothly.
        /// </remarks>
        /// <param name="s">The session object.</param>
        /// <returns><see cref="ActionResult.Success"/></returns>
        [CustomAction]
        public static ActionResult Patch(Session s)
        {
            ISession session = new SessionWrapper(s);
            try
            {
                // The previous installer left this key around
                // which causes both the services and the user to be deleted on rollback.
                // So delete it now to ensure rollbacks work smoothly.
                Registry.LocalMachine.DeleteSubKey(@"Software\Datadog\Datadog Agent\installRollback");
            }
            catch (Exception e)
            {
                // Don't need full stack trace
                session.Log($"Cannot patch previous installation: {e.Message}");
            }

            return ActionResult.Success;
        }
    }
}
