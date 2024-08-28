using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Microsoft.Win32;
using System;
using System.Security.Principal;
using ServiceController = Datadog.CustomActions.Native.ServiceController;

namespace Datadog.CustomActions
{
    // Fetch and process registry value(s) and return a string to be assigned to a WIX property.
    using GetRegistryPropertyHandler = Func<string>;

    public class InstallStateCustomActions
    {
        protected readonly ISession _session;
        protected readonly IRegistryServices _registryServices;
        protected readonly IServiceController _serviceController;

        protected readonly INativeMethods _nativeMethods;

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
        public static void RegistryProperty(ISession session, string propertyName, GetRegistryPropertyHandler handler)
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
        public static void RegistryValueProperty(ISession session, string propertyName, IRegistryKey registryKey,
            string registryValue)
        {
            RegistryProperty(session, propertyName,
                () => registryKey.GetValue(registryValue)?.ToString());
        }

        /// <summary>
        /// Sets the WIX property DDAGENTUSER_NAME
        /// </summary>
        /// <remarks>
        /// The user account can be provided to the installer by
        /// * The registry
        /// * The command line
        /// * The agent user dialog
        /// The user account domain and name are stored separately in the registry
        /// but are passed together on the command line and the agent user dialog.
        /// This function will combine the registry properties if they exist.
        /// Preference is given to creds provided on the command line and the agent user dialog.
        /// For UI installs it ensures that the agent user dialog is pre-populated.
        /// </remarks>
        protected void LoadAgentUserProperty(IRegistryKey subkey)
        {
            // handle user input property
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
            // also separately store the registry values for use in rollback
            RegistryValueProperty(_session, "DDAGENT_installedDomain", subkey, "installedDomain");
            RegistryValueProperty(_session, "DDAGENT_installedUser", subkey, "installedUser");
        }

        protected void StoreAgentUserInRegistry(IRegistryKey subkey)
        {
            // Store the agent user in the registry
            // The agent user is stored in the registry as separate domain and user values.
            // Store the processes values from ProcessDDAgentUserCredentials if they exist.
            // Fallback to the original values if they don't (e.g. uninstall doesn't run ProcessDDAgentUserCredentials).
            var domain = _session.Property("DDAGENTUSER_PROCESSED_DOMAIN");
            if (string.IsNullOrEmpty(domain))
            {
                domain = _session.Property("DDAGENT_installedDomain");
            }
            if (!string.IsNullOrEmpty(domain))
            {
                _session.Log($"Storing installedDomain={domain}");
                subkey.SetValue("installedDomain", domain, RegistryValueKind.String);
            }
            var user = _session.Property("DDAGENTUSER_PROCESSED_NAME");
            if (string.IsNullOrEmpty(user))
            {
                user = _session.Property("DDAGENT_installedUser");
            }
            if (!string.IsNullOrEmpty(user))
            {
                _session.Log($"Storing installedUser={user}");
                subkey.SetValue("installedUser", user, RegistryValueKind.String);
            }
        }

        protected void RemoveAgentUserInRegistry(IRegistryKey subkey)
        {
            foreach (var value in new[] { "installedDomain", "installedUser" })
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

        public static SecurityIdentifier GetPreviousAgentUser(ISession session, IRegistryServices registryServices,
            INativeMethods nativeMethods)
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
