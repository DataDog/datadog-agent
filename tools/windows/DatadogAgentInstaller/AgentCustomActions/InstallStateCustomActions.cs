using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Microsoft.Deployment.WindowsInstaller;
using System;
using System.Collections.Generic;
using System.IO;

namespace Datadog.AgentCustomActions
{
    public class ReadInstallStateCA : Datadog.CustomActions.InstallStateCustomActions
    {
        public ReadInstallStateCA(ISession session) : base(session)
        {
        }

        public ReadInstallStateCA(ISession session,
            IRegistryServices registryServices,
            IServiceController serviceController,
            INativeMethods nativeMethods)
            : base(session, registryServices, serviceController, nativeMethods)
        {
        }

        /// <summary>
        /// Assigns WIX properties that were not provided by the user to their registry values.
        /// </summary>
        /// <remarks>
        /// Custom Action that runs (only once) in either the InstallUISequence or the InstallExecuteSequence.
        ///
        /// During removing-for-upgrade the installer being removed does not receive any properties from the
        /// installer being installed, only UPGRADINGPRODUCTCODE is set. Thus the state for the installer being
        /// removed will come from the registry values only.
        /// </remarks>
        public ActionResult ReadInstallState()
        {
            try
            {
                using var subkey =
                    _registryServices.OpenRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey);
                if (subkey != null)
                {
                    LoadAgentUserProperty(subkey);
                    RegistryValueProperty(_session, "PROJECTLOCATION", subkey, "InstallPath");
                    RegistryValueProperty(_session, "APPLICATIONDATADIRECTORY", subkey, "ConfigRoot");
                }

                GetWindowsBuildVersion();
                SetDDDriverRollback();
                SetRemoveFolderExProperties();
            }
            catch (Exception e)
            {
                _session.Log($"Error reading install state: {e}");
                return ActionResult.Failure;
            }

            return ActionResult.Success;
        }

        // <summary>
        // Sets driver-specific properties to 1 if the driver services should be removed on rollback.
        // </summary>
        // <remarks>
        // Previous versions of the installer did not remove the driver services on rollback.
        // In order to remain compatible with these versions, we must not remove the driver services
        // when rolling back to these versions.
        // </remarks>
        private void SetDDDriverRollback()
        {
            // On a fresh install, set rollback flags to ensure drivers are deleted on rollback.
            _session["DDDRIVERROLLBACK_NPM"] = "1";
            _session["DDDRIVERROLLBACK_PROCMON"] = "1";

            try
            {
                // We've seen cases where WIX_UPGRADE_DETECTED refers to products that are not actually installed,
                // some of these cases are further corrupted and `GetVersionString` fails.
                // We've also seen cases where WIX_UPGRADE_DETECTED contains MULTIPLE product codes, even though only one is expected.
                // These appear to be Windows installer bugs and it creates a state where we can't reliably determine
                // if this is a major upgrade or a "fresh" install, and we can't reliably determine the previous version.
                var upgradeDetected = _session["WIX_UPGRADE_DETECTED"];
                if (!string.IsNullOrEmpty(upgradeDetected)) // This is an upgrade, conditionally set rollback flags.
                {
                    _session.Log($"WIX_UPGRADE_DETECTED: {_session["WIX_UPGRADE_DETECTED"]}");
                    var versionString = _nativeMethods.GetVersionString(upgradeDetected);
                    _session.Log($"versionString: {versionString}");
                    // Using Version class
                    // https://learn.microsoft.com/en-us/dotnet/api/system.version?view=net-8.0
                    var currentVersion = new Version(versionString);
                    var procmonDriverMinimumVersion = new Version(currentVersion.Major, 52);
                    var driverRollbackMinimumVersion = new Version(currentVersion.Major, 56);

                    var compareResult = currentVersion.CompareTo(driverRollbackMinimumVersion);
                    if (compareResult < 0) // currentVersion is less than minimumVersion
                    {
                        // case: upgrading from a version that did not implement driver rollback
                        // Clear NPM flag to ensure NPM service is not deleted on rollback.
                        _session["DDDRIVERROLLBACK_NPM"] = "";

                        var compare_52 = currentVersion.CompareTo(procmonDriverMinimumVersion);
                        if (compare_52 >= 0) //currentVersion is greater or equal to 6.52/7.52
                        {
                            // case: upgrading from a version that did include the procmon driver
                            // Clear PROCMON flag to ensure procmon driver is kept on rollback for compatibility.
                            _session["DDDRIVERROLLBACK_PROCMON"] = "";
                        }
                    }
                }
            }
            catch (Exception e)
            {
                _session.Log($"Error setting DDDriverRollback, values may not be set correctly for the version being upgraded from: {e}");
                // Do not fail the installer here.
                // We should only end up here in the Windows installer bug cases described above.
                // In this case we assume that the version being upgraded from is a recent version that implements driver rollback.
                // If upgrading from a version that did not implement driver rollback AND the upgrade happens
                // to rollback then the host could be left without the driver installed.
            }

            _session.Log($"DDDriverRollback_NPM: {_session["DDDRIVERROLLBACK_NPM"]}");
            _session.Log($"DDDriverRollback_Procmon: {_session["DDDRIVERROLLBACK_PROCMON"]}");
        }

        /// <summary>
        /// Sets the properties used by the WiX util RemoveFolderEx to cleanup non-tracked paths.
        ///
        /// https://wixtoolset.org/docs/v3/xsd/util/removefolderex/
        /// </summary>
        /// <remarks>
        /// RemoveFolderEx only takes a property as input, not IDs or paths, meaning we can't
        /// pass something like $PROJECTLOCATION\bin\agent in WiX. Instead, we have to set
        /// the properties in a custom action.
        ///
        /// The RemoveFolderEx elements in WiX are configured to only run at uninstall time,
        /// so the properties values are only relevant then. However, the WixRemoveFolderEx
        /// custom action will fail fast if any of the properties are empty. So we must always
        /// provide a value to the properties to prevent an error in the log and to accomodate other
        /// uses of RemoveFolderEx that run at other times (at time of writing there are none).
        /// Though this error does not stop the installer, it will ignore it and continue.
        ///
        /// RemoveFolderEx handles rollback and will restore any file paths that it deletes.
        ///
        /// We specify specific subdirectories under PROJECTLOCATION instead of specifying PROJECTLOCATION
        /// itself to reduce the impact when the Agent is erroneously installed to an existing directory.
        ///
        /// This action copies PROJECTLOCATION and thus changes to PROJECTLOCATION will not be reflected
        /// in these properties. This should not be an issue since PROJECTLOCATION should not change during
        /// uninstallation.
        /// </remarks>
        private void SetRemoveFolderExProperties()
        {
            var installDir = _session["PROJECTLOCATION"];
            if (string.IsNullOrEmpty(installDir))
            {
                if (!string.IsNullOrEmpty(_session["REMOVE"]))
                {
                    _session.Log(
                        "PROJECTLOCATION is not set, cannot set RemoveFolderEx properties, some files may be left behind in the installation directory.");
                }

                return;
                // We cannot throw an exception here because the installer will fail. This case can happen, for example,
                // if the cleanup script deleted the registry keys before running the uninstaller.
            }

            foreach (var entry in PathsToRemoveOnUninstall())
            {
                _session[entry.Key] = Path.Combine(installDir, entry.Value);
            }
        }

        public static Dictionary<string, string> PathsToRemoveOnUninstall()
        {
            var pathPropertyMap = new Dictionary<string, string>();
            var paths = new List<string>
            {
                "bin\\agent",
                "embedded3",
                // embedded2 only exists in Agent 6, so an error will be logged, but install will continue
                "embedded2",
            };
            for (var i = 0; i < paths.Count; i++)
            {
                // property names are a maximum of 72 characters (can't find a source for this, but can verify in Property table schema in Orca)
                // WixRemoveFolderEx creates properties like PROJECTLOCATION_0 so mimic that here.
                // include lowercase letters so the property isn't made public.
                pathPropertyMap.Add($"dd_PROJECTLOCATION_{i}", paths[i]);
            }

            return pathPropertyMap;
        }
    }

    public class WriteInstallStateCA : Datadog.CustomActions.InstallStateCustomActions
    {
        public WriteInstallStateCA(ISession session) : base(session)
        {
        }

        /// <summary>
        /// Deferred custom action that stores properties in the registry
        /// </summary>
        /// <remarks>
        /// WiX RegistryValue elements are only written when their parent Feature is installed. This means
        /// that on change/modify operations the registry keys are not updated. This custom action writes
        /// the properties to the registry that we need to change during change/modify installer operations.
        /// </remarks>
        public ActionResult WriteInstallState()
        {
            try
            {
                using var subkey =
                    _registryServices.CreateRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey);
                if (subkey == null)
                {
                    throw new Exception("Unable to create agent registry key");
                }

                StoreAgentUserInRegistry(subkey);
            }
            catch (Exception e)
            {
                _session.Log($"Error storing registry properties: {e}");
                return ActionResult.Failure;
            }

            return ActionResult.Success;
        }


        /// <summary>
        /// Uninstall CA that removes the changes from the WriteInstallState CA
        /// </summary>
        /// <remarks>
        /// If these registry values are not removed then MSI won't remove the key.
        /// </remarks>
        public ActionResult DeleteInstallState()
        {
            try
            {
                using var subkey =
                    _registryServices.OpenRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey,
                        writable: true);
                if (subkey == null)
                {
                    // registry key does not exist, nothing to do
                    _session.Log(
                        $"Registry key HKLM\\{Constants.DatadogAgentRegistryKey} does not exist, there are no values to remove.");
                    return ActionResult.Success;
                }

                RemoveAgentUserInRegistry(subkey);
            }
            catch (Exception e)
            {
                _session.Log($"Warning, could not access registry key {Constants.DatadogAgentRegistryKey}: {e}");
                // This step can fail without failing the un-installation.
            }

            return ActionResult.Success;
        }
    }
}
