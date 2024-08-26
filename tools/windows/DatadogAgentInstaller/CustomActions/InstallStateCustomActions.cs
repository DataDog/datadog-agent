using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Microsoft.Deployment.WindowsInstaller;
using Microsoft.Win32;
using System;
using System.Collections.Generic;
using System.IO;
using System.Security.Principal;
using ServiceController = Datadog.CustomActions.Native.ServiceController;

namespace Datadog.CustomActions
{
    // Fetch and process registry value(s) and return a string to be assigned to a WIX property.
    using GetRegistryPropertyHandler = Func<string>;

    public class InstallStateCustomActions
    {
        private readonly ISession _session;
        private readonly IRegistryServices _registryServices;
        private readonly IServiceController _serviceController;

        private readonly INativeMethods _nativeMethods;

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

        public InstallStateCustomActions(
            ISession session,
            IRegistryServices registryServices,
            IServiceController serviceController,
            INativeMethods nativeMethods)
        {
            _session = session;
            _registryServices = registryServices;
            _serviceController = serviceController;
            _nativeMethods = nativeMethods;
        }

        public InstallStateCustomActions(ISession session)
            : this(
                session,
                new RegistryServices(),
                new ServiceController(),
                new Win32NativeMethods())
        {
        }

        /// <summary>
        /// If the WIX property <c>propertyName</c> does not have a value, assign it the value returned by <c>handler</c>.
        /// This gives precedence to properties provided on the command line over the registry values.
        /// </summary>
        private static void RegistryProperty(ISession session, string propertyName, GetRegistryPropertyHandler handler)
        {
            if (string.IsNullOrEmpty(session[propertyName]))
            {
                try
                {
                    var propertyVal = handler();
                    if (!string.IsNullOrEmpty(propertyVal))
                    {
                        session[propertyName] = propertyVal;
                        session.Log($"Found {propertyName} in registry {session[propertyName]}");
                    }
                }
                catch (Exception e)
                {
                    session.Log($"Exception processing registry value for {propertyName}: {e}");
                }
            }
            else
            {
                session.Log($"User provided {propertyName} {session[propertyName]}");
            }
        }

        /// <summary>
        /// Convenience wrapper of <c>RegistryProperty</c> for properties that have an exact 1:1 mapping to a registry value
        /// and don't require additional processing.
        /// </summary>
        private static void RegistryValueProperty(ISession session, string propertyName, IRegistryKey registryKey,
            string registryValue)
        {
            RegistryProperty(session, propertyName,
                () => registryKey.GetValue(registryValue)?.ToString());
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
                    // DDAGENTUSER_NAME
                    //
                    // The user account can be provided to the installer by
                    // * The registry
                    // * The command line
                    // * The agent user dialog
                    // The user account domain and name are stored separately in the registry
                    // but are passed together on the command line and the agent user dialog.
                    // This function will combine the registry properties if they exist.
                    // Preference is given to creds provided on the command line and the agent user dialog.
                    // For UI installs it ensures that the agent user dialog is pre-populated.
                    RegistryProperty(_session, "DDAGENTUSER_NAME",
                        () =>
                        {
                            var domain = subkey.GetValue("installedDomain")?.ToString();
                            var user = subkey.GetValue("installedUser")?.ToString();
                            if (!string.IsNullOrEmpty(domain) && !string.IsNullOrEmpty(user))
                            {
                                return $"{domain}\\{user}";
                            }

                            return string.Empty;
                        });

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
                    _session.Log("PROJECTLOCATION is not set, cannot set RemoveFolderEx properties, some files may be left behind in the installation directory.");
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

        /// <summary>
        /// WiX doesn't support getting the real build number on Windows 10+ so we must fetch it ourselves
        /// </summary>
        public void GetWindowsBuildVersion()
        {
            using var subkey = _registryServices.OpenRegistryKey(Registries.LocalMachine,
                @"Software\Microsoft\Windows NT\CurrentVersion");
            if (subkey != null)
            {
                var currentBuild = subkey.GetValue("CurrentBuild");
                if (currentBuild != null)
                {
                    _session["DDAGENT_WINDOWSBUILD"] = subkey.GetValue("CurrentBuild").ToString();
                    _session.Log($"WindowsBuild: {_session["DDAGENT_WINDOWSBUILD"]}");
                }
            }
            else
            {
                _session.Log("WindowsBuild not found");
            }
        }

        // <summary>
        // Sets driver-specific properties to 1 if the driver services should be removed on rollback.
        // </summary>
        // <remarks>
        // Previous versions of the installer did not remove the driver services on rollback.
        // In order to remain compatible with these versions, we must not remove the driver services
        // when rolling back to these versions.
        // </remarks>
        public void SetDDDriverRollback()
        {
            var upgradeDetected = _session["WIX_UPGRADE_DETECTED"];

            if (!string.IsNullOrEmpty(upgradeDetected)) // This is an upgrade, conditionally set rollback flags.
            {
                var versionString = _nativeMethods.GetVersionString(upgradeDetected);
                // Using Version class
                // https://learn.microsoft.com/en-us/dotnet/api/system.version?view=net-8.0
                var currentVersion = new Version(versionString);
                Version procmonDriverMinimumVersion;
                Version driverRollbackMinimumVersion;

                // Check major version
                if (versionString[0] == '7')
                {
                    procmonDriverMinimumVersion = new Version("7.52");
                    driverRollbackMinimumVersion = new Version("7.56");
                }
                else
                {
                    procmonDriverMinimumVersion = new Version("6.52");
                    driverRollbackMinimumVersion = new Version("6.56");
                }

                var compareResult = currentVersion.CompareTo(driverRollbackMinimumVersion);
                if (compareResult < 0) // currentVersion is less than minimumVersion
                {
                    // case: upgrading from a version that did not implement driver rollback
                    // Clear NPM flag to ensure NPM service is not deleted on rollback.
                    _session["DDDRIVERROLLBACK_NPM"] = "";

                    var compare_52 = currentVersion.CompareTo(procmonDriverMinimumVersion);
                    if (compare_52 < 0) //currentVersion is less than 6.52/7.52
                    {
                        // case: upgrading from a version that did not the include procmon driver
                        // Set PROCMON flag to ensure procmon driver is deleted on rollback.
                        _session["DDDRIVERROLLBACK_PROCMON"] = "1";
                    }
                    else
                    {
                        _session["DDDRIVERROLLBACK_PROCMON"] = "";
                    }
                }
                else // currentVersion is not less than minimumVersion
                {
                    _session["DDDRIVERROLLBACK_NPM"] = "1";
                    _session["DDDRIVERROLLBACK_PROCMON"] = "1";
                }
            }
            else  // This is a fresh install, set rollback flags to ensure drivers are deleted on rollback.
            {
                _session["DDDRIVERROLLBACK_NPM"] = "1";
                _session["DDDRIVERROLLBACK_PROCMON"] = "1";
            }
            _session.Log($"DDDriverRollback_NPM: {_session["DDDRIVERROLLBACK_NPM"]}");
            _session.Log($"DDDriverRollback_Procmon: {_session["DDDRIVERROLLBACK_PROCMON"]}");
        }

        [CustomAction]
        public static ActionResult ReadInstallState(Session session)
        {
            return new InstallStateCustomActions(new SessionWrapper(session)).ReadInstallState();
        }

        [CustomAction]
        public static ActionResult ReadWindowsVersion(Session session)
        {
            new InstallStateCustomActions(new SessionWrapper(session)).GetWindowsBuildVersion();
            return ActionResult.Success;
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

                _session.Log($"Storing installedDomain={_session.Property("DDAGENTUSER_PROCESSED_DOMAIN")}");
                subkey.SetValue("installedDomain", _session.Property("DDAGENTUSER_PROCESSED_DOMAIN"),
                    RegistryValueKind.String);
                _session.Log($"Storing installedUser={_session.Property("DDAGENTUSER_PROCESSED_NAME")}");
                subkey.SetValue("installedUser", _session.Property("DDAGENTUSER_PROCESSED_NAME"),
                    RegistryValueKind.String);
            }
            catch (Exception e)
            {
                _session.Log($"Error storing registry properties: {e}");
                return ActionResult.Failure;
            }

            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult WriteInstallState(Session session)
        {
            return new InstallStateCustomActions(new SessionWrapper(session)).WriteInstallState();
        }


        /// <summary>
        /// Uninstall CA that removes the changes from the WriteInstallState CA
        /// </summary>
        /// <remarks>
        /// If these registry values are not removed then MSI won't remove the key.
        /// </remarks>
        public ActionResult UninstallWriteInstallState()
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

                foreach (var value in new[]
                         {
                             "installedDomain",
                             "installedUser"
                         })
                {
                    try
                    {
                        subkey.DeleteValue(value);
                    }
                    catch (Exception e)
                    {
                        // Don't print stack trace as it may be seen as a terminal error by readers of the log.
                        _session.Log($"Warning, cannot removing registry value: {e.Message}");
                    }
                }
            }
            catch (Exception e)
            {
                _session.Log($"Warning, could not access registry key {Constants.DatadogAgentRegistryKey}: {e}");
                // This step can fail without failing the un-installation.
            }

            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult UninstallWriteInstallState(Session session)
        {
            return new InstallStateCustomActions(new SessionWrapper(session)).UninstallWriteInstallState();
        }

        public static SecurityIdentifier GetPreviousAgentUser(ISession session, IRegistryServices registryServices, INativeMethods nativeMethods)
        {
            try
            {
                using var subkey =
                    registryServices.OpenRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey);
                if (subkey == null)
                {
                    throw new Exception("Datadog registry key does not exist");
                }
                var domain = subkey.GetValue("installedDomain")?.ToString();
                var user = subkey.GetValue("installedUser")?.ToString();
                if (string.IsNullOrEmpty(domain) || string.IsNullOrEmpty(user))
                {
                    throw new Exception("Agent user information is not in registry");
                }

                var accountName = $"{domain}\\{user}";
                session.Log($"Found agent user information in registry {accountName}");
                var userFound = nativeMethods.LookupAccountName(accountName,
                    out _,
                    out _,
                    out var securityIdentifier,
                    out _);
                if (!userFound || securityIdentifier == null)
                {
                    throw new Exception($"Could not find account for user {accountName}.");
                }

                session.Log($"Found previous agent user {accountName} ({securityIdentifier})");
                return securityIdentifier;
            }
            catch (Exception e)
            {
                session.Log($"Could not find previous agent user: {e}");
            }

            return null;
        }
    }
}
