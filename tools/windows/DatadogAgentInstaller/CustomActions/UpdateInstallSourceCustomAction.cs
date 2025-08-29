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
    public class UpdateInstallSourceCustomAction
    {
        /// <summary>
        /// Update the install source on non fleet deployments of MSI.
        /// </summary>
        /// <remarks>
        /// This function will call the updateRegistryInstallSource function of the
        // installer.
        /// </remarks>
        /// <param name="s">The session object.</param>
        /// <returns><see cref="ActionResult.Success"/></returns>
        public static ActionResult UpdateInstallSource(Session s)
        {
            ISession session = new SessionWrapper(s);
            var msiPath = session.Property("DATABASE");
            // get installer path
            var installerPath = session.Property("PROJECTLOCATION");
            installerPath = Path.Combine(installerPath, "bin", "datadog-installer.exe");

            // check if this is a fleet install
            var fleetInstall = session.Property("FLEET_INSTALL");
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

            // run the package-command datadog-agent updateRegistryInstallSource
            var proc = session.RunCommand(installerPath, $"package-command datadog-agent updateRegistryInstallSource");

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
