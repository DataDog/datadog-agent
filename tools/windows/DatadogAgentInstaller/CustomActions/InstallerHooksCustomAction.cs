using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using WixToolset.Dtf.WindowsInstaller;
using System;
using System.Collections.Generic;
using System.IO;

namespace Datadog.CustomActions
{
    public class InstallerHooksCustomAction
    {
        private static readonly (string MsiProperty, string EnvKey)[] InstallerHookEnvProps =
        {
            ("PROJECTLOCATION", "DD_PROJECTLOCATION"),
            ("APPLICATIONDATADIRECTORY", "DD_APPLICATIONDATADIRECTORY"),
            ("DD_INSTALLER_REGISTRY_URL", "DD_INSTALLER_REGISTRY_URL"),
            ("DD_INSTALLER_REGISTRY_AUTH", "DD_INSTALLER_REGISTRY_AUTH"),
            ("DD_INSTALLER_REGISTRY_USERNAME", "DD_INSTALLER_REGISTRY_USERNAME"),
            ("DD_INSTALLER_REGISTRY_PASSWORD", "DD_INSTALLER_REGISTRY_PASSWORD"),
            ("DD_OTELCOLLECTOR_ENABLED", "DD_OTELCOLLECTOR_ENABLED"),
            // EUDM gate for the ai-usage extension (installed via installAgentExtensions)
            ("DD_INFRASTRUCTURE_MODE", "DD_INFRASTRUCTURE_MODE"),
        };

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
            foreach (var (msiProperty, envKey) in InstallerHookEnvProps)
            {
                var value = _session.Property(msiProperty);
                if (!string.IsNullOrEmpty(value))
                {
                    env[envKey] = value;
                }
            }
            return env;
        }

        private ActionResult RunHook(string hookArgs)
        {
            if (!File.Exists(_installerExecutable))
            {
                _session.Log($"Installer executable not found at {_installerExecutable}");
                return ActionResult.Failure;
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
