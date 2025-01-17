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
                StoreAgentUserPassword();
            }
            catch (Exception e)
            {
                _session.Log($"Error storing registry properties: {e}");
                return ActionResult.Failure;
            }

            return ActionResult.Success;
        }

        /// <summary>
        /// Store the agent user in the LSA secret store
        /// </summary>
        /// <remarks>
        /// Store the password in LSA secret store so that it can be used during Fleet Automation remote upgrades
        /// This is the same place that Windows Service Manager stores service account passwords, so
        /// this is not introducing NEW risk.
        /// https://docs.microsoft.com/en-us/windows/win32/services/service-accounts
        /// https://learn.microsoft.com/en-us/windows/win32/secmgmt/storing-private-data
        /// </remarks>
        private void StoreAgentUserPassword()
        {
            // use M$ prefix to indicate the secret is "System" level secret / Machine private data object.
            // these "cannot be accessed remotely" and "can be accessed only by the operating system"
            // https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-lsad/483f1b6e-7b14-4341-9ab2-9b99c01f896e
            // https://learn.microsoft.com/en-us/windows/win32/secmgmt/private-data-object
            var secretType = "M$";
            var keyName = $"{secretType}datadog_ddagentuser_password";

            var isServiceAccount = _session.Property("DDAGENTUSER_IS_SERVICE_ACCOUNT") == "1";
            var ddagentuserPassword = _session.Property("DDAGENTUSER_PROCESSED_PASSWORD");
            if (isServiceAccount)
            {
                // If ddagentuser is a service account, it has no password, so remove any previous entries from the LSA
                // NOTE: The Agent installer allows upgrades without re-providing the password, so the
                //       password may be empty 
                // NOTE: This is a difference in behavior between the Fleet Installer and the Agent installer.
                //       The Agent installer allows upgrades without re-providing the password. However
                //       the Fleet Installer must require the password always be provided.
                _nativeMethods.RemoveSecret(keyName);
            }
            else
            {
                _nativeMethods.StoreSecret(keyName, ddagentuserPassword);
            }


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
