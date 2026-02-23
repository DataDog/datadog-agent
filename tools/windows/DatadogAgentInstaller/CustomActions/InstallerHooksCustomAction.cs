using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using WixToolset.Dtf.WindowsInstaller;
using System;
using System.Collections.Generic;
using System.IO;

namespace Datadog.CustomActions
{
    /// <summary>
    /// MSI custom actions that call the datadog-installer prerm/postinst hooks
    /// to save/restore Agent extensions during install, upgrade, and uninstall.
    ///
    /// The pre-remove hook is skipped when FLEET_INSTALL=1 because the fleet automation
    /// saves/removes extensions explicitly before invoking the MSI.
    /// The post-install hook always runs (even for fleet installs) so that extensions
    /// are restored before StartServices starts the agent.
    /// </summary>
    public class InstallerHooksCustomAction
    {
        private readonly ISession _session;
        private readonly string _installerExecutable;

        public InstallerHooksCustomAction(ISession session)
        {
            _session = session;
            var installDir = session.Property("PROJECTLOCATION");
            _installerExecutable = Path.Combine(installDir, "bin", "datadog-installer.exe");
        }

        private bool ShouldSkip()
        {
            var fleetInstall = _session.Property("FLEET_INSTALL");
            return !string.IsNullOrEmpty(fleetInstall) && fleetInstall == "1";
        }

        private bool IsUpgrade()
        {
            var upgradingProductCode = _session.Property("UPGRADINGPRODUCTCODE");
            return !string.IsNullOrEmpty(upgradingProductCode);
        }

        private Dictionary<string, string> InstallerEnvironmentVariables()
        {
            var env = new Dictionary<string, string>();
            var registryProps = new[]
            {
                "DD_INSTALLER_REGISTRY_URL",
                "DD_INSTALLER_REGISTRY_AUTH",
                "DD_INSTALLER_REGISTRY_USERNAME",
                "DD_INSTALLER_REGISTRY_PASSWORD",
            };
            foreach (var prop in registryProps)
            {
                var value = _session.Property(prop);
                if (!string.IsNullOrEmpty(value))
                {
                    env[prop] = value;
                }
            }
            return env;
        }

        private ActionResult RunHook(string hookArgs)
        {
            if (!File.Exists(_installerExecutable))
            {
                // The installer binary is expected to exist whenever this hook runs
                // (prerm and postinst conditions guard against the no-previous-install case),
                // but if it is somehow missing, treat it as a no-op rather than a hard failure
                // so the MSI operation can still complete.
                _session.Log($"Installer executable not found at {_installerExecutable}, skipping hook");
                return ActionResult.Success;
            }

            try
            {
                _session.Log($"Running installer hook: {hookArgs}");
                using (var proc = _session.RunCommand(_installerExecutable, hookArgs, InstallerEnvironmentVariables()))
                {
                    if (proc.ExitCode != 0)
                    {
                        _session.Log($"Installer hook '{hookArgs}' failed with exit code: {proc.ExitCode}");
                        return ActionResult.Failure;
                    }
                }
                return ActionResult.Success;
            }
            catch (Exception ex)
            {
                _session.Log($"Error running installer hook '{hookArgs}': {ex.Message}");
                return ActionResult.Failure;
            }
        }

        // Deferred: called during MSI uninstall or upgrade (before the old version is removed).
        // Skipped for fleet installs: fleet saves/removes extensions explicitly before invoking the MSI.
        private ActionResult RunPreRemoveHookImpl()
        {
            if (ShouldSkip())
            {
                _session.Log("Skipping pre-remove hook as FLEET_INSTALL is set to 1");
                return ActionResult.Success;
            }
            if (IsUpgrade())
            {
                return RunHook("prerm --upgrade datadog-agent msi");
            }
            // full uninstall
            return RunHook("prerm datadog-agent msi");
        }

        // Deferred: called after MSI install/upgrade completes
        private ActionResult RunPostInstallHookImpl()
        {
            return RunHook("postinst datadog-agent msi");
        }

        // Static entry points for WiX custom actions

        [CustomAction]
        public static ActionResult RunPreRemoveHook(Session session)
        {
            return new InstallerHooksCustomAction(new SessionWrapper(session)).RunPreRemoveHookImpl();
        }

        [CustomAction]
        public static ActionResult RunPostInstallHook(Session session)
        {
            return new InstallerHooksCustomAction(new SessionWrapper(session)).RunPostInstallHookImpl();
        }
    }
}
