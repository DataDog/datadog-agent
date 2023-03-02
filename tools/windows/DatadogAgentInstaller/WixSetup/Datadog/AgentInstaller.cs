using System;
using System.IO;
using System.Linq;
using System.Xml.Linq;
using Datadog.CustomActions;
using Microsoft.Deployment.WindowsInstaller;
using NineDigit.WixSharpExtensions;
using WixSharp;
using WixSharp.CommonTasks;

namespace WixSetup.Datadog
{
    public class AgentInstaller : IWixProjectEvents
    {
        // Company
        private const string CompanyFullName = "Datadog, inc.";

        // Product
        private const string ProductFullName = "Datadog Agent";
        private const string ProductDescription = "Datadog helps you monitor your infrastructure and application";
        private const string ProductHelpUrl = @"https://help.datadoghq.com/hc/en-us";
        private const string ProductAboutUrl = @"https://www.datadoghq.com/about/";
        private const string ProductComment = @"Copyright 2015 - Present Datadog";
        private const string ProductContact = @"https://www.datadoghq.com/about/contact/";

        // same value for all versions; must not be changed
        private static readonly Guid ProductUpgradeCode = new("0c50421b-aefb-4f15-a809-7af256d608a5");
        private static readonly string ProductLicenceRtfFilePath = Path.Combine("assets", "LICENSE.rtf");
        private static readonly string ProductIconFilePath = Path.Combine("assets", "project.ico");
        private static readonly string InstallerBackgroundImagePath = Path.Combine("assets", "dialog_background.bmp");
        private static readonly string InstallerBannerImagePath = Path.Combine("assets", "banner_background.bmp");

        // Source directories
        private const string InstallerSource = @"C:\opt\datadog-agent";
        private const string BinSource = @"C:\omnibus-ruby\src\datadog-agent\src\github.com\DataDog\datadog-agent\bin";
        private const string EtcSource = @"C:\omnibus-ruby\src\etc\datadog-agent";

        private readonly AgentBinaries _agentBinaries;
        private readonly AgentFeatures _agentFeatures = new();
        private readonly AgentPython _agentPython = new();
        private readonly AgentVersion _agentVersion;
        private readonly AgentSignature _agentSignature;
        private readonly AgentCustomActions _agentCustomActions = new();
        private readonly AgentInstallerUI _agentInstallerUi;

        public AgentInstaller(string version = null)
        {
            if (version == null)
            {
                _agentVersion = new AgentVersion();
            }
            else
            {
                _agentVersion = new AgentVersion(version);
            }
            _agentBinaries = new AgentBinaries(BinSource, InstallerSource);
            _agentSignature = new AgentSignature(this, _agentPython, _agentBinaries);
            _agentInstallerUi = new AgentInstallerUI(this, _agentCustomActions);
        }

        public Project ConfigureProject()
        {
            var project = new ManagedProject("Datadog Agent",
                new User
                {
                    // CreateUser fails with ERROR_BAD_USERNAME if Name is a fully qualified user name
                    Name = "[DDAGENTUSER_PROCESSED_NAME]",
                    Domain = "[DDAGENTUSER_PROCESSED_DOMAIN]",
                    Password = "[DDAGENTUSER_PROCESSED_PASSWORD]",
                    PasswordNeverExpires = true,
                    RemoveOnUninstall = true,
                    FailIfExists = false,
                    UpdateIfExists = true,
                    CreateUser = true
                },
                new Property("MsiLogging", "iwearucmop!"),
                new Property("APIKEY")
                {
                    AttributesDefinition = "Hidden=yes;Secure=yes"
                },
                // User provided password property
                new Property("DDAGENTUSER_PASSWORD")
                {
                    AttributesDefinition = "Hidden=yes;Secure=yes"
                },
                // ProcessDDAgentUserCredentials CustomAction processed password property
                new Property("DDAGENTUSER_PROCESSED_PASSWORD")
                {
                    AttributesDefinition = "Hidden=yes;Secure=yes"
                },
                new Property("PROJECTLOCATION")
                {
                    AttributesDefinition = "Secure=yes",
                },
                new Property("APPLICATIONDATADIRECTORY")
                {
                    AttributesDefinition = "Secure=yes",
                },
                new CloseApplication(new Id("CloseTrayApp"),
                    Path.GetFileName(_agentBinaries.Tray),
                    closeMessage: true,
                    rebootPrompt: false
                )
                {
                    Timeout = 1
                },
                new RegKey(
                    _agentFeatures.MainApplication,
                    RegistryHive.LocalMachine, @"Software\Datadog\Datadog Agent",
                    // Store these properties in the registry for retrieval by future
                    // installer runs via the ReadRegistryProperties CA.
                    new RegValue("InstallPath", "[PROJECTLOCATION]") { Win64 = true },
                    new RegValue("ConfigRoot", "[APPLICATIONDATADIRECTORY]") { Win64 = true },
                    new RegValue("installedDomain", "[DDAGENTUSER_PROCESSED_DOMAIN]") { Win64 = true },
                    new RegValue("installedUser", "[DDAGENTUSER_PROCESSED_NAME]") { Win64 = true }
                )
                {
                    Win64 = true
                },
                new RemoveRegistryKey(_agentFeatures.MainApplication, @"Software\Datadog\Datadog Agent")
            );
            project
            .SetCustomActions(_agentCustomActions)
            .SetProjectInfo(
                upgradeCode: ProductUpgradeCode,
                name: ProductFullName,
                description: ProductDescription,
                // SetProjectInfo throws an Exception is Revision is != 0
                // we use Revision = 2 for the next gen installer while it's still a prototype
                version: new Version(_agentVersion.Version.Major, _agentVersion.Version.Minor,
                    _agentVersion.Version.Build, 0)
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
                bannerImage: new FileInfo(InstallerBannerImagePath),
                // $@"{installerSource}\LICENSE" is not RTF and Compiler.AllowNonRtfLicense = true doesn't help.
                licenceRtfFile: new FileInfo(ProductLicenceRtfFilePath)
            )
            .AddDirectories(
                CreateProgramFilesFolder(),
                CreateAppDataFolder(),
                new Dir(@"%ProgramMenu%\Datadog",
                    new ExeFileShortcut
                    {
                        Name = "Datadog Agent Manager",
                        Target = "[AGENT]ddtray.exe",
                        Arguments = "\"--launch-gui\"",
                        WorkingDirectory = "AGENT",
                    }
                ),
                new Dir("logs")
            );

            // enable the ability to repair the installation even when the original MSI is no longer available.
            project.EnableResilientPackage();

            project.MajorUpgrade = MajorUpgrade.Default;
            // When set to yes, WiX sets the msidbUpgradeAttributesVersionMaxInclusive attribute,
            // which tells MSI to treat a product with the same version as a major upgrade.
            // This is useful when two product versions differ only in the fourth version field.
            // MSI specifically ignores that field when comparing product versions,
            // so two products that differ only in the fourth version field are the same product and
            // need this attribute set to "true" to be detected.
            // Note that because MSI ignores the fourth product version field,
            // setting this attribute to yes also allows downgrades when the first three product version fields are identical.
            // For example, product version 1.0.0.1 will "upgrade" 1.0.0.2998 because they're seen as the same version (1.0.0).
            // That could reintroduce serious bugs so the safest choice is to change the first three version fields and
            // omit this attribute to get the default of no.
            project.MajorUpgrade.AllowSameVersionUpgrades = false;
            project.MajorUpgrade.Schedule = UpgradeSchedule.afterInstallInitialize;
            project.MajorUpgrade.DowngradeErrorMessage =
                "Automatic downgrades are not supported.  Uninstall the current version, and then reinstall the desired version.";
            project.ReinstallMode = "amus";
            
            project.Platform = Platform.x64;
            // MSI 5.0 was shipped in Windows Server 2012 R2.
            // https://learn.microsoft.com/en-us/windows/win32/msi/released-versions-of-windows-installer
            project.InstallerVersion = 500;
            project.DefaultFeature = _agentFeatures.MainApplication;
            project.Codepage = "1252";
            project.InstallPrivileges = InstallPrivileges.elevated;
            project.LocalizationFile = "localization-en-us.wxl";
            project.OutFileName = $"datadog-agent-ng-{_agentVersion.PackageVersion}-1-x86_64";
            project.DigitalSignature = _agentSignature.Signature;

            // clear default media as we will add it via MediaTemplate
            project.Media.Clear();
            project.WixSourceGenerated += document =>
            {
                WixSourceGenerated?.Invoke(document);
                document
                    .Select("Wix/Product")
                    .AddElement("MediaTemplate", "CabinetTemplate=cab{0}.cab; CompressionLevel=high; EmbedCab=yes; MaximumUncompressedMediaSize=2");
                document
                    .FindAll("RemoveFolder")
                    .Where(x => x.HasAttribute("Id",
                        value => value.StartsWith("APPLICATIONDATADIRECTORY") ||
                                 value.StartsWith("EXAMPLECONFSLOCATION")))
                    .Remove();
                document
                    .FindAll("Component")
                    .Where(x => x.Parent.HasAttribute("Id",
                        value => value.StartsWith("APPLICATIONDATADIRECTORY") ||
                                 value.StartsWith("EXAMPLECONFSLOCATION")))
                    .ForEach(c => c.SetAttributeValue("KeyPath", "yes"));
                document
                    .Select("Wix/Product")
                    .AddElement("CustomActionRef", "Id=WixFailWhenDeferred");
                document
                    .Select("Wix/Product/InstallExecuteSequence")
                    .AddElement("DeleteServices", value: "(Installed AND (REMOVE=\"ALL\") AND NOT (WIX_UPGRADE_DETECTED OR UPGRADINGPRODUCTCODE))");
            };
            project.WixSourceFormated += (ref string content) => WixSourceFormated?.Invoke(content);
            project.WixSourceSaved += name => WixSourceSaved?.Invoke(name);

            project.UI = WUI.WixUI_Common;
            project.CustomUI = _agentInstallerUi;

            project.ResolveWildCards(pruneEmptyDirectories: true);

#if DEBUG_PROPERTIES
            project.BeforeInstall += args =>
            {
                var installed = args.Session.Property("Installed");
                var wixUpgradeDetected = args.Session.Property("WIX_UPGRADE_DETECTED");
                var remove = args.Session.Property("REMOVE");
                var upgradingProductCode = args.Session.Property("UPGRADINGPRODUCTCODE");
                var upgrading = args.Session.Property("Upgrading");
                var uninstalling = args.Session.Property("Uninstalling");

                var firstInstall = string.IsNullOrEmpty(installed) && string.IsNullOrEmpty(wixUpgradeDetected);
                var upgrade =  !string.IsNullOrEmpty(wixUpgradeDetected) && remove != "ALL";
                var uninstall =  !string.IsNullOrEmpty(installed) && remove == "ALL" && !(!string.IsNullOrEmpty(wixUpgradeDetected) || !string.IsNullOrEmpty(upgradingProductCode));
                var maintenance = !string.IsNullOrEmpty(installed) && string.IsNullOrEmpty(uninstalling) &&
                                  !string.IsNullOrEmpty(upgradingProductCode);
                var removingForUpgrade = remove == "ALL" && !string.IsNullOrEmpty(upgradingProductCode);

                MessageBox.Show($"installed={installed}\n" +
                                $"wixUpgradeDetected={wixUpgradeDetected}\n" +
                                $"remove={remove}\n" +
                                $"upgradingProductCode={upgradingProductCode}\n" +
                                $"upgrading={upgrading}\n" +
                                $"uninstalling={uninstalling}\n\n" +
                                $"firstInstall={firstInstall}\n" +
                                $"upgrade={upgrade}\n" +
                                $"uninstall={uninstall}\n" +
                                $"maintenance={maintenance}\n" +
                                $"removingForUpgrade={removingForUpgrade}", "BeforeInstall");
            };
#endif
            return project;
        }

        private Dir CreateProgramFilesFolder()
        {
            var targetBinFolder = CreateBinFolder();
            var binFolder =
                new Dir(new Id("PROJECTLOCATION"), "%ProgramFiles%\\Datadog\\Datadog Agent",
                    targetBinFolder,
                    new Dir("LICENSES",
                        new Files($@"{InstallerSource}\LICENSES\*")
                        ),
                    new DirFiles($@"{InstallerSource}\*.json"),
                    new DirFiles($@"{InstallerSource}\*.txt"),
                    new CompressedDir(this, "embedded3", $@"{InstallerSource}\embedded3")
                );
            if (_agentPython.IncludePython2)
            {
                binFolder.AddFile(new CompressedDir(this, "embedded2", $@"{InstallerSource}\embedded3"));
            }
            return binFolder;
        }

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
                StartOn = SvcEvent.Install_Wait,
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
                // Account must be a fully qualified name.
                Account = "[DDAGENTUSER_PROCESSED_FQ_NAME]",
                Password = "[DDAGENTUSER_PROCESSED_PASSWORD]"
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
                // Account must be a fully qualified name.
                Account = account,
                Password = password,
                DependsOn = new[]
                {
                    new ServiceDependency("datadogagent")
                }
            };
        }

        private Dir CreateBinFolder()
        {
            var agentService = GenerateServiceInstaller("datadogagent", "Datadog Agent", "Send metrics to Datadog");
            var processAgentService = GenerateDependentServiceInstaller(new Id("ddagentprocessservice"), "datadog-process-agent", "Datadog Process Agent", "Send process metrics to Datadog", "LocalSystem");
            var traceAgentService = GenerateDependentServiceInstaller(new Id("ddagenttraceservice"), "datadog-trace-agent", "Datadog Trace Agent", "Send tracing metrics to Datadog", "[DDAGENTUSER_PROCESSED_FQ_NAME]", "[DDAGENTUSER_PROCESSED_PASSWORD]");
            var systemProbeService = GenerateDependentServiceInstaller(new Id("ddagentsysprobeservice"), "datadog-system-probe", "Datadog System Probe", "Send network metrics to Datadog", "LocalSystem");

            var targetBinFolder = new Dir(new Id("BIN"), "bin",
                new WixSharp.File(_agentBinaries.Agent, agentService),
                new EventSource
                {
                    Name = "DatadogAgent",
                    Log = "Application",
                    EventMessageFile = $"[BIN]{Path.GetFileName(_agentBinaries.Agent)}",
                    AttributesDefinition = "SupportsErrors=yes; SupportsInformationals=yes; SupportsWarnings=yes"
                },
                new WixSharp.File(_agentBinaries.LibDatadogAgentThree),
                new Dir(new Id("AGENT"), "agent",
                    new Dir("dist",
                        new Files($@"{InstallerSource}\bin\agent\dist\*")
                    ),
                    new Dir("driver",
                        new Merge(_agentFeatures.Npm, $@"{BinSource}\agent\DDNPM.msm")
                        {
                            Feature = _agentFeatures.Npm
                        }
                    ),
                    new WixSharp.File(_agentBinaries.Tray),
                    new WixSharp.File(_agentBinaries.ProcessAgent, processAgentService),
                    new EventSource
                    {
                        Name = "datadog-process-agent",
                        Log = "Application",
                        EventMessageFile = $"[AGENT]{Path.GetFileName(_agentBinaries.ProcessAgent)}",
                        AttributesDefinition = "SupportsErrors=yes; SupportsInformationals=yes; SupportsWarnings=yes"
                    },
                    new WixSharp.File(_agentBinaries.SystemProbe, systemProbeService),
                    new EventSource
                    {
                        Name = "datadog-system-probe",
                        Log = "Application",
                        EventMessageFile = $"[AGENT]{Path.GetFileName(_agentBinaries.SystemProbe)}",
                        AttributesDefinition = "SupportsErrors=yes; SupportsInformationals=yes; SupportsWarnings=yes"
                    },
                    new WixSharp.File(_agentBinaries.TraceAgent, traceAgentService),
                    new EventSource
                    {
                        Name = "datadog-trace-agent",
                        Log = "Application",
                        EventMessageFile = $"[AGENT]{Path.GetFileName(_agentBinaries.TraceAgent)}",
                        AttributesDefinition = "SupportsErrors=yes; SupportsInformationals=yes; SupportsWarnings=yes"
                    }
                )
            );
            if (_agentPython.IncludePython2)
            {
                targetBinFolder.AddFile(new WixSharp.File(_agentBinaries.LibDatadogAgentTwo));
            };
            return targetBinFolder;
        }

        private Dir CreateAppDataFolder()
        {
            var appData = new Dir(new Id("APPLICATIONDATADIRECTORY"), "Datadog",
                new DirFiles($@"{EtcSource}\*.yaml.example"),
                new Dir("checks.d"),
                new Dir(new Id("EXAMPLECONFSLOCATION"), "conf.d",
                    new Files($@"{EtcSource}\extra_package_files\EXAMPLECONFSLOCATION\*")
                ));

            return new Dir(new Id("%CommonAppData%"), appData)
            {
                Attributes = { { "Name", "CommonAppData" } }
            };
        }

        public event XDocumentGeneratedDlgt WixSourceGenerated;
        public event XDocumentSavedDlgt WixSourceSaved;
        public event XDocumentFormatedDlgt WixSourceFormated;
    }
}
