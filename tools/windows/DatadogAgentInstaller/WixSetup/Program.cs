using Microsoft.Deployment.WindowsInstaller;
using NineDigit.WixSharpExtensions;
using NineDigit.WixSharpExtensions.Resources;
using System;
using System.Linq;
using System.Windows;
using WixSharp;
using WixSharp.CommonTasks;
using WixSharp.Controls;

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

        private const string InstallerSource = @"C:\opt\datadog-agent";
        private const string BinSource = @"C:\omnibus-ruby\src\datadog-agent\src\github.com\DataDog\datadog-agent\bin";
        private const string EtcSource = @"C:\omnibus-ruby\src\datadog-agent\etc\datadog-agent";

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
                Name = name,
                DisplayName = displayName,
                Description = description,
                StartOn = SvcEvent.Install,
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

        private static ServiceInstaller GenerateDependentServiceInstaller(string name, string displayName, string description, string account, string password = null)
        {
            return new ServiceInstaller
            {
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

        private static void ConfigureProject(Project project, bool includePython2)
        {
            // Create user before starting services
            project.AddAction(new ManagedAction(
                CustomActions.UserCustomActions.ProcessDdAgentUserCredentials,
                typeof(CustomActions.UserCustomActions).Assembly.Location,
                Return.check,
                When.Before,
                Step.InstallInitialize,
                WixSharp.Condition.NOT_Installed & WixSharp.Condition.NOT_BeingRemoved
            )
            {
                RefAssemblies = typeof(CustomActions.UserCustomActions).GetReferencesAssembliesPaths().ToArray(),
                UsesProperties = "DDAGENTUSER_NAME=[DDAGENTUSER_NAME], DDAGENTUSER_PASSWORD=[DDAGENTUSER_PASSWORD]"
            });

#if false
            project.AddAction(new ElevatedManagedAction(
                CustomActions.ServicesCustomActions.CreateServices,
                typeof(CustomActions.UserCustomActions).Assembly.Location,
                Return.check,
                When.Before,
                Step.StartServices,
                WixSharp.Condition.NOT_Installed & WixSharp.Condition.NOT_BeingRemoved
            )
            {
                RefAssemblies = typeof(CustomActions.UserCustomActions).GetReferencesAssembliesPaths().ToArray(),
                UsesProperties = "DDAGENTUSER_NAME=[DDAGENTUSER_NAME], DDAGENTUSER_PASSWORD=[DDAGENTUSER_PASSWORD]"
            });
#endif
            project.AddProperties(
                new Property("DDAGENTUSER_NAME", "ddagentuser"),
                new Property("DDAGENTUSER_PASSWORD")
                {
                    AttributesDefinition = "Hidden=yes;Secure=yes"
                }
            );
            project.Add(new User("[DDAGENTUSER_NAME]")
            {
                Domain = "[DDAGENTUSER_DOMAIN]",
                Password = "[DDAGENTUSER_PASSWORD]",
                PasswordNeverExpires = true,
                LogonAsService = true,
                RemoveOnUninstall = true,
                ComponentCondition = new WixSharp.Condition(" (NOT (DDAGENTUSER_FOUND=\"true\")) ")
            });

            var npm = new Feature("NPM", description: "Network Performance Monitoring", isEnabled: false, allowChange: true, configurableDir: "APPLICATIONROOTDIRECTORY");
            var app = new Feature("MainApplication", description: "Datadog Agent", isEnabled: true, allowChange: false, configurableDir: "APPLICATIONROOTDIRECTORY");
            app.Children.Add(npm);

            var agentService        = GenerateServiceInstaller("datadogagent", "Datadog Agent", "Send metrics to Datadog");
            var processAgentService = GenerateDependentServiceInstaller("datadog-process-agent", "Datadog Process Agent", "Send process metrics to Datadog", "LocalSystem");
            var traceAgentService   = GenerateDependentServiceInstaller("datadog-trace-agent", "Datadog Trace Agent", "Send tracing metrics to Datadog", "[DDAGENTUSER_DOMAIN]\\[DDAGENTUSER_NAME]", "[DDAGENTUSER_PASSWORD]");
            var systemProbeService  = GenerateDependentServiceInstaller("datadog-system-probe", "Datadog System Probe", "Send network metrics to Datadog", "LocalSystem");

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

            project.AddDirs(
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
            );

            project.Add(
                new CloseApplication(new Id("CloseTrayApp"), "ddtray.exe", closeMessage: true, rebootPrompt: false)
                {
                    Timeout = 1
                }
            );

            // clear default media as we will add it via MediaTemplate
            project.Media.Clear();
            project.WixSourceGenerated += document =>
            {
                document.Select("Wix/Product")
                        .AddElement("MediaTemplate", "CabinetTemplate=cab{0}.cab; CompressionLevel=mszip; EmbedCab=yes; MaximumUncompressedMediaSize=2");
            };

            project.Platform = Platform.x64;
            // MSI 4.0+ required
            project.InstallerVersion = 400;
            project.DefaultFeature = app;
            project.Codepage = "1252";
            project.InstallPrivileges = InstallPrivileges.elevated;
            project.SetProjectInfo(
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
                // enable the ability to repair the installation even when the original MSI is no longer available.
                //.EnableResilientPackage() // Resilient package requires a .Net version newer than what is installed on 2008R2
                ;
        }

        private static void Main()
        {
            Compiler.LightOptions += "-sval ";
            Compiler.LightOptions += "-reusecab ";
            Compiler.LightOptions += "-cc \"cabcache\"";

            var project = new Project("Datadog Agent");
            ConfigureProject(project, false);
            project.OutFileName = "datadog-agent-7.40.0-1-x86_64";
            project.UI = WUI.WixUI_FeatureTree;

            project.BuildMsi();
        }
    }
}
