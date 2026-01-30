using Datadog.CustomActions;
using System;
using System.IO;
using WixSetup.Datadog_Agent;
using WixSetup.Datadog_Installer;
using WixSharp;
using Action = System.Action;

namespace WixSetup
{
    internal abstract class Program
    {
        private static string BuildMsi<TInstaller>(Action msiSpecificAction) where TInstaller : IMsiInstallerProject, new()
        {
            msiSpecificAction();

            var project = new TInstaller().Configure();

            if (!string.IsNullOrEmpty(Environment.GetEnvironmentVariable("BUILD_MSI_CMD")))
            {
                // In the CI, tasks/msi.py uses the BuildMsiCmd to be able to sign the binaries before building the MSI
                return project.BuildMsiCmd();
            }

            // When building in vstudio, build the MSI directly
            // Save a copy of the WXS for analysis since WixSharp deletes it after it's done generating the MSI.
            project.WixSourceSaved += path =>
            {
                System.IO.File.Copy(path, $"wix/{typeof(TInstaller).Name}.g.wxs", overwrite: true);
            };

            return project.BuildMsi();
        }

        private static void Main()
        {
            var cabcachedir = "cabcache";
            if (!string.IsNullOrEmpty(Environment.GetEnvironmentVariable("AGENT_MSI_OUTDIR")))
            {
                // Set custom output directory (WixSharp defaults to current directory)
                cabcachedir = Path.Combine(Environment.GetEnvironmentVariable("AGENT_MSI_OUTDIR"), cabcachedir);
            }

            // WiX 5 migration: LightOptions and CandleOptions are obsolete in WixSharp_wix4.
            // WiX 5 uses unified 'wix build' command via Compiler.WixOptions.
            // 
            // Option migration from WiX 3:
            // - -sval: NOT NEEDED in WiX 5 (validation is separate: 'wix msi validate')
            // - -reusecab: Implicit in WiX5's -cabcache behavior (automatic reuse)
            // - -cc <dir>: Cabinet caching directory (still supported in WiX 5)
            // - -sw1150: NOT SUPPORTED in WiX 5 command line (warning suppression is MSBuild-only)
            // - -arch x64: Not needed in WixOptions; handled by project.Platform = Platform.x64
            Compiler.WixOptions += $"-cc \"{cabcachedir}\" ";

            // We don't use WixUI_InstallDir, so disable Wix# auto-handling of the INSTALLDIR property for this UI.
            // If we ever change this and want to use the Wix# auto-detection, we will have to set this value to
            // the actual installdir property (currently PROJECTLOCATION) or use `new InstallDir()` instead of `new Dir()`.
            // Current UI docs: https://www.firegiant.com/wix/tutorial/user-interface-revisited/customizations-galore/
            // WixUI_InstallDir docs: https://wixtoolset.org/docs/v3/wixui/dialog_reference/wixui_installdir/
            Compiler.AutoGeneration.InstallDirDefaultId = null;

            var omnibusTarget = Environment.GetEnvironmentVariable("OMNIBUS_TARGET");
            if (string.IsNullOrEmpty(omnibusTarget) || omnibusTarget == "main")
            {
                BuildMsi<AgentInstaller>(() =>
                {
                    // Print this line during the CI build so we can check that the CiInfo class picked up the
                    // %PACKAGE_VERSION% environment variable correctly.
                    Console.WriteLine($"Building MSI installer for Datadog Agent version {CiInfo.PackageVersion}");
                });
            }

            if (string.IsNullOrEmpty(omnibusTarget) || omnibusTarget == "installer")
            {
                BuildMsi<DatadogInstaller>(() =>
                {
                    Console.WriteLine("Building MSI installer for the Datadog Installer");
                });
            }
        }
    }

}
