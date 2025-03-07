using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Newtonsoft.Json;
using System;
using System.IO;

namespace Datadog.CustomActions.Rollback
{
    public class InstallerPackageRollback : IRollbackAction
    {
        [JsonProperty("InstallerCommand")] private string _installerCommand;

        [JsonConstructor]
        public InstallerPackageRollback()
        {
        }

        public InstallerPackageRollback(string installerCommand)
        {
            _installerCommand = installerCommand;
        }

        public void Restore(ISession session, IFileSystemServices fileSystemServices,
            IServiceController serviceController)
        {
            string installDir = session.Property("INSTALLDIR");
            session.Log($"installDir: {installDir}");
            // TODO remove me
            installDir = "C:\\Program Files\\Datadog\\Datadog Installer";
            string installerExecutable = System.IO.Path.Combine(installDir, "datadog-installer.exe");
            using (var proc = session.RunCommand(installerExecutable, _installerCommand))
            {
                if (proc.ExitCode != 0)
                {
                    session.Log(
                        $"error running rollback command {installerExecutable} {_installerCommand} failed with exit code {proc.ExitCode}");
                }
            }
        }
    }
}
