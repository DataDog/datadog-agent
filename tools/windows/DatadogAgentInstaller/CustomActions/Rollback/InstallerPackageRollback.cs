using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Newtonsoft.Json;
using System;
using System.Collections.Generic;
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
            var installDir = session.Property("PROJECTLOCATION");
            var installerExecutable = System.IO.Path.Combine(installDir, "bin", "datadog-installer.exe");

            var installerEnvVariables = new Dictionary<string, string>();
            installerEnvVariables["DD_API_KEY"] = session.Property("APIKEY");
            installerEnvVariables["DD_SITE"] = session.Property("SITE");
            // set the environment variable to prevent the agent from being uninstalled
            // as the MSI will handle the agent installation and state
            installerEnvVariables["DD_NO_AGENT_UNINSTALL"] = "true";

            using (var proc = session.RunCommand(installerExecutable, _installerCommand, installerEnvVariables))
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
