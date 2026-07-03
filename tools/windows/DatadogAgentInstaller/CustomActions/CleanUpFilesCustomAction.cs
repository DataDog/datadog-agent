using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using WixToolset.Dtf.WindowsInstaller;
using System;
using System.IO;
using System.Linq;
using Microsoft.Win32;

namespace Datadog.CustomActions
{
    public class CleanUpFilesCustomAction
    {
        private const string AdpProcmgrConfigFileName = "datadog-agent-data-plane.yaml";
        private const string DdotProcmgrConfigFileName = "datadog-agent-ddot.yaml";
        private const string ParProcmgrConfigFileName = "datadog-agent-action.yaml";
        private const string ProcessProcmgrConfigFileName = "datadog-agent-process.yaml";

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
        /// Rollback-only: remove fleet-written PAR processes.d YAML after a failed upgrade.
        /// Older agents start PAR via SCM and do not suppress it when this file is left behind.
        /// </summary>
        public static ActionResult RemoveParFleetProcmgrConfigOnUpgradeRollback(Session session)
        {
            return RemoveParFleetProcmgrConfigOnUpgradeRollback(new SessionWrapper(session));
        }

        /// <summary>
        /// Rollback-only: remove fleet-written process-agent processes.d YAML after a failed upgrade.
        /// Older agents start process-agent via SCM and do not suppress it when this file is left behind.
        /// </summary>
        public static ActionResult RemoveProcessFleetProcmgrConfigOnUpgradeRollback(Session session)
        {
            return RemoveProcessFleetProcmgrConfigOnUpgradeRollback(new SessionWrapper(session));
        }

        /// <summary>
        /// Clear the install-session marker after a successful MSI install/upgrade.
        /// </summary>
        public static ActionResult ClearPARProcmgrConfigWrittenThisInstallMarker(Session session)
        {
            return ClearPARProcmgrConfigWrittenThisInstallMarker(new SessionWrapper(session));
        }

        /// <summary>
        /// Clear the process-agent install-session marker after a successful MSI install/upgrade.
        /// </summary>
        public static ActionResult ClearProcessProcmgrConfigWrittenThisInstallMarker(Session session)
        {
            return ClearProcessProcmgrConfigWrittenThisInstallMarker(new SessionWrapper(session));
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
            TryRemoveFleetProcmgrConfigFiles(session, session.Property("PROJECTLOCATION"),
                AdpProcmgrConfigFileName, DdotProcmgrConfigFileName, ParProcmgrConfigFileName, ProcessProcmgrConfigFileName);
            return ActionResult.Success;
        }

        private static ActionResult RemoveParFleetProcmgrConfigOnUpgradeRollback(ISession session)
        {
            if (!PARProcmgrConfigMarkedWrittenThisInstall(session))
            {
                session.Log("PAR processes.d was not written this install; skip upgrade rollback cleanup.");
                return ActionResult.Success;
            }
            TryRemoveFleetProcmgrConfigFiles(session, session.Property("PROJECTLOCATION"), ParProcmgrConfigFileName);
            clearParProcmgrInstallMarkerRegistry(session);
            return ActionResult.Success;
        }

        private static ActionResult RemoveProcessFleetProcmgrConfigOnUpgradeRollback(ISession session)
        {
            if (!ProcessProcmgrConfigMarkedWrittenThisInstall(session))
            {
                session.Log("process-agent processes.d was not written this install; skip upgrade rollback cleanup.");
                return ActionResult.Success;
            }
            TryRemoveFleetProcmgrConfigFiles(session, session.Property("PROJECTLOCATION"), ProcessProcmgrConfigFileName);
            clearProcessProcmgrInstallMarkerRegistry(session);
            return ActionResult.Success;
        }

        private static ActionResult ClearPARProcmgrConfigWrittenThisInstallMarker(ISession session)
        {
            clearParProcmgrInstallMarkerRegistry(session);
            return ActionResult.Success;
        }

        private static ActionResult ClearProcessProcmgrConfigWrittenThisInstallMarker(ISession session)
        {
            clearProcessProcmgrInstallMarkerRegistry(session);
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

        private static void TryRemoveFleetProcmgrConfigFiles(ISession session, string projectLocation, params string[] fileNames)
        {
            if (string.IsNullOrEmpty(projectLocation))
            {
                return;
            }

            var processesDir = Path.Combine(projectLocation, "processes.d");
            foreach (var fileName in fileNames)
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

        private static bool PARProcmgrConfigMarkedWrittenThisInstall(ISession session)
        {
            try
            {
                using var key = Registry.LocalMachine.OpenSubKey(Constants.DatadogAgentRegistryKey, writable: false);
                if (key == null)
                {
                    return false;
                }

                var value = key.GetValue(Constants.PARProcmgrConfigWrittenThisInstallValue);
                return value is int marker && marker != 0;
            }
            catch (Exception e)
            {
                session.Log($"Error reading PAR procmgr install marker: {e}");
                return false;
            }
        }

        private static void clearParProcmgrInstallMarkerRegistry(ISession session)
        {
            try
            {
                using var key = Registry.LocalMachine.OpenSubKey(Constants.DatadogAgentRegistryKey, writable: true);
                key?.DeleteValue(Constants.PARProcmgrConfigWrittenThisInstallValue, throwOnMissingValue: false);
            }
            catch (Exception e)
            {
                session.Log($"Error clearing PAR procmgr install marker: {e}");
            }
        }

        private static bool ProcessProcmgrConfigMarkedWrittenThisInstall(ISession session)
        {
            try
            {
                using var key = Registry.LocalMachine.OpenSubKey(Constants.DatadogAgentRegistryKey, writable: false);
                if (key == null)
                {
                    return false;
                }

                var value = key.GetValue(Constants.ProcessProcmgrConfigWrittenThisInstallValue);
                return value is int marker && marker != 0;
            }
            catch (Exception e)
            {
                session.Log($"Error reading process-agent procmgr install marker: {e}");
                return false;
            }
        }

        private static void clearProcessProcmgrInstallMarkerRegistry(ISession session)
        {
            try
            {
                using var key = Registry.LocalMachine.OpenSubKey(Constants.DatadogAgentRegistryKey, writable: true);
                key?.DeleteValue(Constants.ProcessProcmgrConfigWrittenThisInstallValue, throwOnMissingValue: false);
            }
            catch (Exception e)
            {
                session.Log($"Error clearing process-agent procmgr install marker: {e}");
            }
        }
    }
}
