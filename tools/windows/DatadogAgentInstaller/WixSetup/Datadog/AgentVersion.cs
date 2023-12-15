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
        : this(Environment.GetEnvironmentVariable("PACKAGE_VERSION"))
        {

        }

        public AgentVersion(string packageVersion)
        {
            PackageVersion = packageVersion;
            Version = new Version(7, 99, 0, 0);
            if (PackageVersion != null)
            {
                var versionRegex = new Regex(@"(?<major>\d+)[.](?<minor>\d+)[.](?<build>\d+)([-~]rc.(?<rc>\d+))?");
                var versionMatch = versionRegex.Match(PackageVersion);
                if (versionMatch.Groups["rc"].Success)
                {
                    Version = new Version(
                        major: versionMatch.Groups["major"].Value.ToInt(),
                        minor: versionMatch.Groups["minor"].Value.ToInt(),
                        build: versionMatch.Groups["build"].Value.ToInt(),
                        revision: versionMatch.Groups["rc"].Value.ToInt()
                    );
                }
                else
                {
                    Version = new Version(
                        major: versionMatch.Groups["major"].Value.ToInt(),
                        minor: versionMatch.Groups["minor"].Value.ToInt(),
                        build: versionMatch.Groups["build"].Value.ToInt(),
                        revision: 0
                    );
                }
            }
            else
            {
                PackageVersion = $"{Version.Major}.{Version.Minor}.{Version.Build}";
            }
        }
    }
}
