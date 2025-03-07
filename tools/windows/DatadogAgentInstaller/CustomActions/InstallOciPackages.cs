using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Rollback;
using Microsoft.Deployment.WindowsInstaller;
using System;
using System.IO;
using Datadog.CustomActions.Native;

namespace Datadog.CustomActions
{

    public class InstallOciPackages
    {
        private readonly ISession _session;
        private readonly string _installerExecutable;
        private readonly string _site;
        private readonly RollbackDataStore _rollbackDataStore;

        public InstallOciPackages(ISession session)
        {
            _session = session;
            string installDir = session.Property("INSTALLDIR");
            _site = session.Property("SITE");
            session.Log($"installDir: {installDir}");
            // TODO remove me
            installDir = "C:\\Program Files\\Datadog\\Datadog Installer";
            _installerExecutable = System.IO.Path.Combine(installDir, "datadog-installer.exe");
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

        private (string Name, string Version) ParseVersion(string library)
        {
            library = library.Trim();
            int index = library.IndexOf(',');
            if (index == -1)
            {
                return (library, string.Empty);
            }

            return (library.Substring(0, index), library.Substring(index + 1));
        }

        private ActionResult InstallPackages()
        {
            try
            {
                _session.Log("Installing Oci Packages");
                string instrumentationEnabled = _session.Property("DD_APM_INSTRUMENTATION_ENABLED");
                _session.Log($"instrumentationEnabled: {instrumentationEnabled}");
                if (instrumentationEnabled != "iis")
                {
                    _session.Log("Only DD_APM_INSTRUMENTATION_ENABLED=iis is supported");
                    return ActionResult.Failure;
                }
                string librariesRaw = _session.Property("DD_APM_INSTRUMENTATION_LIBRARIES");
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

        private ActionResult UninstallPackages()
        {
            string librariesRaw = _session.Property("DD_APM_INSTRUMENTATION_LIBRARIES");
            _session.Log($"Uninstalling Oci Packages {librariesRaw}");
            var libraries = librariesRaw.Split(',');
            foreach (var library in libraries)
            {
                var libWithVersion = ParseVersion(library);
                try
                {
                    if (IsPackageInstalled(libWithVersion.Name))
                    {
                        UninstallPackage(libWithVersion.Name);
                    }
                }
                catch (Exception ex)
                {

                    _session.Log($"Error while uninstalling {libWithVersion.Name} library: " + ex.Message);
                }
            }
            return ActionResult.Success;
        }

        private ActionResult RollbackState()
        {
            _rollbackDataStore.Load();
            _rollbackDataStore.Restore();
            return ActionResult.Success;
        }

        private bool IsPackageInstalled(string library)
        {
            string packageName = PackageName(library);
            using (var proc = _session.RunCommand(_installerExecutable, $"is-installed {packageName}"))
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
            string ociImageName = OciImageName(library);
            using (var proc = _session.RunCommand(_installerExecutable, $"install  {PackageUrl(library, version)}"))
            {
                if (proc.ExitCode != 0)
                {
                    throw new Exception($"'datadog-installer install {ociImageName}' failed with exit code: {proc.ExitCode}");
                }
            }
        }

        private void UninstallPackage(string library)
        {
            string packageName = PackageName(library);
            using (var proc = _session.RunCommand(_installerExecutable, $"remove {packageName}"))
            {
                if (proc.ExitCode != 0)
                {
                    throw new Exception($"'datadog-installer remove {packageName}' failed with exit code: {proc.ExitCode}");
                }
            }
        }

        public static ActionResult InstallPackages(Session session)
        {
            return new InstallOciPackages(new SessionWrapper(session)).InstallPackages();
        }

        public static ActionResult UninstallPackages(Session session)
        {
            return new InstallOciPackages(new SessionWrapper(session)).UninstallPackages();
        }

        public static ActionResult RollbackActions(Session session)
        {
            return new InstallOciPackages(new SessionWrapper(session)).RollbackState();
        }
    }
}
