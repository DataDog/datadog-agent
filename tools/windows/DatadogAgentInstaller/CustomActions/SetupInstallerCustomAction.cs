using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Datadog.CustomActions.Rollback;
using Microsoft.Deployment.WindowsInstaller;
using System;
using System.Diagnostics;
using System.IO;

namespace Datadog.CustomActions
{
    public class SetupInstallerCustomAction
    {
        /// <summary>
        /// Setup the Installer on non fleet deployments of MSI.
        /// </summary>
        /// <remarks>
        /// This function will call the setup-installer function of the
        // installer.
        /// </remarks>
        /// <param name="s">The session object.</param>
        /// <returns><see cref="ActionResult.Success"/></returns>
        public static ActionResult SetupInstaller(Session s)
        {
            ISession session = new SessionWrapper(s);
            var fleetInstall = session.Property("FLEET_INSTALL");
            var msiPath = session.Property("OriginalDatabase");
            if (!string.IsNullOrEmpty(fleetInstall) && fleetInstall == "1")
            {
                session.Log("Skipping installer setup as this is a FLEET install.");
                return ActionResult.Success;
            }
            // verify that msiPath is a valid path
            if (!File.Exists(msiPath))
            {
                session.Log($"msiPath does not exist: {msiPath}");
                return ActionResult.Failure;
            }
            var installerPath = "C:\\Program Files\\Datadog\\Datadog Installer\\datadog-installer.exe";

            // run the setup on the installer
            var proc = session.RunCommand(installerPath, $"setup-installer \"{msiPath}\"");

            if (proc.ExitCode != 0)
            {
                session.Log($"install script exited with code: {proc.ExitCode}");
                proc.Close();
                return ActionResult.Failure;
            }
            proc.Close();
            return ActionResult.Success;
        }
    }
}
