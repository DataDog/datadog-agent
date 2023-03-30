using System;
using Datadog.CustomActions;
using WixSharp;
using WixSetup.Datadog;

namespace WixSetup
{
    internal abstract class Program
    {
        private static void BuildMsi(string version = null)
        {
            // Print this line during the CI build so we can check that the CiInfo class picked up the
            // %PACKAGE_VERSION% environment variable correctly.
            Console.WriteLine($"Building MSI installer for Datadog Agent version {CiInfo.PackageVersion}");
            var project = new AgentInstaller(version)
                .ConfigureProject();

#if DEBUG
            // Save a copy of the WXS for analysis since WixSharp deletes it after it's done generating the MSI.
            project.WixSourceSaved += path =>
            {
                System.IO.File.Copy(path, "wix/WixSetup.g.wxs", overwrite: true);
            };
#endif

            project
                .BuildMsi();
        }

        private static void Main()
        {
            Compiler.LightOptions += "-sval -reusecab -cc \"cabcache\"";
            // ServiceConfig functionality is documented in the Windows Installer SDK to "not [work] as expected." Consider replacing ServiceConfig with the WixUtilExtension ServiceConfig element.
            Compiler.CandleOptions += "-sw1150";

#if DEBUG
            // Useful to produce multiple versions of the installer for testing.
            BuildMsi("7.43.0~rc.3+git.485.14b9337");
#endif
            BuildMsi();
        }
    }

}
