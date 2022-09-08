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
        const string CompanyFullName = "Datadog, inc.";

        // Product
        const string ProductFullName = "Datadog Agent";
        const string ProductDescription = "Datadog helps you monitor your infrastructure and application";
        const string ProductComments = "My application comment";
        const string ProductHelpUrl = @"https://help.datadoghq.com/hc/en-us";
        const string ProductAboutUrl = @"https://www.datadoghq.com/about/";
        const string ProductContact = @"https://www.datadoghq.com/about/contact/";
        readonly static Guid ProductUpgradeCode = new Guid("0c50421b-aefb-4f15-a809-7af256d608a5"); // same value for all versions; must not be changed
        readonly static string ProductLicenceRTFFilePath = System.IO.Path.Combine("assets", "LICENSE.rtf");
        readonly static string ProductIconFilePath = System.IO.Path.Combine("assets", "project.ico");
        readonly static string InstallerBackgroundImagePath = System.IO.Path.Combine("assets", "dialog_background.bmp");
        readonly static string InstallerBannerImagePath = System.IO.Path.Combine("assets", "banner_background.bmp");

        const string installerSource = @"C:\opt\datadog-agent";
        const string binSource = @"C:\omnibus-ruby\src\datadog-agent\src\github.com\DataDog\datadog-agent\bin";
        const string etcSource = @"C:\omnibus-ruby\src\datadog-agent\etc\datadog-agent";
        /*
        static PermissionEx DefaultPermissions()
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

        static ServiceInstaller GenerateServiceInstaller(string name, string displayName, string description)
        {
            return new ServiceInstaller
            {
                Name = name,
                DisplayName = displayName,
                Description = description,
                StartOn = SvcEvent.Install,
                Start = SvcStartType.auto,
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
                Account = @"[DDAGENTUSER_DOMAIN]\[DDAGENTUSER_NAME]",
                Password = "[DDAGENTUSER_PASSWORD]"
            };
        }

        static ServiceInstaller GenerateDependentServiceInstaller(string name, string displayName, string description, string account)
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
                //Account = account,
                DependsOn = new[]
                {
                    new ServiceDependency("datadogagent")
                }
            };
        }
        */


        static void ConfigureProject(Project project, bool includePython2)
        {
            project.AddAction(new ElevatedManagedAction(
                CustomActions.CreateUser,
                typeof(CustomActions).Assembly.Location,
                Return.check,
                When.After,
                Step.InstallInitialize,
                WixSharp.Condition.NOT_Installed
            )
            {
                RefAssemblies = typeof(CustomActions).GetReferencesAssembliesPaths().ToArray(),
                UsesProperties = "DDAGENTUSER_NAME=[DDAGENTUSER_NAME], DDAGENTUSER_PASSWORD=[DDAGENTUSER_PASSWORD]"
            });
            project.AddProperties(
                new Property("DDAGENTUSER_NAME", "ddagentuser"),
                new Property("DDAGENTUSER_PASSWORD")
                {
                    AttributesDefinition = "Hidden=yes;Secure=yes"
                }
            );
            //project.Add(new User("[DDAGENTUSER_NAME]")
            //{
            //    Domain = "[DDAGENTUSER_DOMAIN]",
            //    Password = "[DDAGENTUSER_PASSWORD]",
            //    PasswordNeverExpires = true,
            //    CreateUser = true,
            //    LogonAsService = true,
            //    ComponentCondition = new Condition(@"!DDAGENTUSER_DOMAIN = .\")
            //});
            //project.Add(new User("[DDAGENTUSER_NAME]")
            //{
            //    Password = "[DDAGENTUSER_PASSWORD]",
            //    PasswordNeverExpires = true,
            //    CreateUser = true,
            //    LogonAsService = true,
            //    UpdateIfExists = true
            //});

            var npm = new Feature("NPM", description: "Network Performance Monitoring", isEnabled: false, allowChange: true, configurableDir: "APPLICATIONROOTDIRECTORY");
            var app = new Feature("MainApplication", description: "Datadog Agent", isEnabled: true, allowChange: false, configurableDir: "APPLICATIONROOTDIRECTORY");
            app.Children.Add(npm);

            //var agentService        = GenerateServiceInstaller("datadogagent", "Datadog Agent", "Send metrics to Datadog");
            //var processAgentService = GenerateDependentServiceInstaller("datadog-process-agent", "Datadog Process Agent", "Send process metrics to Datadog", @".\LOCAL_SYSTEM");
            //var traceAgentService   = GenerateDependentServiceInstaller("datadog-trace-agent", "Datadog Trace Agent", "Send tracing metrics to Datadog", "@\"[DDAGENTUSER_DOMAIN]\\[DDAGENTUSER_NAME]\"");
            //var systemProbeService  = GenerateDependentServiceInstaller("datadog-system-probe", "Datadog System Probe", "Send network metrics to Datadog", @".\LOCAL_SYSTEM");

            var targetBinFolder = new Dir("bin",
                                    new File($@"{binSource}\agent\agent.exe"),
                                    new File($@"{binSource}\agent\libdatadog-agent-three.dll"),
                                    new Dir("agent",
                                        new Dir("dist",
                                            new Files($@"{installerSource}\bin\agent\dist\*")
                                        ),
                                        new Merge(npm, $@"{binSource}\agent\DDNPM.msm")
                                        {
                                            Feature = npm
                                        },
                                        new File($@"{binSource}\agent\ddtray.exe"),
                                        new File($@"{binSource}\agent\process-agent.exe"),
                                        new File($@"{binSource}\agent\security-agent.exe"),
                                        new File($@"{binSource}\agent\system-probe.exe"),
                                        new File($@"{binSource}\agent\trace-agent.exe")
                                        )
                                    );

            if (includePython2)
            {
                targetBinFolder.AddFile(new File($@"{binSource}\agent\libdatadog-agent-two.dll"));
            }

            project.AddDirs(
                new Dir(new Id("APPLICATIONROOTDIRECTORY"), @"%ProgramFiles%\Datadog",
                    new Dir(new Id("PROJECTLOCATION"), "Datadog Agent", targetBinFolder),
                    new Dir("LICENSES",
                        new Files($@"{installerSource}\LICENSES\*")
                    ),
                    new DirFiles($@"{installerSource}\*.json"),
                    new DirFiles($@"{installerSource}\*.txt")
                ),
                new Dir(new Id("APPLICATIONDATADIRECTORY"), @"%CommonAppData%\Datadog",
                    new DirPermission("[WIX_ACCOUNT_ADMINISTRATORS]", GenericPermission.All),
                    new DirPermission("[WIX_ACCOUNT_LOCALSYSTEM]", GenericPermission.All),
                    new DirPermission("[WIX_ACCOUNT_USERS]", GenericPermission.All),
                    new DirFiles($@"{etcSource}\*.yaml.example"),
                    new Dir("checks.d"),
                    new Dir(new Id("EXAMPLECONFSLOCATION"), "conf.d",
                        new Files($@"{etcSource}\extra_package_files\EXAMPLECONFSLOCATION\*")
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

            project.Media.Clear(); // clear default media as we will add it via MediaTemplate
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
                    upgradeCode: ProductUpgradeCode, // unique for this project; same value for all versions; must not be changed between versions.
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
                    licenceRtfFile: new System.IO.FileInfo(ProductLicenceRTFFilePath) // $@"{installerSource}\LICENSE" is not RTF and Compiler.AllowNonRtfLicense = true doesn't help.
                )
                .EnableResilientPackage()   // enable the ability to repair the installation even when the original MSI is no longer available.
                ;
        }

        static void Main()
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
