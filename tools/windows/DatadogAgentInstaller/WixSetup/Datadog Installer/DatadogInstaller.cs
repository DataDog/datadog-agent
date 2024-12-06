using NineDigit.WixSharpExtensions;
using System;
using System.IO;
using WixSetup.Datadog_Agent;
using WixSharp;
using WixSharp.CommonTasks;
using Condition = WixSharp.Condition;

namespace WixSetup.Datadog_Installer
{
    public class DatadogInstaller : IMsiInstallerProject, IWixProjectEvents
    {
        // Company
        private const string CompanyFullName = "Datadog, Inc.";

        // Product
        private const string ProductFullName = "Datadog Installer";
        private const string ProductDescription = "Datadog Installer manages the installation of Datadog products";
        private const string ProductHelpUrl = @"https://help.datadoghq.com/hc/en-us";
        private const string ProductAboutUrl = @"https://www.datadoghq.com/about/";
        private const string ProductComment = @"Copyright 2015 - Present Datadog";
        private const string ProductContact = @"https://www.datadoghq.com/about/contact/";

        // same value for all versions; must not be changed
        private static readonly Guid ProductUpgradeCode = new("1639DAD6-C892-405D-9CDB-38A1359C9C0F");
        private static readonly string ProductIconFilePath = Path.Combine("assets", "project.ico");
        private static readonly string InstallerBackgroundImagePath = Path.Combine("assets", "dialog_background.bmp");
        private static readonly string InstallerBannerImagePath = Path.Combine("assets", "banner_background.bmp");

        private readonly DatadogInstallerCustomActions _installerCustomActions = new();
        private readonly AgentVersion _agentVersion = new();

        public Project Configure()
        {
            var project = new ManagedProject("Datadog Installer",
                // Use 2 LaunchConditions, one for server versions,
                // one for client versions.
                MinimumSupportedWindowsVersion.WindowsServer2016 |
                MinimumSupportedWindowsVersion.Windows10,
                new Property("MsiLogging", "iwearucmop!"),
                new Property("APIKEY")
                {
                    AttributesDefinition = "Hidden=yes;Secure=yes"
                },
                // Hides the "send flare" button in the fatal error dialog
                new Property("HIDE_FLARE", "1"),
                new Property("APPLICATIONDATADIRECTORY",
                    new RegistrySearch(RegistryHive.LocalMachine, @"SOFTWARE\Datadog\Datadog Agent", "ConfigRoot", RegistrySearchType.raw))
                {
                    // Default value if the registry key is not found
                    // Can't use [CommonAppDataFolder] because of CNDL1077 illegal reference to another property.
                    // Can't use %CommonAppDataFolder% because it's a Wix property.
                    Value = @"C:\ProgramData\Datadog",
                    AttributesDefinition = "Secure=yes",
                },
                // User provided password property
                new Property("DDAGENTUSER_PASSWORD")
                {
                    AttributesDefinition = "Hidden=yes"
                },
                // ProcessDDAgentUserCredentials CustomAction processed password property
                new Property("DDAGENTUSER_PROCESSED_PASSWORD")
                {
                    AttributesDefinition = "Hidden=yes"
                },
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
                )
            );

            // Always generate a new GUID otherwise WixSharp will generate one based on
            // the version
            project.ProductId = Guid.NewGuid();
            project
                .SetCustomActions(_installerCustomActions)
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

            project.SetNetFxPrerequisite(Condition.Net45_Installed,
                "This application requires the .Net Framework 4.5, or later to be installed.");

            project.MajorUpgrade = MajorUpgrade.Default;
            // Set to true otherwise RC versions can't upgrade each other.
            project.MajorUpgrade.AllowSameVersionUpgrades = true;
            project.MajorUpgrade.Schedule = UpgradeSchedule.afterInstallInitialize;
            project.MajorUpgrade.DowngradeErrorMessage =
                "Automatic downgrades are not supported.  Uninstall the current version, and then reinstall the desired version.";
            project.ReinstallMode = "amus";
            project.Platform = Platform.x64;
            project.InstallerVersion = 500;
            project.Codepage = "1252";
            project.InstallPrivileges = InstallPrivileges.elevated;
            project.LocalizationFile = "localization-en-us.wxl";
            if (!string.IsNullOrEmpty(Environment.GetEnvironmentVariable("AGENT_MSI_OUTDIR")))
            {
                // Set custom output directory (WixSharp defaults to current directory)
                project.OutDir = Environment.GetEnvironmentVariable("AGENT_MSI_OUTDIR");
            }
            project.OutFileName = $"datadog-installer-{_agentVersion.PackageVersion}-1-x86_64";
            project.Package.AttributesDefinition = $"Comments={ProductComment}";
            project.UI = WUI.WixUI_Common;
            project.CustomUI = new DatadogInstallerUI(this, _installerCustomActions);
            project.WixSourceGenerated += document =>
            {
                WixSourceGenerated?.Invoke(document);
                document
                    .Select("Wix/Product")
                    .AddElement("CustomActionRef", "Id=WixFailWhenDeferred");
            };
            project.WixSourceFormated += (ref string content) => WixSourceFormated?.Invoke(content);
            project.WixSourceSaved += name => WixSourceSaved?.Invoke(name);
            return project;
        }

        public event XDocumentGeneratedDlgt WixSourceGenerated;
        public event XDocumentSavedDlgt WixSourceSaved;
        public event XDocumentFormatedDlgt WixSourceFormated;
    }
}
