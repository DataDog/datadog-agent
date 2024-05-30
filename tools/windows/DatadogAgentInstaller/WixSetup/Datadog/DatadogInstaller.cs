using System;
using System.IO;
using System.Security.Principal;
using NineDigit.WixSharpExtensions;
using WixSharp;

namespace WixSetup.Datadog
{
    public class DatadogInstaller : IMsiInstallerProject
    {
        // Company
        private const string CompanyFullName = "Datadog, Inc.";

        // Product
        private const string ProductFullName = "Datadog Installer";
        private const string ProductDescription = "Datadog Installer manages the installation of Datadog product";
        private const string ProductHelpUrl = @"https://help.datadoghq.com/hc/en-us";
        private const string ProductAboutUrl = @"https://www.datadoghq.com/about/";
        private const string ProductComment = @"Copyright 2015 - Present Datadog";
        private const string ProductContact = @"https://www.datadoghq.com/about/contact/";

        // same value for all versions; must not be changed
        private static readonly Guid ProductUpgradeCode = new("1639DAD6-C892-405D-9CDB-38A1359C9C0F");
        private static readonly string ProductIconFilePath = Path.Combine("assets", "project.ico");
        private static readonly string InstallerBackgroundImagePath = Path.Combine("assets", "dialog_background.bmp");
        private static readonly string InstallerBannerImagePath = Path.Combine("assets", "banner_background.bmp");

        public Project Configure()
        {
            var project = new Project("Datadog Installer",
                new Property("MsiLogging", "iwearucmop!"),
                new Dir(@"%ProgramFiles%\Datadog\Datadog Installer",
                    new WixSharp.File(@"C:\opt\datadog-installer\datadog-installer.exe",
                        new ServiceInstaller
                        {
                            Name = "Datadog Installer",
                            StartOn = SvcEvent.Install,
                            StopOn = SvcEvent.InstallUninstall_Wait,
                            RemoveOn = SvcEvent.Uninstall_Wait,
                            Start = SvcStartType.auto,
                            DelayedAutoStart = true,
                            ServiceSid = ServiceSid.none,
                            FirstFailureActionType = FailureActionType.restart,
                            SecondFailureActionType = FailureActionType.restart,
                            ThirdFailureActionType = FailureActionType.restart,
                            Arguments = "run",
                            RestartServiceDelayInSeconds = 30,
                            ResetPeriodInDays = 1,
                            PreShutdownDelay = 1000 * 60 * 3,
                            Account = "LocalSystem",
                            Vital = true
                        })
                ));

            // Always generate a new GUID otherwise WixSharp will generate one based on
            // the version
            project.ProductId = Guid.NewGuid();
            project
                .SetProjectInfo(
                    upgradeCode: ProductUpgradeCode,
                    name: ProductFullName,
                    description: ProductDescription,
                    // The installer is not versioned.
                    version: new Version(1, 0, 0, 0)
                )
                .SetControlPanelInfo(
                    name: ProductFullName,
                    manufacturer: CompanyFullName,
                    readme: ProductHelpUrl,
                    comment: ProductComment,
                    contact: ProductContact,
                    helpUrl: new Uri(ProductHelpUrl),
                    aboutUrl: new Uri(ProductAboutUrl),
                    productIconFilePath: new FileInfo(ProductIconFilePath)
                )
                .SetMinimalUI(
                    backgroundImage: new FileInfo(InstallerBackgroundImagePath),
                    bannerImage: new FileInfo(InstallerBannerImagePath)
                );
            project.MajorUpgrade = new MajorUpgrade
            {
                // Let it downgrade to a lower version, we always ask the bootstrapper to update to the latest version anyway.
                AllowDowngrades = true
            };
            project.Platform = Platform.x64;
            project.InstallerVersion = 500;
            project.Codepage = "1252";
            project.InstallPrivileges = InstallPrivileges.elevated;
            if (!string.IsNullOrEmpty(Environment.GetEnvironmentVariable("AGENT_MSI_OUTDIR")))
            {
                // Set custom output directory (WixSharp defaults to current directory)
                project.OutDir = Environment.GetEnvironmentVariable("AGENT_MSI_OUTDIR");
            }
            project.OutFileName = $"datadog-installer-1-x86_64";
            project.Package.AttributesDefinition = $"Comments={ProductComment}";
            //project.UI = WUI.WixUI_Common;

            return project;
        }
    }
}
