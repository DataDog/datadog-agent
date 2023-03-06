using System;
using Datadog.CustomActions;
using WixSharp;
using WixSetup.Datadog;

namespace WixSetup
{
    internal class Program
    {
        private static void Main()
        {
            // Print this line during the CI build so we can check that the CiInfo class picked up the
            // %PACKAGE_VERSION% environment variable correctly.
            Console.WriteLine($"Building MSI installer for Datadog Agent version {CiInfo.PackageVersion}");
            var installer = new AgentInstaller();
            Compiler.LightOptions += "-sval -reusecab -cc \"cabcache\"";
            // ServiceConfig functionality is documented in the Windows Installer SDK to "not [work] as expected." Consider replacing ServiceConfig with the WixUtilExtension ServiceConfig element.
            Compiler.CandleOptions += "-sw1150";
            installer.ConfigureProject().BuildMsi();
        }
    }

}
