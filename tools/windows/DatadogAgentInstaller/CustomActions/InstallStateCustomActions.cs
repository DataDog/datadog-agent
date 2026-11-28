using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Microsoft.Win32;
using System;
using System.Security.Cryptography;
using System.Security.Principal;
using ServiceController = Datadog.CustomActions.Native.ServiceController;

namespace Datadog.CustomActions
{
    // Fetch and process registry value(s) and return a string to be assigned to a WIX property.
    using GetRegistryPropertyHandler = Func<string>;

    public class InstallStateCustomActions
    {
        // Registry value name under HKLM\Constants.DatadogAgentRegistryKey that mirrors the
        // DDAGENTUSER_KEEP_RIGHTS MSI property so remote-triggered upgrades can recover the
        // operator's opt-out without it being re-supplied on the command line.
        protected const string KeepRightsRegistryValueName = "keepRights";

        /// <summary>
        /// Set when the stored Agent account was not used because the Agent service is missing or runs as another account.
        /// </summary>
        // Public and declared in the Agent installer project so it reaches the execute sequence from the UI sequence.
        public const string StoredAgentUserMismatchPropertyName = "DDAGENTUSER_STORED_MISMATCH";

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
        /// When <c>serviceName</c> is set, the stored account is used only if that service is installed and
        /// runs as it. Otherwise the default local ddagentuser account is used if the service runs as it,
        /// else DDAGENTUSER_NAME is left unset and <c>StoredAgentUserMismatchPropertyName</c> is set so that
        /// the install asks for the account.
        /// </remarks>
        protected void LoadAgentUserProperty(IRegistryKey subkey, string serviceName = null)
        {
            // handle user input property
            RegistryProperty(_session, "DDAGENTUSER_NAME",
                () =>
                {
                    var domain = subkey.GetValue("installedDomain")?.ToString();
                    var user = subkey.GetValue("installedUser")?.ToString();
                    if (string.IsNullOrEmpty(domain) || string.IsNullOrEmpty(user))
                    {
                        return string.Empty;
                    }

                    var account = $"{domain}\\{user}";
                    string serviceAccount = null;
                    bool runsAsAccount;
                    try
                    {
                        runsAsAccount = serviceName == null || ServiceRunsAsAccount(serviceName, account, out serviceAccount);
                    }
                    catch (Exception e)
                    {
                        _session.Log($"Could not compare stored account {account} with the {serviceName} service account: {e}");
                        runsAsAccount = false;
                    }

                    if (runsAsAccount)
                    {
                        return account;
                    }

                    // An install without a stored account uses this default, so keeping it changes nothing.
                    var defaultAccount = $"{Environment.MachineName}\\ddagentuser";
                    if (string.Equals(serviceAccount, defaultAccount, StringComparison.OrdinalIgnoreCase) ||
                        string.Equals(serviceAccount, ".\\ddagentuser", StringComparison.OrdinalIgnoreCase))
                    {
                        _session.Log($"Using the {serviceName} service account {defaultAccount}");
                        return defaultAccount;
                    }

                    _session[StoredAgentUserMismatchPropertyName] = "true";
                    return string.Empty;
                });
            // also separately store the registry values for use in rollback
            RegistryValueProperty(_session, "DDAGENT_installedDomain", subkey, "installedDomain");
            RegistryValueProperty(_session, "DDAGENT_installedUser", subkey, "installedUser");
        }

        /// <summary>
        /// Returns true if the service is installed and its logon account is <c>account</c>.
        /// </summary>
        /// <remarks>
        /// Only administrators can change a service's logon account.
        /// </remarks>
        private bool ServiceRunsAsAccount(string serviceName, string account, out string serviceAccount)
        {
            using var serviceKey = _registryServices.OpenRegistryKey(Registries.LocalMachine,
                $@"SYSTEM\CurrentControlSet\Services\{serviceName}");
            serviceAccount = serviceKey?.GetValue("ObjectName")?.ToString();
            if (string.IsNullOrEmpty(serviceAccount))
            {
                _session.Log($"Stored account {account} found but the {serviceName} service is not installed");
                return false;
            }

            if (IsSameAccount(account, serviceAccount))
            {
                return true;
            }

            _session.Log(
                $"Stored account {account} does not match the {serviceName} service account {serviceAccount}");
            return false;
        }

        private bool IsSameAccount(string storedAccount, string serviceAccount)
        {
            if (string.Equals(storedAccount, serviceAccount, StringComparison.OrdinalIgnoreCase))
            {
                return true;
            }

            if (!_nativeMethods.LookupAccountName(storedAccount, out var storedUser, out var storedDomain,
                    out var storedSid, out _))
            {
                return false;
            }

            var storedName = $"{storedDomain}\\{storedUser}";
            var builtIn = BuiltInServiceAccount(serviceAccount);
            if (builtIn != null)
            {
                // Stored built-in names are localized; the SID is not.
                return string.Equals(storedName, builtIn.Item1, StringComparison.OrdinalIgnoreCase) ||
                       (storedSid != null && storedSid.Equals(new SecurityIdentifier(builtIn.Item2, null)));
            }

            // The service manager writes local accounts as .\user, which LookupAccountName does not accept.
            if (serviceAccount.StartsWith(".\\"))
            {
                serviceAccount = $"{Environment.MachineName}{serviceAccount.Substring(1)}";
            }

            if (!_nativeMethods.LookupAccountName(serviceAccount, out var serviceUser, out var serviceDomain,
                    out var serviceSid, out _))
            {
                return false;
            }

            return string.Equals(storedName, $"{serviceDomain}\\{serviceUser}", StringComparison.OrdinalIgnoreCase) ||
                   (storedSid != null && storedSid.Equals(serviceSid));
        }

        /// <summary>
        /// Returns the English name and SID type of a built-in account named as the service manager names it, or null.
        /// </summary>
        private static Tuple<string, WellKnownSidType> BuiltInServiceAccount(string account)
        {
            switch (account.ToUpperInvariant())
            {
                case "LOCALSYSTEM":
                case "NT AUTHORITY\\SYSTEM":
                    return Tuple.Create("NT AUTHORITY\\SYSTEM", WellKnownSidType.LocalSystemSid);
                case "LOCALSERVICE":
                case "NT AUTHORITY\\LOCALSERVICE":
                case "NT AUTHORITY\\LOCAL SERVICE":
                    return Tuple.Create("NT AUTHORITY\\LOCAL SERVICE", WellKnownSidType.LocalServiceSid);
                case "NETWORKSERVICE":
                case "NT AUTHORITY\\NETWORKSERVICE":
                case "NT AUTHORITY\\NETWORK SERVICE":
                    return Tuple.Create("NT AUTHORITY\\NETWORK SERVICE", WellKnownSidType.NetworkServiceSid);
                default:
                    return null;
            }
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
            foreach (var value in new[] { "installedDomain", "installedUser", KeepRightsRegistryValueName })
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
        /// Persist the DDAGENTUSER_KEEP_RIGHTS opt-out to the registry so that remote-triggered
        /// upgrades (which inherit only what's on disk, not what was on the command line) can
        /// honor the operator's choice. The value is cleared from the registry when the property
        /// is empty so flipping back to defaults takes a single uninstall/reinstall cycle.
        /// </summary>
        protected void StoreKeepUserRightsInRegistry(IRegistryKey subkey)
        {
            var keepRights = _session.Property(ConfigureUserCustomActions.KeepRightsPropertyName);
            if (!string.IsNullOrEmpty(keepRights))
            {
                _session.Log($"Storing {KeepRightsRegistryValueName}={keepRights}");
                subkey.SetValue(KeepRightsRegistryValueName, keepRights, RegistryValueKind.String);
            }
            else
            {
                try
                {
                    subkey.DeleteValue(KeepRightsRegistryValueName);
                }
                catch (Exception)
                {
                    // Value was not previously set; nothing to do.
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
            _session.Log("FIPS enabled: " + CryptoConfig.AllowOnlyFipsAlgorithms);
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
                    out var nameUse);
                if (!userFound || securityIdentifier == null)
                {
                    throw new Exception($"Could not find account for user {accountName}.");
                }

                // Accept users and the built-in service accounts, never groups such as Everyone.
                var isBuiltInServiceAccount = securityIdentifier.IsWellKnown(WellKnownSidType.LocalSystemSid) ||
                                              securityIdentifier.IsWellKnown(WellKnownSidType.LocalServiceSid) ||
                                              securityIdentifier.IsWellKnown(WellKnownSidType.NetworkServiceSid);
                if (nameUse != SID_NAME_USE.SidTypeUser && !isBuiltInServiceAccount)
                {
                    throw new Exception($"{accountName} is not a user account.");
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
