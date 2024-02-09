using System;
using System.IO;
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

            if (!string.IsNullOrEmpty(Environment.GetEnvironmentVariable("BUILD_MSI_CMD")))
            {
                // In the CI, tasks/msi.py uses the BuildMsiCmd to be able to sign the binaries before building the MSI
                project.BuildMsiCmd();
            }
            else
            {
                // When building in vstudio, build the MSI directly
                // Save a copy of the WXS for analysis since WixSharp deletes it after it's done generating the MSI.
                project.WixSourceSaved += path =>
                {
                    System.IO.File.Copy(path, "wix/WixSetup.g.wxs", overwrite: true);
                };
                project.BuildMsi();
            }
        }

        private static void Main()
        {
            var cabcachedir = "cabcache";
            if (!string.IsNullOrEmpty(Environment.GetEnvironmentVariable("AGENT_MSI_OUTDIR")))
            {
                // Set custom output directory (WixSharp defaults to current directory)
                cabcachedir = Path.Combine(Environment.GetEnvironmentVariable("AGENT_MSI_OUTDIR"), cabcachedir);
            }
            Compiler.LightOptions += $"-sval -reusecab -cc \"{cabcachedir}\"";
            // ServiceConfig functionality is documented in the Windows Installer SDK to "not [work] as expected." Consider replacing ServiceConfig with the WixUtilExtension ServiceConfig element.
            Compiler.CandleOptions += "-sw1150";

            // We don't use WixUI_InstallDir, so disable Wix# auto-handling of the INSTALLDIR property for this UI.
            // If we ever change this and want to use the Wix# auto-detection, we will have to set this value to
            // the actual installdir property (currently PROJECTLOCATION) or use `new InstallDir()` instead of `new Dir()`.
            // Current UI docs: https://www.firegiant.com/wix/tutorial/user-interface-revisited/customizations-galore/
            // WixUI_InstallDir docs: https://wixtoolset.org/docs/v3/wixui/dialog_reference/wixui_installdir/
            Compiler.AutoGeneration.InstallDirDefaultId = null;
#if false
            // Useful to produce multiple versions of the installer for testing.
            BuildMsi("7.43.0~rc.3+git.485.14b9337");
#endif
            BuildMsi();
        }
    }

}
