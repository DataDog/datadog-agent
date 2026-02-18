using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using WixToolset.Dtf.WindowsInstaller;
using System;
using System.IO;

namespace Datadog.CustomActions
{
    /// <summary>
    /// MSI custom actions that call the datadog-installer prerm/postinst hooks
    /// to save/restore Agent extensions during install, upgrade, and uninstall.
    ///
    /// These actions are skipped when FLEET_INSTALL=1 because the fleet automation
    /// handles extension lifecycle through its own experiment hooks.
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

        private ActionResult RunHook(string hookArgs)
        {
            if (ShouldSkip())
            {
                _session.Log("Skipping installer hook as FLEET_INSTALL is set to 1");
                return ActionResult.Success;
            }

            if (!File.Exists(_installerExecutable))
            {
                _session.Log($"Installer executable not found at {_installerExecutable}");
                return ActionResult.Failure;
            }

            // TODO: What environment variables are needed for the hook?

            try
            {
                _session.Log($"Running installer hook: {hookArgs}");
                using (var proc = _session.RunCommand(_installerExecutable, hookArgs))
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
        private ActionResult RunPreRemoveHookImpl()
        {
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

        // Rollback: if prerm succeeded but later actions failed, restore extensions
        private ActionResult RollbackPreRemoveHookImpl()
        {
            // TODO: rollback of full uninstall will fail because prerm doesn't save the list of extensions.
            return RunHook("postinst datadog-agent msi");
        }

        // Rollback: if postinst succeeded but later actions failed, remove extensions
        private ActionResult RollbackPostInstallHookImpl()
        {
            return RunHook("prerm datadog-agent msi");
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

        [CustomAction]
        public static ActionResult RollbackPreRemoveHook(Session session)
        {
            return new InstallerHooksCustomAction(new SessionWrapper(session)).RollbackPreRemoveHookImpl();
        }

        [CustomAction]
        public static ActionResult RollbackPostInstallHook(Session session)
        {
            return new InstallerHooksCustomAction(new SessionWrapper(session)).RollbackPostInstallHookImpl();
        }
    }
}
