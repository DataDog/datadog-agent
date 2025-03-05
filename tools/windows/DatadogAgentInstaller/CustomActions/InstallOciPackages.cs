using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Microsoft.Deployment.WindowsInstaller;
using System;
using System.IO;

namespace Datadog.CustomActions
{

    public class InstallOciPackages
    {
        private readonly ISession _session;
        private readonly string _installerExecutable;
        private readonly string _site;

        public InstallOciPackages(ISession session)
        {
            _session = session;
            string installDir = session.Property("INSTALLDIR");
            _site = session.Property("SITE");
            session.Log($"installDir: {installDir}");
            // TODO remove me
            installDir = "C:\\Program Files\\Datadog\\Datadog Installer";
            _installerExecutable = System.IO.Path.Combine(installDir, "datadog-installer.exe");
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
                    if (IsPackageInstalled(libWithVersion.Name))
                    {
                        // TODO rollback should not do anything
                    }
                    else
                    {
                        // TODO rollback should uninstall the library
                    }
                    InstallPackage(libWithVersion.Name, libWithVersion.Version);
                }
                // session.Log($"libraries: {libraries}");
                // if (libraries != "dotnet" || instrumentationEnabled != "iis")
                // {
                //     session.Log("Skipping dotnet library installation");
                //     return ActionResult.Success;
                // }
                // session.Log("Installing dotnet library");
                // session.Log($"installer executable path: {exePath}");
                // // TODO: Replace the version and read from disk
                // session.RunCommand(exePath, "install oci://install.datad0g.com/apm-library-dotnet-package:3.12");
                // // TODO: Should we use RollbackDataStore to store the rollback instructions
                return ActionResult.Success;
            }
            catch (Exception ex)
            {
                _session.Log("Error while installing dotnet library: " + ex.Message);
                return ActionResult.Failure;
            }
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
            _session.RunCommand(_installerExecutable, $"install  {PackageUrl(library, version)}");
        }

        // private void UpdatePackage(string library, string version)
        // {
        //     string ociImageName = OciImageName(library);
        //     string packageName = PackageName(library);
        //     try
        //     {
        //         _session.RunCommand(_installerExecutable, $"promote-experiment {packageName}");
        //     }
        //     catch (Exception ex)
        //     {
        //         // If there is no experiment this will fail
        //         session.Log($"Promoting experiment for {library} failed with: " + ex.Message);
        //         session.Log($"Continuing with {library} installation");
        //     }
        //     _session.RunCommand(_installerExecutable, $"install-experiment {_ociRepository}/{ociImageName}:{version}");
        // }

        public static ActionResult InstallPackages(Session session)
        {
            return new InstallOciPackages(new SessionWrapper(session)).InstallPackages();
        }
    }
}
