using NineDigit.WixSharpExtensions;
using System;
using System.Drawing;
using Datadog.CustomActions;
using WixSharp;
using WixSharp.CommonTasks;
using WixSharp.Controls;
using Condition = WixSharp.Condition;
using FontStyle = System.Drawing.FontStyle;

namespace WixSetup
{
    internal class Program
    {
        // Company
        private const string CompanyFullName = "Datadog, inc.";

        // Product
        private const string ProductFullName = "Datadog Agent";
        private const string ProductDescription = "Datadog helps you monitor your infrastructure and application";
        private const string ProductComments = "My application comment";
        private const string ProductHelpUrl = @"https://help.datadoghq.com/hc/en-us";
        private const string ProductAboutUrl = @"https://www.datadoghq.com/about/";
        private const string ProductContact = @"https://www.datadoghq.com/about/contact/";
        private static readonly Guid ProductUpgradeCode = new Guid("0c50421b-aefb-4f15-a809-7af256d608a5"); // same value for all versions; must not be changed
        private static readonly string ProductLicenceRtfFilePath = System.IO.Path.Combine("assets", "LICENSE.rtf");
        private static readonly string ProductIconFilePath = System.IO.Path.Combine("assets", "project.ico");
        private static readonly string InstallerBackgroundImagePath = System.IO.Path.Combine("assets", "dialog_background.bmp");
        private static readonly string InstallerBannerImagePath = System.IO.Path.Combine("assets", "banner_background.bmp");

        // Source directories
        private const string InstallerSource = @"C:\opt\datadog-agent";
        private const string BinSource = @"C:\omnibus-ruby\src\datadog-agent\src\github.com\DataDog\datadog-agent\bin";
        private const string EtcSource = @"C:\omnibus-ruby\src\etc\datadog-agent";

        private static PermissionEx DefaultPermissions()
        {
            return new PermissionEx
            {
                User = "Everyone",
                ServicePauseContinue = true,
                ServiceQueryStatus = true,
                ServiceStart = true,
                ServiceStop = true,
                ServiceUserDefinedControl = true
            };
        }

        private static ServiceInstaller GenerateServiceInstaller(string name, string displayName, string description)
        {
            return new ServiceInstaller
            {
                Id = new Id("ddagentservice"),
                Name = name,
                DisplayName = displayName,
                Description = description,
                StartOn = null,
                Start = SvcStartType.auto,
                DelayedAutoStart = false,
                RemoveOn = SvcEvent.Uninstall_Wait,
                ServiceSid = ServiceSid.none,
                FirstFailureActionType = FailureActionType.restart,
                SecondFailureActionType = FailureActionType.restart,
                ThirdFailureActionType = FailureActionType.restart,
                RestartServiceDelayInSeconds = 60,
                ResetPeriodInDays = 0,
                PreShutdownDelay = 1000 * 60 * 3,
                PermissionEx = DefaultPermissions(),
                Account = "[DDAGENTUSER_DOMAIN]\\[DDAGENTUSER_NAME]",
                Password = "[DDAGENTUSER_PASSWORD]"
            };
        }

        private static ServiceInstaller GenerateDependentServiceInstaller(Id id, string name, string displayName, string description, string account, string password = null)
        {
            return new ServiceInstaller
            {
                Id = id,
                Name = name,
                DisplayName = displayName,
                Description = description,
                StartOn = null,
                Start = SvcStartType.demand,
                RemoveOn = SvcEvent.Uninstall_Wait,
                ServiceSid = ServiceSid.none,
                FirstFailureActionType = FailureActionType.restart,
                SecondFailureActionType = FailureActionType.restart,
                ThirdFailureActionType = FailureActionType.restart,
                RestartServiceDelayInSeconds = 60,
                ResetPeriodInDays = 0,
                PreShutdownDelay = 1000 * 60 * 3,
                PermissionEx = DefaultPermissions(),
                Interactive = false,
                Type = SvcType.ownProcess,
                Account = account,
                Password = password,
                DependsOn = new[]
                {
                    new ServiceDependency("datadogagent")
                }
            };
        }

        private static void Main()
        {
            Compiler.LightOptions += "-sval ";
            Compiler.LightOptions += "-reusecab ";
            Compiler.LightOptions += "-cc \"cabcache\"";

            bool includePython2 = false;

            var npm = new Feature("NPM", description: "Network Performance Monitoring", isEnabled: false, allowChange: true, configurableDir: "APPLICATIONROOTDIRECTORY");
            var app = new Feature("MainApplication", description: "Datadog Agent", isEnabled: true, allowChange: false, configurableDir: "APPLICATIONROOTDIRECTORY");
            app.Children.Add(npm);

            var agentService = GenerateServiceInstaller("datadogagent", "Datadog Agent", "Send metrics to Datadog");
            var processAgentService = GenerateDependentServiceInstaller(new Id("ddagentprocessservice"), "datadog-process-agent", "Datadog Process Agent", "Send process metrics to Datadog", "LocalSystem");
            var traceAgentService = GenerateDependentServiceInstaller(new Id("ddagenttraceservice"), "datadog-trace-agent", "Datadog Trace Agent", "Send tracing metrics to Datadog", "[DDAGENTUSER_DOMAIN]\\[DDAGENTUSER_NAME]", "[DDAGENTUSER_PASSWORD]");
            var systemProbeService = GenerateDependentServiceInstaller(new Id("ddagentsysprobeservice"), "datadog-system-probe", "Datadog System Probe", "Send network metrics to Datadog", "LocalSystem");

            var targetBinFolder = new Dir("bin",
                                    new File($@"{BinSource}\agent\agent.exe", agentService),
                                    new File($@"{BinSource}\agent\libdatadog-agent-three.dll"),
                                    new Dir("agent",
                                        new Dir("dist",
                                            new Files($@"{InstallerSource}\bin\agent\dist\*")
                                        ),
                                        new Merge(npm, $@"{BinSource}\agent\DDNPM.msm")
                                        {
                                            Feature = npm
                                        },
                                        new File($@"{BinSource}\agent\ddtray.exe"),
                                        new File($@"{BinSource}\agent\process-agent.exe", processAgentService),
                                        new File($@"{BinSource}\agent\security-agent.exe"),
                                        new File($@"{BinSource}\agent\system-probe.exe", systemProbeService),
                                        new File($@"{BinSource}\agent\trace-agent.exe", traceAgentService)
                                        )
                                    );

            if (includePython2)
            {
                targetBinFolder.AddFile(new File($@"{BinSource}\agent\libdatadog-agent-two.dll"));
            }

            //var showApiKeyDialog =
            //    new ShowClrDialogAction(nameof(ApiKeyDialog.Factory), typeof(UserCustomActions).Assembly.Location);

            var findApiKey = new CustomAction<ConfigUserActions>(
                    ConfigUserActions.FindAPIKey
                )
                .SetProperties("APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY]");

            var project = new Project("Datadog Agent",
                new User(new Id("ddagentuser"), "[DDAGENTUSER_NAME]")
                {
                    Domain = "[DDAGENTUSER_DOMAIN]",
                    Password = "[DDAGENTUSER_PASSWORD]",
                    PasswordNeverExpires = true,
                    RemoveOnUninstall = true,
                    //ComponentCondition = Condition.NOT("DDAGENTUSER_FOUND=\"true\")")
                },
                findApiKey,

                new CustomAction<UserCustomActions>(
                    UserCustomActions.ProcessDdAgentUserCredentials,
                    Return.check,
                    When.Before,
                    Step.InstallInitialize,
                    Condition.NOT_Installed & Condition.NOT_BeingRemoved
                )
                .SetProperties("DDAGENTUSER_NAME=[DDAGENTUSER_NAME], DDAGENTUSER_PASSWORD=[DDAGENTUSER_PASSWORD]")
                .HideTarget(true),

                //showApiKeyDialog,
                new Property("APIKEY")
                {
                    AttributesDefinition = "Hidden=yes;Secure=yes"
                },
                new Property("DDAGENTUSER_NAME", "ddagentuser"),
                new Property("DDAGENTUSER_PASSWORD")
                {
                    AttributesDefinition = "Hidden=yes;Secure=yes"
                },
                new CloseApplication(new Id("CloseTrayApp"), "ddtray.exe", closeMessage: true, rebootPrompt: false)
                {
                    Timeout = 1
                }
            )
            .SetProjectInfo(
                // unique for this project; same value for all versions; must not be changed between versions.
                upgradeCode: ProductUpgradeCode,
                name: ProductFullName,
                description: ProductDescription,
                version: new Version(7, 99, 0, 0) // TODO: Grab this from environment/command line
            )
            .SetControlPanelInfo(
                name: ProductFullName,
                manufacturer: CompanyFullName,
                readme: ProductHelpUrl,
                comment: ProductComments,
                contact: ProductContact,
                helpUrl: new Uri(ProductHelpUrl),
                aboutUrl: new Uri(ProductAboutUrl),
                productIconFilePath: new System.IO.FileInfo(ProductIconFilePath)
            )
            .DisableDowngradeToPreviousVersion()
            .SetMinimalUI(
                backgroundImage: new System.IO.FileInfo(InstallerBackgroundImagePath),
                bannerImage: new System.IO.FileInfo(InstallerBannerImagePath),
                // $@"{installerSource}\LICENSE" is not RTF and Compiler.AllowNonRtfLicense = true doesn't help.
                licenceRtfFile: new System.IO.FileInfo(ProductLicenceRtfFilePath)
            )
            .AddDirectories(
                new Dir(new Id("APPLICATIONROOTDIRECTORY"), @"%ProgramFiles%\Datadog",
                    new Dir(new Id("PROJECTLOCATION"), "Datadog Agent", targetBinFolder),
                    new Dir("LICENSES",
                        new Files($@"{InstallerSource}\LICENSES\*")
                    ),
                    new DirFiles($@"{InstallerSource}\*.json"),
                    new DirFiles($@"{InstallerSource}\*.txt")
                ),
                new Dir(new Id("APPLICATIONDATADIRECTORY"), @"%CommonAppData%\Datadog",
                    new DirPermission("[WIX_ACCOUNT_ADMINISTRATORS]", GenericPermission.All),
                    new DirPermission("[WIX_ACCOUNT_LOCALSYSTEM]", GenericPermission.All),
                    new DirPermission("[WIX_ACCOUNT_USERS]", GenericPermission.All),
                    new DirFiles($@"{EtcSource}\*.yaml.example"),
                    new Dir("checks.d"),
                    new Dir(new Id("EXAMPLECONFSLOCATION"), "conf.d",
                        new Files($@"{EtcSource}\extra_package_files\EXAMPLECONFSLOCATION\*")
                    )
                ),
                new Dir(@"%ProgramMenu%\Datadog",
                    new ExeFileShortcut
                    {
                        Name = "Datadog Agent Manager",
                        Target = "[AGENT]ddtray.exe",
                        Arguments = "&quot;-launch-gui&quot;",
                        WorkingDirectory = "AGENT",
                    }
                ),
                new Dir("logs")
            )
            // enable the ability to repair the installation even when the original MSI is no longer available.
            //.EnableResilientPackage() // Resilient package requires a .Net version newer than what is installed on 2008 R2
            ;

            project.Platform = Platform.x64;
            // MSI 4.0+ required
            project.InstallerVersion = 400;
            project.DefaultFeature = app;
            project.Codepage = "1252";
            project.InstallPrivileges = InstallPrivileges.elevated;
            project.LocalizationFile = "localization-en-us.wxl";
            project.OutFileName = "datadog-agent-7.40.0-1-x86_64";

            // clear default media as we will add it via MediaTemplate
            project.Media.Clear();
            project.WixSourceGenerated += document =>
            {
                document.Select("Wix/Product")
                        .AddElement("MediaTemplate", "CabinetTemplate=cab{0}.cab; CompressionLevel=mszip; EmbedCab=yes; MaximumUncompressedMediaSize=2");

                var ui = document.Root.Select("Product/UI");
                // Need to customize here since color is not supported with standard methods
                ui.AddTextStyle("WixUI_Font_Normal_White", new Font("Tahoma", 8), Color.White);
                ui.AddTextStyle("WixUI_Font_Bigger_White", new Font("Tahoma", 12), Color.White);
                ui.AddTextStyle("WixUI_Font_Title_White", new Font("Tahoma", 9, FontStyle.Bold), Color.White);
            };

            project.UI = WUI.WixUI_Common;
            project.CustomUI = new CustomUI();

            project.CustomUI.On(NativeDialogs.WelcomeDlg, Buttons.Next, new ShowDialog(NativeDialogs.LicenseAgreementDlg, Condition.NOT_Installed));
            project.CustomUI.On(NativeDialogs.WelcomeDlg, Buttons.Next, new ShowDialog(NativeDialogs.VerifyReadyDlg, Conditions.Installed_AND_PATCH));

            project.CustomUI.On(NativeDialogs.LicenseAgreementDlg, Buttons.Back, new ShowDialog(NativeDialogs.WelcomeDlg));
            project.CustomUI.On(NativeDialogs.LicenseAgreementDlg, Buttons.Next, new ShowDialog(NativeDialogs.CustomizeDlg, Conditions.LicenseAccepted));

            project.CustomUI.On(NativeDialogs.CustomizeDlg, Buttons.Back, new ShowDialog(NativeDialogs.MaintenanceTypeDlg, Condition.Installed) { Order = 1 });
            project.CustomUI.On(NativeDialogs.CustomizeDlg, Buttons.Back, new ShowDialog(NativeDialogs.LicenseAgreementDlg, Condition.NOT_Installed) { Order = 2 });
            project.CustomUI.On(NativeDialogs.CustomizeDlg, Buttons.Next, new ExecuteCustomAction(findApiKey.Id) { Order = 1 });
            project.CustomUI.On(NativeDialogs.CustomizeDlg, Buttons.Next, new ShowDialog(Dialogs.ApiKeyDialog) { Order = 2 });

            project.CustomUI.On(Dialogs.ApiKeyDialog, Buttons.Next, new ShowDialog(Dialogs.SiteSelectionDialog));
            project.CustomUI.On(Dialogs.ApiKeyDialog, Buttons.Back, new ShowDialog(NativeDialogs.CustomizeDlg, Condition.NOT_Installed));
            project.CustomUI.On(Dialogs.ApiKeyDialog, Buttons.Back, new ShowDialog(NativeDialogs.MaintenanceTypeDlg, Conditions.Installed_AND_NOT_PATCH));

            project.CustomUI.On(Dialogs.SiteSelectionDialog, Buttons.Next, new ShowDialog(NativeDialogs.VerifyReadyDlg));
            project.CustomUI.On(Dialogs.SiteSelectionDialog, Buttons.Back, new ShowDialog(Dialogs.ApiKeyDialog));

            project.CustomUI.On(NativeDialogs.VerifyReadyDlg, Buttons.Back, new ShowDialog(NativeDialogs.CustomizeDlg, Condition.NOT_Installed | Condition.Create("WixUI_InstallMode = \"Change\"")) { Order = 1 });
            project.CustomUI.On(NativeDialogs.VerifyReadyDlg, Buttons.Back, new ShowDialog(NativeDialogs.MaintenanceTypeDlg, Conditions.Installed_AND_NOT_PATCH) { Order = 2 });
            project.CustomUI.On(NativeDialogs.VerifyReadyDlg, Buttons.Next, new ShowDialog(NativeDialogs.WelcomeDlg, Conditions.Installed_AND_PATCH) { Order = 3 });

            project.CustomUI.On(NativeDialogs.MaintenanceWelcomeDlg, Buttons.Next, new ShowDialog(NativeDialogs.MaintenanceTypeDlg));

            project.CustomUI.On(NativeDialogs.MaintenanceTypeDlg, "ChangeButton", new ShowDialog(NativeDialogs.CustomizeDlg));
            project.CustomUI.On(NativeDialogs.MaintenanceTypeDlg, Buttons.Repair, new ShowDialog(NativeDialogs.VerifyReadyDlg));
            project.CustomUI.On(NativeDialogs.MaintenanceTypeDlg, Buttons.Remove, new ShowDialog(NativeDialogs.VerifyReadyDlg));
            project.CustomUI.On(NativeDialogs.MaintenanceTypeDlg, Buttons.Back, new ShowDialog(NativeDialogs.MaintenanceWelcomeDlg));

            project.AddXmlInclude("dialogs/apikeydlg.wxi")
                   .AddXmlInclude("dialogs/sitedlg.wxi");

            project.BuildMsi();
        }
    }

}
