using System;
using System.ComponentModel;
using System.ServiceProcess;
using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Microsoft.Deployment.WindowsInstaller;
using Microsoft.Win32;
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

        public InstallStateCustomActions(
            ISession session,
            IRegistryServices registryServices,
            IServiceController serviceController)
        {
            _session = session;
            _registryServices = registryServices;
            _serviceController = serviceController;
        }

        public InstallStateCustomActions(ISession session)
        : this(
            session,
            new RegistryServices(),
            new ServiceController())
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
        private static void RegistryValueProperty(ISession session, string propertyName, IRegistryKey registryKey, string registryValue)
        {
            RegistryProperty(session, propertyName,
                () => registryKey.GetValue(registryValue)?.ToString());
        }

        /// <summary>
        /// Assigns WIX properties that were not provided by the user to their registry values.
        /// </summary>
        /// <remarks>
        /// Custom Action that runs (only once) in either the InstallUISequence or the InstallExecuteSequence.
        /// </remarks>
        public ActionResult ReadInstallState()
        {
            try
            {
                using (var subkey = _registryServices.OpenRegistryKey(Registries.LocalMachine, @"Software\Datadog\Datadog Agent"))
                {
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
                        RegistryValueProperty(_session, "ALLOWCLOSEDSOURCE", subkey, "AllowClosedSource");
                    }
                }

                if (string.IsNullOrEmpty(_session.Property("ALLOWCLOSEDSOURCE")))
                {
                    _session.Log("Cannot find the \"AllowClosedSource\" registry key, checking the NPM service state.");
                    var ddNpmService = _serviceController.Find(Constants.NpmServiceName);
                    if (ddNpmService != null)
                    {
                        _session["ALLOWCLOSEDSOURCE"] = ddNpmService.StartType != ServiceStartMode.Disabled ? Constants.AllowClosedSource_Yes : Constants.AllowClosedSource_No;
                        _session.Log($"{Constants.NpmServiceName} start type is {ddNpmService.StartType}, setting \"AllowClosedSource\" to: {_session["ALLOWCLOSEDSOURCE"]}");

                    }
                    else
                    {
                        _session.Log("NPM service not found, assuming closed source consent was not given.");
                        _session["ALLOWCLOSEDSOURCE"] = Constants.AllowClosedSource_No;
                    }
                }
                else
                {
                    _session.Log($"Found \"AllowClosedSource\" key, with value: {_session["ALLOWCLOSEDSOURCE"]}");
                }

                if (_session.Property("ALLOWCLOSEDSOURCE") == Constants.AllowClosedSource_Yes)
                {
                    // Set another property that will control the state of the Allow Closed Source checkbox in the GUI
                    // We cannot use the same property because the checkbox interprets any property values as "checked",
                    // but we need the property to be either "0" or "1" in order for the WiX RegistryValue element to
                    // write the value to the registry, it will fail if the property is not set. So we must either add
                    // another custom action or another property.
                    _session["CHECKBOX_ALLOWCLOSEDSOURCE"] = Constants.AllowClosedSource_Yes;
                }
                else
                {
                    // Ensure we always set this property, otherwise the RegistryValue action will fail
                    _session["ALLOWCLOSEDSOURCE"] = Constants.AllowClosedSource_No;
                }

                using (var subkey = _registryServices.OpenRegistryKey(Registries.LocalMachine, @"Software\Microsoft\Windows NT\CurrentVersion"))
                {
                    if (subkey != null)
                    {
                        var currentBuild = subkey.GetValue("CurrentBuild");
                        if (currentBuild != null)
                        {
                            _session["WindowsBuild"] = subkey.GetValue("CurrentBuild").ToString();
                            _session.Log($"WindowsBuild: {_session["WindowsBuild"]}");
                        }
                    }
                    else
                    {
                        _session.Log("WindowsBuild not found");
                    }
                }

            }
            catch (Exception e)
            {
                _session.Log($"Error processing registry properties: {e}");
                return ActionResult.Failure;
            }
            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult ReadInstallState(Session session)
        {
            return new InstallStateCustomActions(new SessionWrapper(session)).ReadInstallState();
        }


        /// <summary>
        /// Deferred custom action that stores properties in the registry
        /// </summary>
        /// /// <remarks>
        /// WiX RegistryValue elements are only written when their parent Feature is installed. This means
        /// that on change/modify operations the registry keys are not updated. This custom action writes
        /// the properties to the registry that we need to change during change/modify installer operations.
        /// </remarks>
        public ActionResult WriteInstallState()
        {
            try
            {
                using var subkey =
                    _registryServices.CreateRegistryKey(Registries.LocalMachine, @"Software\Datadog\Datadog Agent");
                if (subkey == null)
                {
                    throw new Exception("Unable to create agent registry key");
                }
                _session.Log($"Storing installedDomain={_session.Property("DDAGENTUSER_PROCESSED_DOMAIN")}");
                subkey.SetValue("installedDomain", _session.Property("DDAGENTUSER_PROCESSED_DOMAIN"), RegistryValueKind.String);
                _session.Log($"Storing installedUser={_session.Property("DDAGENTUSER_PROCESSED_NAME")}");
                subkey.SetValue("installedUser", _session.Property("DDAGENTUSER_PROCESSED_NAME"), RegistryValueKind.String);
                _session.Log($"Storing AllowClosedSource={_session.Property("ALLOWCLOSEDSOURCE")}");
                subkey.SetValue("AllowClosedSource", _session.Property("ALLOWCLOSEDSOURCE"), RegistryValueKind.DWord);
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
    }
}
