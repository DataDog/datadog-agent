using System;
using System.Text.RegularExpressions;
using WixSharp;

namespace WixSetup.Datadog
{
    public class AgentVersion
    {
        public Version Version { get; }

        public string PackageVersion { get; }

        public AgentVersion()
        {
            Version = new Version(7, 99, 0, 2);
            PackageVersion = Environment.GetEnvironmentVariable("PACKAGE_VERSION");
            if (PackageVersion != null)
            {
                var versionRegex = new Regex(@"(?<major>\d+)[.](?<minor>\d+)[.](?<build>\d+)");
                var versionMatch = versionRegex.Match(PackageVersion);
                Version = new Version(
                    versionMatch.Groups["major"].Value.ToInt(),
                    versionMatch.Groups["minor"].Value.ToInt(),
                    2 // Use 1 once we replace the main installer with this one.
                );
            }
            else
            {
                PackageVersion = $"{Version.Major}.{Version.Minor}.{Version.Build}";
            }
        }
    }
}
