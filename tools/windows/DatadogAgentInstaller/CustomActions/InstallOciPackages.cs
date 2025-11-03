using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Datadog.CustomActions.Rollback;
using Microsoft.Deployment.WindowsInstaller;
using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Text.RegularExpressions;
using Newtonsoft.Json;

namespace Datadog.CustomActions
{

    public class InstallOciPackages
    {
        private readonly ISession _session;
        private readonly string _installerExecutable;
        private readonly string _site;
        private readonly string _apiKey;
        private readonly string _overrideRegistryUrl;
        private readonly string _remoteUpdates;
        private readonly string _infrastructureMode;
        private readonly RollbackDataStore _rollbackDataStore;

        public InstallOciPackages(ISession session)
        {
            _session = session;
            var installDir = session.Property("PROJECTLOCATION");
            _site = session.Property("SITE");
            _apiKey = session.Property("APIKEY");
            _overrideRegistryUrl = session.Property("DD_INSTALLER_REGISTRY_URL");
            _remoteUpdates = session.Property("DD_REMOTE_UPDATES");
            _infrastructureMode = session.Property("DD_INFRASTRUCTURE_MODE");
            _installerExecutable = System.IO.Path.Combine(installDir, "bin", "datadog-installer.exe");
            _rollbackDataStore = new RollbackDataStore(session, "InstallOciPackages", new FileSystemServices(), new ServiceController());
        }

        private bool ShouldPurge()
        {
            var keepInstalledPackages = _session.Property("KEEP_INSTALLED_PACKAGES");
            // KEEP_INSTALLED_PACKAGES=1 means don't purge (keep packages)
            // Default behavior (when not set or set to 0) is to purge
            return string.IsNullOrEmpty(keepInstalledPackages) || keepInstalledPackages != "1";
        }

        private bool ShouldInstall()
        {
            var fleetInstall = _session.Property("FLEET_INSTALL");
            return string.IsNullOrEmpty(fleetInstall) || fleetInstall != "1";
        }

        private Dictionary<string, string> InstallerEnvironmentVariables()
        {
            var env = new Dictionary<string, string>();

            // Skip agent installation - we only want the OCI packages
            env["DD_NO_AGENT_INSTALL"] = "true";

            if (!string.IsNullOrEmpty(_remoteUpdates))
            {
                env["DD_REMOTE_UPDATES"] = _remoteUpdates;
            }
            if (!string.IsNullOrEmpty(_apiKey))
            {
                env["DD_API_KEY"] = _apiKey;
            }
            if (!string.IsNullOrEmpty(_site))
            {
                env["DD_SITE"] = _site;
            }
            if (!string.IsNullOrEmpty(_overrideRegistryUrl))
            {
                env["DD_INSTALLER_REGISTRY_URL"] = _overrideRegistryUrl;
            }

            // Add APM instrumentation configuration
            var instrumentationEnabled = _session.Property("DD_APM_INSTRUMENTATION_ENABLED");
            if (!string.IsNullOrEmpty(instrumentationEnabled))
            {
                env["DD_APM_INSTRUMENTATION_ENABLED"] = instrumentationEnabled;
            }

            var libraries = _session.Property("DD_APM_INSTRUMENTATION_LIBRARIES");
            if (!string.IsNullOrEmpty(libraries))
            {
                env["DD_APM_INSTRUMENTATION_LIBRARIES"] = libraries;
            }
            var apmVersion = _session.Property("DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_APM_INJECT");
            if (!string.IsNullOrEmpty(apmVersion))
            {
                env["DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_APM_INJECT"] = apmVersion;
            }

            if (!string.IsNullOrEmpty(_infrastructureMode))
            {
                env["DD_INFRASTRUCTURE_MODE"] = _infrastructureMode;
            }

            return env;
        }
        private Dictionary<string, string> PurgeEnvironmentVariables()
        {
            var env = new Dictionary<string, string> { { "DD_NO_AGENT_UNINSTALL", "true" } };
            if (!string.IsNullOrEmpty(_apiKey))
            {
                env["DD_API_KEY"] = _apiKey;
            }
            if (!string.IsNullOrEmpty(_site))
            {
                env["DD_SITE"] = _site;
            }
            return env;
        }

        private ActionResult InstallPackages()
        {
            if (!ShouldInstall())
            {
                _session.Log("Skipping install as FLEET_INSTALL is set to 1");
                return ActionResult.Success;
            }
            try
            {
                _session.Log("Running datadog-installer setup");
                var instrumentationEnabled = _session.Property("DD_APM_INSTRUMENTATION_ENABLED");
                var librariesRaw = _session.Property("DD_APM_INSTRUMENTATION_LIBRARIES");
                _session.Log($"instrumentationEnabled: {instrumentationEnabled}");

                // add purge command to the rollback data store
                // this will only be run on first install, not on upgrade
                // if install command fails we still wanna purge our packages
                _rollbackDataStore.Add(new InstallerPackageRollback("purge"));

                // If DD_OTEL_OCI_INSTALL is provided, validate and install DDOT explicitly
                var ddotInstallTargetRaw = _session.Property("DD_OTEL_OCI_INSTALL");
                if (!string.IsNullOrEmpty(ddotInstallTargetRaw))
                {
                    _session.Log($"DD_OTEL_OCI_INSTALL provided: {ddotInstallTargetRaw}");
                    var expectedTag = GetExpectedAgentOciTag();
                    if (string.IsNullOrEmpty(expectedTag))
                    {
                        _session.Log("Failed to determine expected Agent version tag");
                        return ActionResult.Failure;
                    }

                    if (!TryNormalizeDdOtUrl(ddotInstallTargetRaw, expectedTag, out var normalizedUrl, out var normalizeErr))
                    {
                        _session.Log($"Failed to normalize DD_OTEL_OCI_INSTALL: {normalizeErr}");
                        return ActionResult.Failure;
                    }

                    _session.Log($"Installing DDOT from: {normalizedUrl}");
                    var env = InstallerEnvironmentVariables();
                    var installArgs = $"install \"{normalizedUrl}\"";
                    using (var procInstall = _session.RunCommand(_installerExecutable, installArgs, env))
                    {
                        if (procInstall.ExitCode != 0)
                        {
                            _session.Log($"'datadog-installer {installArgs}' failed with exit code: {procInstall.ExitCode}");
                            return ActionResult.Failure;
                        }
                    }

                    // Post-install verification: ensure ddot stable equals expectedTag
                    var statesJson = RunAndCapture(_installerExecutable, "get-states", env, out var exitCodeStates);
                    if (exitCodeStates != 0 || string.IsNullOrEmpty(statesJson))
                    {
                        _session.Log("Failed to retrieve installer states for verification");
                        return ActionResult.Failure;
                    }
                    if (!IsDdotVersionExpected(statesJson, out var installedStable))
                    {
                        _session.Log("Could not find DDOT package state in get-states output");
                        return ActionResult.Failure;
                    }
                    if (!string.Equals(installedStable, expectedTag, StringComparison.Ordinal))
                    {
                        _session.Log($"DDOT installed stable '{installedStable}' does not match expected '{expectedTag}'. Removing package.");
                        using (var procRemove = _session.RunCommand(_installerExecutable, "remove datadog-ddot", env))
                        {
                            // ignore remove failures; we still fail the action
                        }
                        return ActionResult.Failure;
                    }
                }

                // Run the installer setup command with the flavor
                using (var proc = _session.RunCommand(_installerExecutable, "setup --flavor=default", InstallerEnvironmentVariables()))
                {
                    if (proc.ExitCode != 0)
                    {
                        throw new Exception($"'datadog-installer setup --flavor=default' failed with exit code: {proc.ExitCode}");
                    }
                }

                return ActionResult.Success;
            }
            catch (Exception ex)
            {
                _session.Log("Error while running installer setup: " + ex.Message);
                return ActionResult.Failure;
            }
            finally
            {
                _rollbackDataStore.Store();
            }
        }

        private static string NormalizeAgentVersionToOciTag(string raw)
        {
            if (string.IsNullOrEmpty(raw))
            {
                return raw;
            }
            var v = raw.Trim();
            v = v.Replace('+', '.');
            v = v.Replace('~', '-');
            if (!v.EndsWith("-1", StringComparison.Ordinal))
            {
                v = v + "-1";
            }
            return v;
        }

        private string GetExpectedAgentOciTag()
        {
            try
            {
                var output = RunAndCapture(_installerExecutable, "version", null, out var exitCode);
                if (exitCode != 0)
                {
                    _session.Log($"datadog-installer version returned exit {exitCode}");
                    return null;
                }
                return NormalizeAgentVersionToOciTag(output);
            }
            catch (Exception e)
            {
                _session.Log($"Error determining installer version: {e.Message}");
                return null;
            }
        }

        private static bool TryNormalizeDdOtUrl(string input, string expectedTag, out string normalized, out string error)
        {
            normalized = null;
            error = null;
            if (string.IsNullOrEmpty(input))
            {
                error = "empty input";
                return false;
            }

            // Local path support -> convert to file:/// URI
            if (!input.StartsWith("oci://", StringComparison.OrdinalIgnoreCase) &&
                !input.StartsWith("file://", StringComparison.OrdinalIgnoreCase))
            {
                // If it looks like a Windows path or UNC, treat as local
                if (Path.IsPathRooted(input) || input.StartsWith("\\\\"))
                {
                    var uriPath = input.Replace('\\', '/');
                    if (!uriPath.StartsWith("/"))
                    {
                        uriPath = "/" + uriPath;
                    }
                    normalized = $"file://{uriPath}";
                    return true;
                }
                error = "unsupported URL format (expected oci:// or file:// or absolute path)";
                return false;
            }

            if (input.StartsWith("file://", StringComparison.OrdinalIgnoreCase))
            {
                normalized = input;
                return true; // version will be verified post-install
            }

            // oci:// URL normalization
            var url = input;
            var lastSlash = url.LastIndexOf('/') + 1;
            if (lastSlash <= 0 || lastSlash >= url.Length)
            {
                error = "invalid oci url";
                return false;
            }

            // Enforce canonical image name: ddot-package
            var endOfName = url.Length;
            var atPos = url.IndexOf('@', lastSlash);
            if (atPos != -1)
            {
                endOfName = atPos;
            }
            var colonPos = url.IndexOf(':', lastSlash);
            if (colonPos != -1 && (atPos == -1 || colonPos < atPos))
            {
                endOfName = colonPos;
            }
            var imageName = url.Substring(lastSlash, endOfName - lastSlash);
            if (!imageName.Equals("ddot-package", StringComparison.OrdinalIgnoreCase))
            {
                error = "invalid ddot image name; expected 'ddot-package'";
                return false;
            }

            // Tag handling
            lastSlash = url.LastIndexOf('/') + 1;
            colonPos = url.IndexOf(':', lastSlash);
            atPos = url.IndexOf('@', lastSlash);
            if (colonPos == -1 && atPos == -1)
            {
                // No tag provided -> append expected tag
                url = url + ":" + expectedTag;
            }
            else if (colonPos != -1)
            {
                var tagEnd = atPos == -1 ? url.Length : atPos;
                var tag = url.Substring(colonPos + 1, tagEnd - (colonPos + 1));
                if (!string.Equals(tag, expectedTag, StringComparison.Ordinal))
                {
                    error = $"provided tag '{tag}' does not match expected '{expectedTag}'";
                    return false;
                }
            }

            normalized = url;
            return true;
        }

        private static string RunAndCapture(string fileName, string arguments, IDictionary<string, string> environment, out int exitCode)
        {
            var psi = new ProcessStartInfo
            {
                CreateNoWindow = true,
                UseShellExecute = false,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                FileName = fileName,
                Arguments = arguments
            };
            if (environment != null)
            {
                foreach (var kvp in environment)
                {
                    psi.Environment[kvp.Key] = kvp.Value;
                }
            }
            using (var proc = new Process { StartInfo = psi })
            {
                proc.Start();
                var stdout = proc.StandardOutput.ReadToEnd();
                var stderr = proc.StandardError.ReadToEnd();
                proc.WaitForExit();
                exitCode = proc.ExitCode;
                return stdout.Trim();
            }
        }

        private static bool IsDdotVersionExpected(string statesJson, out string stable)
        {
            stable = null;
            try
            {
                var root = JsonConvert.DeserializeObject<InstallerStates>(statesJson);
                if (root?.States == null)
                {
                    return false;
                }
                if (root.States.TryGetValue("datadog-agent-ddot", out var state) && !string.IsNullOrEmpty(state.Stable))
                {
                    stable = state.Stable;
                    return true;
                }
            }
            catch
            {
                // ignore
            }
            return false;
        }

        private class InstallerStates
        {
            public Dictionary<string, PackageState> States { get; set; }
        }

        private class PackageState
        {
            public string Stable { get; set; }
            public string Experiment { get; set; }
        }

        private ActionResult PurgePackages()
        {
            if (!ShouldPurge())
            {
                _session.Log("Skipping purge as KEEP_INSTALLED_PACKAGES is set to 1");
                return ActionResult.Success;
            }
            var fleetInstall = _session.Property("FLEET_INSTALL");
            if (!string.IsNullOrEmpty(fleetInstall) && fleetInstall == "1")
            {
                _session.Log("Skipping purge as FLEET_INSTALL is set to 1");
                return ActionResult.Success;
            }
            try
            {
                _session.Log("Running datadog-installer purge");
                var env = PurgeEnvironmentVariables();
                using (var proc = _session.RunCommand(_installerExecutable, "purge", env))
                {
                    if (proc.ExitCode != 0)
                    {
                        _session.Log($"'datadog-installer purge' failed with exit code: {proc.ExitCode}");
                        return ActionResult.Failure;
                    }
                }
                return ActionResult.Success;
            }
            catch (Exception ex)
            {
                _session.Log("Error while running installer purge: " + ex.Message);
                return ActionResult.Failure;
            }
        }

        private ActionResult RollbackState()
        {
            _rollbackDataStore.Load();
            _rollbackDataStore.Restore();
            return ActionResult.Success;
        }

        public static ActionResult InstallPackages(Session session)
        {
            return new InstallOciPackages(new SessionWrapper(session)).InstallPackages();
        }

        public static ActionResult RollbackActions(Session session)
        {
            return new InstallOciPackages(new SessionWrapper(session)).RollbackState();
        }

        public static ActionResult PurgePackages(Session session)
        {
            return new InstallOciPackages(new SessionWrapper(session)).PurgePackages();
        }
    }
}
