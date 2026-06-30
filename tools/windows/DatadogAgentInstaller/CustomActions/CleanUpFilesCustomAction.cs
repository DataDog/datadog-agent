using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using WixToolset.Dtf.WindowsInstaller;
using System;
using System.IO;
using System.Linq;

namespace Datadog.CustomActions
{
    public class CleanUpFilesCustomAction
    {
        private const string AdpProcmgrConfigFileName = "datadog-agent-data-plane.yaml";
        private const string DdotProcmgrConfigFileName = "datadog-agent-ddot.yaml";

        private static ActionResult CleanupFiles(ISession session)
        {
            var projectLocation = session.Property("PROJECTLOCATION");

            // Post-RemoveFiles tail: drop empty processes.d and empty install root after MSI components
            // are gone (fleet prerm already removed processes.d YAML on full uninstall).
            if (session.Property("CLEANUP_TAIL") == "1")
            {
                TryRemoveEmptyInstallDir(session, projectLocation);
                return ActionResult.Success;
            }

            // Fresh-install rollback: fleet postinst may have written processes.d YAML that MSI does
            // not track. Upgrade rollback passes UPGRADINGPRODUCTCODE so prerm-retained yaml is kept.
            if (session.Property("ROLLBACK") == "1" && string.IsNullOrEmpty(session.Property("UPGRADINGPRODUCTCODE")))
            {
                TryRemoveFleetProcmgrConfigFiles(session, projectLocation);
            }

            var toDelete = new[]
            {
                // may contain python files created outside of install
                Path.Combine(projectLocation, "embedded2"),
                Path.Combine(projectLocation, "embedded3"),
                Path.Combine(projectLocation, "python-scripts"),
            }
            // installation specific files
            .Concat(session.GeneratedPaths());

            foreach (var path in toDelete)
            {
                try
                {
                    if (Directory.Exists(path))
                    {
                        session.Log($"Deleting directory \"{path}\"");
                        Directory.Delete(path, true);
                    }
                    else if (File.Exists(path))
                    {
                        session.Log($"Deleting file \"{path}\"");
                        File.Delete(path);
                    }
                    else
                    {
                        session.Log($"{path} not found, skip deletion.");
                    }
                }
                catch (Exception e)
                {
                    session.Log($"Error while deleting file: {e}");
                    // Don't fail in cleanup/rollback actions otherwise
                    // we may brick the installation.
                }
            }

            if (session.Property("ROLLBACK") == "1")
            {
                TryRemoveEmptyInstallDir(session, projectLocation);
            }

            return ActionResult.Success;
        }

        private static void TryRemoveFleetProcmgrConfigFiles(ISession session, string projectLocation)
        {
            if (string.IsNullOrEmpty(projectLocation))
            {
                return;
            }

            var processesDir = Path.Combine(projectLocation, "processes.d");
            foreach (var fileName in new[] { AdpProcmgrConfigFileName, DdotProcmgrConfigFileName })
            {
                var path = Path.Combine(processesDir, fileName);
                try
                {
                    if (!File.Exists(path))
                    {
                        session.Log($"{path} not found, skip deletion.");
                        continue;
                    }

                    session.Log($"Deleting fleet process manager config \"{path}\"");
                    File.Delete(path);
                }
                catch (Exception e)
                {
                    session.Log($"Error while deleting fleet process manager config {path}: {e}");
                }
            }
        }

        private static void TryRemoveEmptyInstallDir(ISession session, string projectLocation)
        {
            if (string.IsNullOrEmpty(projectLocation))
            {
                return;
            }

            try
            {
                if (!Directory.Exists(projectLocation))
                {
                    return;
                }

                var processesDir = Path.Combine(projectLocation, "processes.d");
                if (Directory.Exists(processesDir) && !Directory.EnumerateFileSystemEntries(processesDir).Any())
                {
                    session.Log($"Deleting empty directory \"{processesDir}\"");
                    Directory.Delete(processesDir);
                }

                if (Directory.EnumerateFileSystemEntries(projectLocation).Any())
                {
                    session.Log($"{projectLocation} is not empty, skip deletion.");
                    return;
                }

                session.Log($"Deleting empty install directory \"{projectLocation}\"");
                Directory.Delete(projectLocation);
            }
            catch (Exception e)
            {
                session.Log($"Error while deleting empty install directory: {e}");
            }
        }

        public static ActionResult CleanupFiles(Session session)
        {
            return CleanupFiles(new SessionWrapper(session));
        }
    }
}
