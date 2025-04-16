using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Datadog.CustomActions.Rollback;
using Microsoft.Deployment.WindowsInstaller;
using System;
using System.Collections.Generic;
using System.IO;

namespace Datadog.CustomActions
{

    public class InstallOciPackages
    {
        private readonly ISession _session;
        private readonly string _installerExecutable;
        private readonly string _site;
        private readonly string _apiKey;
        private readonly string _overrideRegistryUrl;
        private readonly RollbackDataStore _rollbackDataStore;

        public InstallOciPackages(ISession session)
        {
            _session = session;
            var installDir = session.Property("PROJECTLOCATION");
            _site = session.Property("SITE");
            _apiKey = session.Property("APIKEY");
            _overrideRegistryUrl = session.Property("DD_INSTALLER_REGISTRY_URL");
            _installerExecutable = System.IO.Path.Combine(installDir, "bin", "datadog-installer.exe");
            _rollbackDataStore = new RollbackDataStore(session, "InstallOciPackages", new FileSystemServices(), new ServiceController());
        }


        private string PackageName(string language)
        {
            return $"datadog-apm-library-{language}";
        }

        private string OciImageName(string language)
        {
            return $"apm-library-{language}-package";
        }

        private string PackageUrl(string language, string version)
        {
            if (_site == "datad0g.com")
            {
                return $"oci://install.datad0g.com/{OciImageName(language)}:{version}";
            }
            else
            {
                return $"oci://install.datadoghq.com/{OciImageName(language)}:{version}";
            }
        }

        private NameVersionPair ParseVersion(string library)
        {
            library = library.Trim();
            var index = library.IndexOf(':');
            if (index == -1)
            {
                return new NameVersionPair(library, string.Empty);
            }

            return new NameVersionPair(library.Substring(0, index), library.Substring(index + 1));
        }

        private Dictionary<string, string> InstallerEnvironmentVariables()
        {
            var env = new Dictionary<string, string>();
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
            return env;
        }

        private ActionResult InstallPackages()
        {
            try
            {
                _session.Log("Installing Oci Packages");
                var instrumentationEnabled = _session.Property("DD_APM_INSTRUMENTATION_ENABLED");
                var librariesRaw = _session.Property("DD_APM_INSTRUMENTATION_LIBRARIES");
                _session.Log($"instrumentationEnabled: {instrumentationEnabled}");
                if (string.IsNullOrEmpty(instrumentationEnabled) || string.IsNullOrEmpty(librariesRaw))
                {
                    _session.Log($"instrumentation is disabled or no library is provided skipping");
                    return ActionResult.Success;
                }
                if (instrumentationEnabled != "iis")
                {
                    _session.Log("Only DD_APM_INSTRUMENTATION_ENABLED=iis is supported");
                    return ActionResult.Failure;
                }
                var libraries = librariesRaw.Split(',');
                foreach (var library in libraries)
                {
                    var libWithVersion = ParseVersion(library);
                    if (!IsPackageInstalled(libWithVersion.Name))
                    {
                        _rollbackDataStore.Add(
                            new InstallerPackageRollback($"remove {PackageName(libWithVersion.Name)}"));
                    }

                    InstallPackage(libWithVersion.Name, libWithVersion.Version);
                }

                return ActionResult.Success;
            }
            catch (Exception ex)
            {
                _session.Log("Error while installing oci package: " + ex.Message);
                return ActionResult.Failure;
            }
            finally
            {
                _rollbackDataStore.Store();
            }
        }

        private ActionResult RollbackState()
        {
            _rollbackDataStore.Load();
            _rollbackDataStore.Restore();
            return ActionResult.Success;
        }

        private bool IsPackageInstalled(string library)
        {
            var packageName = PackageName(library);
            using (var proc = _session.RunCommand(_installerExecutable, $"is-installed {packageName}", InstallerEnvironmentVariables()))
            {
                if (proc.ExitCode == 10)
                {
                    return false;
                }

                if (proc.ExitCode != 0)
                {
                    throw new Exception($"'datadog-installer is-installed {packageName}' failed with exit code: {proc.ExitCode}");
                }
                return true;
            }
        }

        private void InstallPackage(string library, string version)
        {
            var ociImageName = OciImageName(library);
            using (var proc = _session.RunCommand(_installerExecutable, $"install  {PackageUrl(library, version)}", InstallerEnvironmentVariables()))
            {
                if (proc.ExitCode != 0)
                {
                    throw new Exception($"'datadog-installer install {ociImageName}' failed with exit code: {proc.ExitCode}");
                }
            }
        }

        public static ActionResult InstallPackages(Session session)
        {
            return new InstallOciPackages(new SessionWrapper(session)).InstallPackages();
        }

        public static ActionResult RollbackActions(Session session)
        {
            return new InstallOciPackages(new SessionWrapper(session)).RollbackState();
        }

        private class NameVersionPair
        {
            public NameVersionPair(string name, string version)
            {
                Name = name;
                Version = version;
            }

            public string Name { get; }
            public string Version { get; }
        }
    }
}
