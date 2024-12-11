using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Microsoft.Deployment.WindowsInstaller;
using System;

namespace Datadog.InstallerCustomActions
{

    public class ReadInstallStateCA : Datadog.CustomActions.InstallStateCustomActions
    {
        public ReadInstallStateCA(ISession session) : base(session)
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
                    _registryServices.OpenRegistryKey(Registries.LocalMachine, Constants.DatadogInstallerRegistryKey);
                if (subkey != null)
                {
                    LoadAgentUserProperty(subkey);
                }

                GetWindowsBuildVersion();
            }
            catch (Exception e)
            {
                _session.Log($"Error reading install state: {e}");
                return ActionResult.Failure;
            }

            return ActionResult.Success;
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
                    _registryServices.CreateRegistryKey(Registries.LocalMachine, Constants.DatadogInstallerRegistryKey);
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
        public ActionResult DeleteInstallState()
        {
            try
            {
                using var subkey =
                    _registryServices.OpenRegistryKey(Registries.LocalMachine, Constants.DatadogInstallerRegistryKey,
                        writable: true);
                if (subkey == null)
                {
                    // registry key does not exist, nothing to do
                    _session.Log(
                        $"Registry key HKLM\\{Constants.DatadogInstallerRegistryKey} does not exist, there are no values to remove.");
                    return ActionResult.Success;
                }

                RemoveAgentUserInRegistry(subkey);

                // Remove the registry key if it is empty.
                // This mimics MSI behavior. If in the future the registry key is added as a component
                // then we can remove this code.
                try
                {
                    _registryServices.DeleteSubKey(Registries.LocalMachine, Constants.DatadogInstallerRegistryKey);
                }
                catch (Exception e)
                {
                    _session.Log($"Warning, could not remove registry key {Constants.DatadogInstallerRegistryKey}: {e}");
                    // This step can fail without failing the un-installation.
                }
            }
            catch (Exception e)
            {
                _session.Log($"Warning, could not access registry key {Constants.DatadogInstallerRegistryKey}: {e}");
                // This step can fail without failing the un-installation.
            }

            return ActionResult.Success;
        }
    }
}
