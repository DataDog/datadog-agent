using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Datadog.CustomActions.Rollback;
using Microsoft.Deployment.WindowsInstaller;
using System;
using System.Collections.Generic;

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
        private readonly RollbackDataStore _rollbackDataStore;

        public InstallOciPackages(ISession session)
        {
            _session = session;
            var installDir = session.Property("PROJECTLOCATION");
            _site = session.Property("SITE");
            _apiKey = session.Property("APIKEY");
            _overrideRegistryUrl = session.Property("DD_INSTALLER_REGISTRY_URL");
            _remoteUpdates = session.Property("DD_REMOTE_UPDATES");
            _installerExecutable = System.IO.Path.Combine(installDir, "bin", "datadog-installer.exe");
            _rollbackDataStore = new RollbackDataStore(session, "InstallOciPackages", new FileSystemServices(), new ServiceController());
        }

        private bool ShouldPurge()
        {
            var purge = _session.Property("PURGE");
            return !string.IsNullOrEmpty(purge) && purge == "1";
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

                // Run the installer setup command with the flavor
                using (var proc = _session.RunCommand(_installerExecutable, "setup --flavor=default", InstallerEnvironmentVariables()))
                {
                    if (proc.ExitCode != 0)
                    {
                        throw new Exception($"'datadog-installer setup --flavor=default' failed with exit code: {proc.ExitCode}");
                    }
                }
                // add purge command to the rollback data store
                // this will only be run on first install, not on upgradet
                _rollbackDataStore.Add(new InstallerPackageRollback("purge"));

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

        private ActionResult PurgePackages()
        {
            if (!ShouldPurge())
            {
                _session.Log("Skipping purge as PURGE is not set to 1");
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
