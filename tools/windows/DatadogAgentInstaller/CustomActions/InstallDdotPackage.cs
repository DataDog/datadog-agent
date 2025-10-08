using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Rollback;
using Datadog.CustomActions.Native;
using Microsoft.Deployment.WindowsInstaller;
using System;
using System.Collections.Generic;
using System.IO;

namespace Datadog.CustomActions
{
    public class InstallDdotPackage
    {
        private readonly ISession _session;
        private readonly string _installerExecutable;
        private readonly string _site;
        private readonly string _apiKey;
        private readonly string _overrideRegistryUrl;
        private readonly RollbackDataStore _rollbackDataStore;

        public InstallDdotPackage(ISession session)
        {
            _session = session;
            var installDir = session.Property("PROJECTLOCATION");
            _site = session.Property("SITE");
            _apiKey = session.Property("APIKEY");
            _overrideRegistryUrl = session.Property("DD_INSTALLER_REGISTRY_URL");
            _installerExecutable = Path.Combine(installDir, "bin", "datadog-installer.exe");
            _rollbackDataStore = new RollbackDataStore(session, "InstallDdotPackage", new FileSystemServices(), new ServiceController());
        }

        private string PackageName()
        {
            return "datadog-agent-ddot";
        }

        private string OciImageName()
        {
            return "ddot-package";
        }

        private string PackageUrl(string version)
        {
            var tag = string.IsNullOrEmpty(version) ? "latest" : version.Trim();
            if (_site == "datad0g.com")
            {
                return $"oci://install.datad0g.com/{OciImageName()}:{tag}";
            }
            else
            {
                return $"oci://install.datadoghq.com/{OciImageName()}:{tag}";
            }
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

        private bool IsPackageInstalled()
        {
            var name = PackageName();
            using (var proc = _session.RunCommand(_installerExecutable, $"is-installed {name}", InstallerEnvironmentVariables()))
            {
                if (proc.ExitCode == 10)
                {
                    return false;
                }

                if (proc.ExitCode != 0)
                {
                    throw new Exception($"'datadog-installer is-installed {name}' failed with exit code: {proc.ExitCode}");
                }
                return true;
            }
        }

        private void InstallPackage(string version)
        {
            using (var proc = _session.RunCommand(_installerExecutable, $"install {PackageUrl(version)}", InstallerEnvironmentVariables()))
            {
                if (proc.ExitCode != 0)
                {
                    throw new Exception($"'datadog-installer install {OciImageName()}' failed with exit code: {proc.ExitCode}");
                }
            }
        }

        private void RemovePackage()
        {
            var name = PackageName();
            using (var proc = _session.RunCommand(_installerExecutable, $"remove {name}", InstallerEnvironmentVariables()))
            {
                if (proc.ExitCode != 0)
                {
                    throw new Exception($"'datadog-installer remove {name}' failed with exit code: {proc.ExitCode}");
                }
            }
        }

        private ActionResult InstallInternal()
        {
            try
            {
                _session.Log("Installing DDOT package (if enabled)");
                var enabled = _session.Property("DD_DDOT_ENABLED");
                var version = _session.Property("DD_DDOT_VERSION");
                _session.Log($"DD_DDOT_ENABLED={enabled}, DD_DDOT_VERSION={version}");

                if (string.IsNullOrEmpty(enabled) || enabled != "1")
                {
                    _session.Log("DDOT install disabled or not requested; skipping");
                    return ActionResult.Success;
                }

                if (!IsPackageInstalled())
                {
                    _rollbackDataStore.Add(new InstallerPackageRollback($"remove {PackageName()}"));
                }

                InstallPackage(version);
                return ActionResult.Success;
            }
            catch (Exception ex)
            {
                _session.Log("Error while installing DDOT package: " + ex.Message);
                return ActionResult.Failure;
            }
            finally
            {
                _rollbackDataStore.Store();
            }
        }

        private ActionResult UninstallInternal()
        {
            try
            {
                _session.Log("UninstallDdotPackage: attempting removal if installed");
                var explicitUninstall = _session.Property("DD_DDOT_UNINSTALL");
                _session.Log($"DD_DDOT_UNINSTALL={explicitUninstall}");
                if (IsPackageInstalled())
                {
                    RemovePackage();
                }
                return ActionResult.Success;
            }
            catch (Exception ex)
            {
                _session.Log("Error while uninstalling DDOT package: " + ex.Message);
                return ActionResult.Failure;
            }
        }

        private ActionResult RollbackState()
        {
            _rollbackDataStore.Load();
            _rollbackDataStore.Restore();
            return ActionResult.Success;
        }

        public static ActionResult Install(Session session)
        {
            return new InstallDdotPackage(new SessionWrapper(session)).InstallInternal();
        }

        public static ActionResult Uninstall(Session session)
        {
            return new InstallDdotPackage(new SessionWrapper(session)).UninstallInternal();
        }

        public static ActionResult Rollback(Session session)
        {
            return new InstallDdotPackage(new SessionWrapper(session)).RollbackState();
        }
    }
}