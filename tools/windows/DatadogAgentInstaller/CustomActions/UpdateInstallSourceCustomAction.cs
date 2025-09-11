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
        public static ActionResult UpdateInstallSource(ISession session)
        {
            var msiPath = session.Property("DATABASE");
            // get installer path
            var installerPath = session.Property("PROJECTLOCATION");
            installerPath = Path.Combine(installerPath, "bin", "datadog-installer.exe");

            // check if this is a fleet install
            // fleet install runs the MSI from the package dir so the install source is already set
            var fleetInstall = session.Property("FLEET_INSTALL");
            if (!string.IsNullOrEmpty(fleetInstall) && fleetInstall == "1")
            {
                session.Log("Skipping install source update as this is a FLEET install.");
                return ActionResult.Success;
            }

            // check if this is a FIPS install
            var agentFlavor = session.Property("AgentFlavor");
            if (!string.IsNullOrEmpty(agentFlavor) && agentFlavor == Constants.FipsFlavor)
            {
                session.Log("Skipping install source update as this is a FIPS install.");
                return ActionResult.Success;
            }

            // verify that msiPath is a valid path
            if (!File.Exists(msiPath))
            {
                session.Log($"msiPath does not exist: {msiPath}");
                return ActionResult.Failure;
            }

            // run the package-command datadog-agent updateRegistryInstallSource
            using (var proc = session.RunCommand(installerPath, $"package-command datadog-agent updateRegistryInstallSource"))
            {
                if (proc.ExitCode != 0)
                {
                    session.Log($"install script exited with code: {proc.ExitCode}");
                    return ActionResult.Failure;
                }
                return ActionResult.Success;
            }
        }

        public static ActionResult UpdateInstallSource(Session session)
        {
            return UpdateInstallSource(new SessionWrapper(session));
        }
    }
}
