using NineDigit.WixSharpExtensions;
using System;
using System.Collections.Generic;
using System.Drawing;
using System.Linq;
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

        public static readonly string Agent = $@"{BinSource}\agent\agent.exe";
        public static readonly string Tray = $@"{BinSource}\agent\ddtray.exe";
        public static readonly string ProcessAgent = $@"{BinSource}\agent\process-agent.exe";
        public static readonly string SecurityAgent = $@"{BinSource}\agent\security-agent.exe";
        public static readonly string SystemProbe = $@"{BinSource}\agent\system-probe.exe";
        public static readonly string TraceAgent = $@"{BinSource}\agent\trace-agent.exe";
        public static readonly string LibDatadogAgentThree = $@"{BinSource}\agent\libdatadog-agent-three.dll";
        public static readonly string LibDatadogAgentTwo = $@"{BinSource}\agent\libdatadog-agent-two.dll";

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
            bool includePython2 = false;
            var pyRuntimes = new[] {"3"};
            var pyRuntimesEnv = Environment.GetEnvironmentVariable("PY_RUNTIMES");
            if (pyRuntimesEnv != null)
            {
                pyRuntimes = pyRuntimesEnv.Split();
                if (pyRuntimes.Any(runtime => runtime == "2"))
                {
                    includePython2 = true;
                }
            }

            var version = new Version(7, 99, 0, 2);
            var envVersion = Environment.GetEnvironmentVariable("PACKAGE_VERSION");
            if (envVersion != null)
            {
                version = Version.Parse(envVersion);
            }

            var pfxFilePath = Environment.GetEnvironmentVariable("SIGN_PFX");
            var pfxFilePassword = Environment.GetEnvironmentVariable("SIGN_PFX_PW");

            DigitalSignature digitalSignature = null;

            if (pfxFilePath != null && pfxFilePassword != null)
            {
                digitalSignature = new DigitalSignature
                {
                    PfxFilePath = pfxFilePath,
                    Password = pfxFilePassword,
                    HashAlgorithm = HashAlgorithmType.sha256,
                    // Only use timestamp servers from Microsoft-approved authenticode providers
                    // See https://docs.microsoft.com/en-us/windows/win32/seccrypto/time-stamping-authenticode-signatures
                    TimeUrls = new List<Uri>
                    {
                        new Uri("http://timestamp.digicert.com"),
                        new Uri("http://timestamp.globalsign.com/scripts/timstamp.dll"),
                        new Uri("http://timestamp.comodoca.com/authenticode"),
                        new Uri("http://www.startssl.com/timestamp"),
                        new Uri("http://timestamp.sectigo.com"),
                    }
                };
            }

            Compiler.LightOptions += "-sval ";
            Compiler.LightOptions += "-reusecab ";
            Compiler.LightOptions += "-cc \"cabcache\"";

            var npm = new Feature("NPM", description: "Network Performance Monitoring", isEnabled: false, allowChange: true, configurableDir: "APPLICATIONROOTDIRECTORY");
            var app = new Feature("MainApplication", description: "Datadog Agent", isEnabled: true, allowChange: false, configurableDir: "APPLICATIONROOTDIRECTORY");
            app.Children.Add(npm);

            var agentService = GenerateServiceInstaller("datadogagent", "Datadog Agent", "Send metrics to Datadog");
            var processAgentService = GenerateDependentServiceInstaller(new Id("ddagentprocessservice"), "datadog-process-agent", "Datadog Process Agent", "Send process metrics to Datadog", "LocalSystem");
            var traceAgentService = GenerateDependentServiceInstaller(new Id("ddagenttraceservice"), "datadog-trace-agent", "Datadog Trace Agent", "Send tracing metrics to Datadog", "[DDAGENTUSER_DOMAIN]\\[DDAGENTUSER_NAME]", "[DDAGENTUSER_PASSWORD]");
            var systemProbeService = GenerateDependentServiceInstaller(new Id("ddagentsysprobeservice"), "datadog-system-probe", "Datadog System Probe", "Send network metrics to Datadog", "LocalSystem");

            var filesToSign = new List<string>
            {
                Agent,
                Tray,
                ProcessAgent,
                SecurityAgent,
                SystemProbe,
                TraceAgent,
                LibDatadogAgentThree
            };

            if (includePython2)
            {
                filesToSign.Add(LibDatadogAgentTwo);
            }

            var targetBinFolder = new Dir("bin",
                                        new File(Agent, agentService),
                                        new File(LibDatadogAgentThree),
                                        new Dir("agent",
                                            new Dir("dist",
                                                new Files($@"{InstallerSource}\bin\agent\dist\*")
                                            ),
                                            new Merge(npm, $@"{BinSource}\agent\DDNPM.msm")
                                            {
                                                Feature = npm
                                            },
                                            new File(Tray),
                                            new File(ProcessAgent, processAgentService),
                                            new File(SecurityAgent),
                                            new File(SystemProbe, systemProbeService),
                                            new File(TraceAgent, traceAgentService)
                                            )
                                        );

            if (includePython2)
            {
                targetBinFolder.AddFile(new File(LibDatadogAgentTwo));
            }

            var readConfig = new CustomAction<ConfigUserActions>(
                    ConfigUserActions.ReadConfig
                )
                .SetProperties("APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY]");
            var writeConfig = new CustomAction<ConfigUserActions>(
                    ConfigUserActions.WriteConfig
                )
                .SetProperties("APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY], " +
                               "SYSPROBE_PRESENT=[SYSPROBE_PRESENT], " +
                               "NPMFEATURE=[NPMFEATURESTATE], " +
                               "ADDLOCAL=[ADDLOCAL], " +
                               "APIKEY=[APIKEY], " +
                               "TAGS=[TAGS], " +
                               "HOSTNAME=[HOSTNAME], " +
                               "PROXY_HOST=[PROXY_HOST], " +
                               "PROXY_PORT=[PROXY_PORT], " +
                               "PROXY_USER=[PROXY_USER], " +
                               "PROXY_PASSWORD=[PROXY_PASSWORD], " +
                               "LOGS_ENABLED=[LOGS_ENABLED], " +
                               "APM_ENABLED=[APM_ENABLED], " +
                               "PROCESS_ENABLED=[PROCESS_ENABLED], " +
                               "PROCESS_DISCOVERY_ENABLED=[PROCESS_DISCOVERY_ENABLED], " +
                               "CMD_PORT=[CMD_PORT], " +
                               "SITE=[SITE], " +
                               "DD_URL=[DD_URL], " +
                               "LOGS_DD_URL=[LOGS_DD_URL], " +
                               "PROCESS_DD_URL=[PROCESS_DD_URL], " +
                               "TRACE_DD_URL=[TRACE_DD_URL], " +
                               "PYVER=[PYVER], " +
                               "HOSTNAME_FQDN_ENABLED=[HOSTNAME_FQDN_ENABLED], " +
                               "NPM=[NPM], " +
                               "EC2_USE_WINDOWS_PREFIX_DETECTION=[EC2_USE_WINDOWS_PREFIX_DETECTION], " +
                               "OVERRIDE_INSTALLATION_METHOD=[OVERRIDE_INSTALLATION_METHOD]");

            var project = new Project("Datadog Agent",
                new User(new Id("ddagentuser"), "[DDAGENTUSER_NAME]")
                {
                    Domain = "[DDAGENTUSER_DOMAIN]",
                    Password = "[DDAGENTUSER_PASSWORD]",
                    PasswordNeverExpires = true,
                    RemoveOnUninstall = true,
                    //ComponentCondition = Condition.NOT("DDAGENTUSER_FOUND=\"true\")")
                },
                readConfig,
                writeConfig,
                new CustomAction<UserCustomActions>(
                    UserCustomActions.ProcessDdAgentUserCredentials,
                    Return.check,
                    When.Before,
                    Step.InstallInitialize,
                    Condition.NOT_Installed & Condition.NOT_BeingRemoved
                )
                .SetProperties("DDAGENTUSER_NAME=[DDAGENTUSER_NAME], DDAGENTUSER_PASSWORD=[DDAGENTUSER_PASSWORD]")
                .HideTarget(true),

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
                // SetProjectInfo throws an Exception is Revision is != 0
                // we use Revision = 2 for the next gen installer while it's still a prototype
                version: new Version(version.Major, version.Minor, version.Build, 0)
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
            project.OutFileName = $"datadog-agent-{version.Major}.{version.Minor}.{version.Build}-{version.Revision}-x86_64";
            project.DigitalSignature = digitalSignature;

            // clear default media as we will add it via MediaTemplate
            project.Media.Clear();
            project.WixSourceGenerated += document =>
            {
                if (digitalSignature != null)
                {
                    foreach (var file in filesToSign)
                    {
                        digitalSignature.Apply(file);
                    }
                }

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
            project.CustomUI.On(NativeDialogs.CustomizeDlg, Buttons.Next, new ExecuteCustomAction(readConfig.Id) { Order = 1 });
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
