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
        private const string ParProcmgrConfigFileName = "datadog-agent-action.yaml";

        /// <summary>
        /// Removes generated install artifacts (embedded2/embedded3, python-scripts, session-generated paths).
        /// Used before InstallFiles, during uninstall, and on rollback.
        /// </summary>
        public static ActionResult CleanupFiles(Session session)
        {
            return CleanupFiles(new SessionWrapper(session));
        }

        /// <summary>
        /// Rollback-only: remove fleet-written processes.d YAML that MSI does not track.
        /// Scheduled only on fresh install rollbacks (see Conditions.FirstInstall on the WiX action).
        /// </summary>
        public static ActionResult RemoveFleetProcmgrConfigOnRollback(Session session)
        {
            return RemoveFleetProcmgrConfigOnRollback(new SessionWrapper(session));
        }

        /// <summary>
        /// Rollback-only: drop an otherwise-empty install root left by a failed fresh install.
        /// </summary>
        public static ActionResult RemoveEmptyInstallDirOnRollback(Session session)
        {
            return RemoveEmptyInstallDirOnRollback(new SessionWrapper(session));
        }

        /// <summary>
        /// Uninstall tail: drop empty processes.d and empty install root after MSI components are removed.
        /// Fleet prerm already removed processes.d YAML on full uninstall.
        /// </summary>
        public static ActionResult RemoveEmptyInstallDirAfterUninstall(Session session)
        {
            return RemoveEmptyInstallDirAfterUninstall(new SessionWrapper(session));
        }

        private static ActionResult CleanupFiles(ISession session)
        {
            RemoveGeneratedArtifactPaths(session, session.Property("PROJECTLOCATION"));
            return ActionResult.Success;
        }

        private static ActionResult RemoveFleetProcmgrConfigOnRollback(ISession session)
        {
            TryRemoveFleetProcmgrConfigFiles(session, session.Property("PROJECTLOCATION"));
            return ActionResult.Success;
        }

        private static ActionResult RemoveEmptyInstallDirOnRollback(ISession session)
        {
            TryRemoveEmptyInstallDir(session, session.Property("PROJECTLOCATION"));
            return ActionResult.Success;
        }

        private static ActionResult RemoveEmptyInstallDirAfterUninstall(ISession session)
        {
            TryRemoveEmptyInstallDir(session, session.Property("PROJECTLOCATION"));
            return ActionResult.Success;
        }

        private static void RemoveGeneratedArtifactPaths(ISession session, string projectLocation)
        {
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
        }

        private static void TryRemoveFleetProcmgrConfigFiles(ISession session, string projectLocation)
        {
            if (string.IsNullOrEmpty(projectLocation))
            {
                return;
            }

            var processesDir = Path.Combine(projectLocation, "processes.d");
            foreach (var fileName in new[] { AdpProcmgrConfigFileName, DdotProcmgrConfigFileName, ParProcmgrConfigFileName })
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
    }
}
