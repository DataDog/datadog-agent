using System;
using System.IO;
using System.Reflection;
using Datadog.CustomActions.Extensions;
using Microsoft.Deployment.WindowsInstaller;

namespace Datadog.CustomActions.Interfaces
{
    public class InstallInfoCustomActions
    {
        private static ActionResult WriteInstallInfo(ISession session)
        {
            var configFolder = session.Property("APPLICATIONDATADIRECTORY");
            var installMethod = session.Property("OVERRIDE_INSTALLATION_METHOD");
            var installInfo = Path.Combine(configFolder, "install_info");
            try
            {
                using var output = new StreamWriter(installInfo);
                if (string.IsNullOrEmpty(installMethod))
                {
                    if (int.Parse(session.Property("UILevel")) > 2)
                    {
                        installMethod = "windows_msi_gui";
                    }
                    else
                    {
                        installMethod = "windows_msi_quiet";
                    }

                }

                // Leave "windows_msi_next_gen" to leave a trace that
                // some other tool used the next gen installer.
                output.Write($@"---
install_method:
  tool: {installMethod}
  tool_version: windows_msi_next_gen_{Assembly.GetExecutingAssembly().GetName().Version}
  installer_version: {CiInfo.PackageVersion}
");
            }
            catch (Exception e)
            {
                session.Log($"Could not write install info: {e}");
                return ActionResult.Failure;
            }
            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult WriteInstallInfo(Session session)
        {
            return WriteInstallInfo(new SessionWrapper(session));
        }
    }
}
