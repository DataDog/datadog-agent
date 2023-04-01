using System;
using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Microsoft.Deployment.WindowsInstaller;
using Microsoft.Win32;

namespace Datadog.CustomActions
{
    // Fetch and process registry value(s) and return a string to be assigned to a WIX property.
    using GetRegistryPropertyHandler = Func<string>;

    public class RegistryCustomActions
    {
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
        private static void RegistryValueProperty(ISession session, string propertyName, RegistryKey registryKey, string registryValue)
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
        private static ActionResult ReadRegistryProperties(ISession session)
        {
            try
            {
                using (var subkey = Registry.LocalMachine.OpenSubKey(@"Software\Datadog\Datadog Agent"))
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
                        RegistryProperty(session, "DDAGENTUSER_NAME",
                            () =>
                            {
                                var domain = subkey.GetValue("installedDomain").ToString();
                                var user = subkey.GetValue("installedUser").ToString();
                                if (!string.IsNullOrEmpty(domain) && !string.IsNullOrEmpty(user))
                                {
                                    return $"{domain}\\{user}";
                                }
                                return string.Empty;
                            });

                        RegistryValueProperty(session, "PROJECTLOCATION", subkey, "InstallPath");
                        RegistryValueProperty(session, "APPLICATIONDATADIRECTORY", subkey, "ConfigRoot");
                        RegistryValueProperty(session, "ALLOWCLOSEDSOURCE", subkey, "AllowClosedSource");
                    }
                }

                if (string.IsNullOrEmpty(session.Property("ALLOWCLOSEDSOURCE")))
                {
                    session.Log("Cannot find the \"AllowClosedSource\" registry key, checking the NPM service state.");
                    using var subkey = Registry.LocalMachine.OpenSubKey(@"SYSTEM\CurrentControlSet\Services\ddnpm");
                    if (subkey != null)
                    {
                        // https://renenyffenegger.ch/notes/Windows/registry/tree/HKEY_LOCAL_MACHINE/System/CurrentControlSet/Services/_driver_name_/index
                        // Start = 4 means service is disabled, so NOT 4 means it's enabled.
                        session["ALLOWCLOSEDSOURCE"] = int.Parse(subkey.GetValue("Start").ToString()) != 4 ? "1" : "0";
                        session.Log($"Found \"AllowClosedSource\" key, with value: {session["ALLOWCLOSEDSOURCE"]}");
                    }
                    else
                    {
                        session.Log("NPM service not found, assuming closed source consent was not given.");
                    }
                }

                using (var subkey = Registry.LocalMachine.OpenSubKey(@"Software\Microsoft\Windows NT\CurrentVersion"))
                {
                    if (subkey != null)
                    {
                        session["WindowsBuild"] = subkey.GetValue("CurrentBuild").ToString();
                        session.Log($"WindowsBuild: {session["WindowsBuild"]}");
                    }
                    else
                    {
                        session.Log("WindowsBuild not found");
                    }
                }

            }
            catch (Exception e)
            {
                session.Log($"Error processing registry properties: {e}");
                return ActionResult.Failure;
            }
            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult ReadRegistryProperties(Session session)
        {
            return ReadRegistryProperties(new SessionWrapper(session));
        }
    }
}
