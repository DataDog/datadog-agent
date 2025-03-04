using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;

namespace Datadog.CustomActions
{
    public static class InstallOciPackages
    {
        private static ActionResult InstallPackages(ISession session)
        {
            try
            {
                string instrumentationEnabled = session.Property("DD_APM_INSTRUMENTATION_ENABLED");
                string libraries = session.Property("DD_APM_INSTRUMENTATION_LIBRARIES");
                if (libraries != "dotnet" || instrumentationEnabled != "iis")
                {
                    session.Log("Skipping dotnet library installation");
                    return ActionResult.Success;
                }
                string installDir = session.Property("INSTALLDIR");
                // TODO remove me
                installDir = "C:\\Program Files\\Datadog\\Datadog Installer";
                string exePath = System.IO.Path.Combine(installDir, "datadog-installer.exe");
                session.Log("Installing dotnet library");
                session.Log($"installer executable path: {exePath}");
                // TODO: Replace the version and read from disk
                session.RunCommand(exePath, "install install.datad0g.com/apm-library-dotnet-package:3.12");
                // TODO: Should we use RollbackDataStore to store the rollback instructions
                return ActionResult.Success;
            }
            catch (Exception ex)
            {
                session.Log("Error while installing dotnet library: " + ex.Message);
                return ActionResult.Failure;
            }
        }

        public static ActionResult InstallPackages(Session session)
        {
            return InstallPackages(new SessionWrapper(session));
        }
    }
}
