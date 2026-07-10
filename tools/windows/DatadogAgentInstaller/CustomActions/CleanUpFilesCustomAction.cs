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
        /// <summary>
        /// Removes generated install artifacts (embedded2/embedded3, python-scripts, session-generated paths).
        /// Used before InstallFiles, during uninstall, and on rollback.
        /// </summary>
        public static ActionResult CleanupFiles(Session session)
        {
            return CleanupFiles(new SessionWrapper(session));
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
                // fleet postinst writes processes.d/*.yaml (untracked by MSI); RemoveFolderEx owns
                // uninstall, this covers install/upgrade/repair rollback and the repair pre-clean.
                Path.Combine(projectLocation, "processes.d"),
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
