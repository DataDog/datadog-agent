using System;
using System.Linq;
using System.ServiceProcess;
using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Microsoft.Deployment.WindowsInstaller;
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
                    var ddNpmService = _serviceController.Find("ddnpm");
                    if (ddNpmService != null)
                    {
                        _session["ALLOWCLOSEDSOURCE"] = ddNpmService.StartType != ServiceStartMode.Disabled ? "1" : "0";
                        _session.Log($"Found \"AllowClosedSource\" key, with value: {_session["ALLOWCLOSEDSOURCE"]}");
                    }
                    else
                    {
                        _session.Log("NPM service not found, assuming closed source consent was not given.");
                    }
                }

                using (var subkey = _registryServices.OpenRegistryKey(Registries.LocalMachine, @"Software\Microsoft\Windows NT\CurrentVersion"))
                {
                    if (subkey != null)
                    {
                        _session["WindowsBuild"] = subkey.GetValue("CurrentBuild").ToString();
                        _session.Log($"WindowsBuild: {_session["WindowsBuild"]}");
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
    }
}
