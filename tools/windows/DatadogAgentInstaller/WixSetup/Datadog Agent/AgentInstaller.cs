using Datadog.AgentCustomActions;
using Datadog.CustomActions;
using NineDigit.WixSharpExtensions;
using System;
using System.IO;
using System.Linq;
using System.Xml.Linq;
using WixSharp;
using WixSharp.CommonTasks;

namespace WixSetup.Datadog_Agent
{
    public class AgentInstaller : IWixProjectEvents, IMsiInstallerProject
    {
        // Company
        private const string CompanyFullName = "Datadog, Inc.";

        // Product
        private const string ProductFullName = "Datadog Agent";
        private const string ProductDescription = "Datadog Agent {0}";
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
        private const string BinSource = @"C:\opt\datadog-agent\bin\agent";
        private const string EtcSource = @"C:\omnibus-ruby\src\etc\datadog-agent";

        private readonly AgentBinaries _agentBinaries;
        private readonly AgentFeatures _agentFeatures = new();
        private readonly AgentVersion _agentVersion;
        private readonly AgentCustomActions _agentCustomActions = new();
        private readonly AgentInstallerUI _agentInstallerUi;

        public AgentInstaller()
        : this(null)
        {

        }

        public AgentInstaller(string version)
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
            _agentInstallerUi = new AgentInstallerUI(this, _agentCustomActions);
        }

        public Project Configure()
        {
            var project = new ManagedProject("Datadog Agent",
                // Use 2 LaunchConditions, one for server versions,
                // one for client versions.
                MinimumSupportedWindowsVersion.WindowsServer2016 |
                MinimumSupportedWindowsVersion.Windows10,
                new Property("MsiLogging", "iwearucmop!"),
                new Property("MSIRESTARTMANAGERCONTROL", "Disable"),
                new Property("APIKEY")
                {
                    AttributesDefinition = "Hidden=yes;Secure=yes"
                },
                new Property("DDAGENTUSER_NAME")
                {
                    AttributesDefinition = "Secure=yes"
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
                // Custom WindowsBuild property since MSI caps theirs at 9600
                new Property("DDAGENT_WINDOWSBUILD")
                {
                    AttributesDefinition = "Secure=yes"
                },
                // set this property to anything to indicate to the merge module that on install rollback, it should
                // execute the install custom action rollback; otherwise it won't.
                new Property("DDDRIVERROLLBACK_NPM", "1"),
                new Property("DDDRIVERROLLBACK_PROCMON", "1"),
                // Add a checkbox at the end of the setup to launch the Datadog Agent Manager
                new LaunchCustomApplicationFromExitDialog(
                    _agentBinaries.TrayId,
                    "!(loc.LaunchAgentManager)",
                    "AGENT",
                    "\"[AGENT]ddtray.exe\" \"--launch-gui\""),
                new CloseApplication(new Id("CloseTrayApp"),
                    Path.GetFileName(_agentBinaries.Tray),
                    closeMessage: true,
                    rebootPrompt: false
                )
                {
                    Timeout = 1,
                    TerminateProcess = 1,
                    EndSessionMessage = true,
                    ElevatedCloseMessage = true,
                    ElevatedEndSessionMessage = true
                },
                new RegKey(
                    _agentFeatures.MainApplication,
                    RegistryHive.LocalMachine, @"Software\Datadog\Datadog Agent",
                    // Store these properties in the registry for retrieval by future
                    // installer runs via the ReadInstallState CA.
                    // Must set KeyPath=yes to ensure WiX# doesn't automatically try to use the parent Directory as the KeyPath,
                    // which can cause the directory to be added to the CreateFolder table.
                    new RegValue("InstallPath", "[PROJECTLOCATION]") { Win64 = true, AttributesDefinition = "KeyPath=yes" },
                    new RegValue("ConfigRoot", "[APPLICATIONDATADIRECTORY]") { Win64 = true, AttributesDefinition = "KeyPath=yes" }
                )
                {
                    Win64 = true
                }
            );

            // Always generate a new GUID otherwise WixSharp will generate one based on
            // the version
            project.ProductId = Guid.NewGuid();
            project
                .SetCustomActions(_agentCustomActions)
                .SetProjectInfo(
                    upgradeCode: ProductUpgradeCode,
                    name: ProductFullName,
                    description: string.Format(ProductDescription, _agentVersion.Version),
                    // This version is overridden below because SetProjectInfo throws an Exception if Revision is != 0
                    version: new Version(
                        _agentVersion.Version.Major,
                        _agentVersion.Version.Minor,
                        _agentVersion.Version.Build,
                        0)
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
                    new Dir(new Id("ProgramMenuDatadog"), @"%ProgramMenu%\Datadog",
                        new ExeFileShortcut
                        {
                            Name = "Datadog Agent Manager",
                            Target = "[AGENT]ddtray.exe",
                            Arguments = "\"--launch-gui\"",
                            WorkingDirectory = "AGENT",
                        }
                    )
                );

            project.SetNetFxPrerequisite(Condition.Net45_Installed,
                "This application requires the .Net Framework 4.5, or later to be installed.");

            // NineDigit.WixSharpExtensions SetProductInfo prohibits setting the revision, so we must do it here instead.
            // The revision is ignored by WiX during upgrades, so it is only useful for documentation purposes.
            project.Version = _agentVersion.Version;

            // Enable the ability to repair the installation even when the original MSI is no longer available.
            // This adds a symlink in %PROGRAMFILES%\Datadog\Datadog Agent which remains even when uninstalled
            // and makes the kitchen test fail.
            // Furthermore this symbolic link points to the locally cached MSI package (%WINDIR%\Installer)
            // and won't help if the customer removed it from there.
            //project.EnableResilientPackage();

            project.MajorUpgrade = MajorUpgrade.Default;
            // Set to true otherwise RC versions can't upgrade each other.
            project.MajorUpgrade.AllowSameVersionUpgrades = true;
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
            if (!string.IsNullOrEmpty(Environment.GetEnvironmentVariable("AGENT_MSI_OUTDIR")))
            {
                // Set custom output directory (WixSharp defaults to current directory)
                project.OutDir = Environment.GetEnvironmentVariable("AGENT_MSI_OUTDIR");
            }
            project.OutFileName = $"datadog-agent-{_agentVersion.PackageVersion}-1-x86_64";
            project.Package.AttributesDefinition = $"Comments={ProductComment}";

            // clear default media as we will add it via MediaTemplate
            project.Media.Clear();
            project.WixSourceGenerated += document =>
            {
                WixSourceGenerated?.Invoke(document);
                document
                    .Select("Wix/Product")
                    .AddElement("MediaTemplate",
                        "CabinetTemplate=cab{0}.cab; CompressionLevel=high; EmbedCab=yes; MaximumUncompressedMediaSize=2");

                // Windows Installer (MSI.dll) calls the obsolete SetFileSecurityW function during CreateFolder rollback,
                // this causes directories in the CreateFolder table to have their SE_DACL_AUTO_INHERITED flag removed.
                // Also, since folders created by CreateFolder aren't removed on uninstall then it can cause empty directories
                // to be left behind on uninstall.
                document.FindAll("CreateFolder")
                    .ForEach(x => x.Remove());
                document.FindAll("RemoveFolder")
                    .ForEach(x => x.Remove());

                // Wix# is auto-adding components for the following directories for some reason, which causes them to be placed
                // in the CreateFolder table.
                document
                    .FindAll("Component")
                    .Where(x => x.HasAttribute("Id",
                        value => value.Equals("TARGETDIR") ||
                                 value.Equals("ProgramFiles64Folder") ||
                                 value.Equals("DatadogAppRoot")))
                    .Remove();
                document
                    .FindAll("ComponentRef")
                    .Where(x => x.HasAttribute("Id",
                        value => value.Equals("TARGETDIR") ||
                                 value.Equals("ProgramFiles64Folder") ||
                                 value.Equals("DatadogAppRoot")))
                    .Remove();
                // END TODO: Wix# adds these automatically
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
                    .AddElement("DeleteServices",
                        value:
                        "(Installed AND (REMOVE=\"ALL\") AND NOT (WIX_UPGRADE_DETECTED OR UPGRADINGPRODUCTCODE))");

                // We don't use the Wix "Merge" MSM feature because it seems to be a no-op...
                document
                    .FindAll("Directory")
                    .First(x => x.HasAttribute("Id", value => value == "AGENT"))
                    .AddElement("Directory", "Id=DRIVER; Name=driver")
                    .AddElement("Merge",
                        $"Id=ddnpminstall; SourceFile={BinSource}\\DDNPM.msm; DiskId=1; Language=1033");
                document
                    .FindAll("Feature")
                    .First(x => x.HasAttribute("Id", value => value == "MainApplication"))
                    .AddElement("MergeRef", "Id=ddnpminstall");
                // Conditionally include the APM injection MSM while it is in active development to make it easier
                // to build/ship without it.
                if (!string.IsNullOrEmpty(Environment.GetEnvironmentVariable("WINDOWS_APMINJECT_MODULE")))
                {
                    document
                        .FindAll("Directory")
                        .First(x => x.HasAttribute("Id", value => value == "AGENT"))
                        .AddElement("Merge",
                            $"Id=ddapminstall; SourceFile={BinSource}\\ddapminstall.msm; DiskId=1; Language=1033");
                    document
                        .FindAll("Feature")
                        .First(x => x.HasAttribute("Id", value => value == "MainApplication"))
                        .AddElement("MergeRef", "Id=ddapminstall");
                }
                document
                    .FindAll("Directory")
                    .First(x => x.HasAttribute("Id", value => value == "AGENT"))
                    .AddElement("Merge",
                        $"Id=ddprocmoninstall; SourceFile={BinSource}\\ddprocmon.msm; DiskId=1; Language=1033");
                document
                    .FindAll("Feature")
                    .First(x => x.HasAttribute("Id", value => value == "MainApplication"))
                    .AddElement("MergeRef", "Id=ddprocmoninstall");
            };
            project.WixSourceFormated += (ref string content) => WixSourceFormated?.Invoke(content);
            project.WixSourceSaved += name => WixSourceSaved?.Invoke(name);

            project.UI = WUI.WixUI_Common;
            project.CustomUI = _agentInstallerUi;


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
                var upgrade = !string.IsNullOrEmpty(wixUpgradeDetected) && remove != "ALL";
                var uninstall =
 !string.IsNullOrEmpty(installed) && remove == "ALL" && !(!string.IsNullOrEmpty(wixUpgradeDetected) || !string.IsNullOrEmpty(upgradingProductCode));
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
            var datadogAgentFolder = new InstallDir(new Id("PROJECTLOCATION"), "Datadog Agent",
                CreateBinFolder(),
                new Dir("LICENSES",
                    new Files($@"{InstallerSource}\LICENSES\*")
                ),
                new DirFiles($@"{InstallerSource}\LICENSE"),
                new DirFiles($@"{InstallerSource}\*.json"),
                new DirFiles($@"{InstallerSource}\*.txt"),
                new CompressedDir(this, "embedded3", $@"{InstallerSource}\embedded3")
            );

            // Recursively delete/backup all files/folders in these paths, they will be restored
            // on rollback. By default WindowsInstller only removes the files it tracks, and these paths
            // may contain untracked files.
            // These properties are set in the ReadInstallState custom action.
            // https://wixtoolset.org/docs/v3/xsd/util/removefolderex/
            foreach (var property in ReadInstallStateCA.PathsToRemoveOnUninstall().Keys)
            {
                datadogAgentFolder.Add(
                    new RemoveFolderEx
                    {
                        On = InstallEvent.uninstall,
                        Property = property
                    }
                );
            }

            return new Dir(new Id("DatadogAppRoot"), "%ProgramFiles%\\Datadog", datadogAgentFolder);
        }

        private static ServiceInstaller GenerateServiceInstaller(string name, string displayName, string description)
        {
            return new ServiceInstaller
            {
                Id = new Id("ddagentservice"),
                Name = name,
                DisplayName = displayName,
                Description = description,
                // Tell MSI not to start the services. We handle service start manually in StartDDServices custom action.
                StartOn = null,
                // Tell MSI not to stop the services. We handle service stop manually in StopDDServices custom action.
                StopOn = null,
                Start = SvcStartType.auto,
                DelayedAutoStart = true,
                RemoveOn = SvcEvent.Uninstall_Wait,
                ServiceSid = ServiceSid.none,
                FirstFailureActionType = FailureActionType.restart,
                SecondFailureActionType = FailureActionType.restart,
                ThirdFailureActionType = FailureActionType.restart,
                RestartServiceDelayInSeconds = 60,
                ResetPeriodInDays = 0,
                PreShutdownDelay = 1000 * 60 * 3,
                // Account must be a fully qualified name.
                Account = "[DDAGENTUSER_PROCESSED_FQ_NAME]",
                Password = "[DDAGENTUSER_PROCESSED_PASSWORD]"
            };
        }

        private static ServiceInstaller GenerateDependentServiceInstaller(
            Id id,
            string name,
            string displayName,
            string description,
            string account,
            string password = null,
            string arguments = null)
        {
            return new ServiceInstaller
            {
                Id = id,
                Name = name,
                DisplayName = displayName,
                Description = description,
                // Tell MSI not to start the services. We handle service start manually in StartDDServices custom action.
                StartOn = null,
                // Tell MSI not to stop the services. We handle service stop manually in StopDDServices custom action.
                StopOn = null,
                Start = SvcStartType.demand,
                RemoveOn = SvcEvent.Uninstall_Wait,
                ServiceSid = ServiceSid.none,
                FirstFailureActionType = FailureActionType.restart,
                SecondFailureActionType = FailureActionType.restart,
                ThirdFailureActionType = FailureActionType.restart,
                RestartServiceDelayInSeconds = 60,
                ResetPeriodInDays = 0,
                PreShutdownDelay = 1000 * 60 * 3,
                Interactive = false,
                Type = SvcType.ownProcess,
                // Account must be a fully qualified name.
                Account = account,
                Password = password,
                Arguments = arguments,
                DependsOn = new[]
                {
                    new ServiceDependency(Constants.AgentServiceName)
                }
            };
        }

        private Dir CreateBinFolder()
        {
            var agentService =
                GenerateServiceInstaller(Constants.AgentServiceName, "Datadog Agent", "Send metrics to Datadog");
            var processAgentService = GenerateDependentServiceInstaller(
                new Id("ddagentprocessservice"),
                Constants.ProcessAgentServiceName,
                "Datadog Process Agent",
                "Send process metrics to Datadog",
                "LocalSystem",
                null,
                "--cfgpath=\"[APPLICATIONDATADIRECTORY]\\datadog.yaml\"");
            var traceAgentService = GenerateDependentServiceInstaller(
                new Id("ddagenttraceservice"),
                Constants.TraceAgentServiceName,
                "Datadog Trace Agent",
                "Send tracing metrics to Datadog",
                "[DDAGENTUSER_PROCESSED_FQ_NAME]",
                "[DDAGENTUSER_PROCESSED_PASSWORD]",
                "--config=\"[APPLICATIONDATADIRECTORY]\\datadog.yaml\"");
            var systemProbeService = GenerateDependentServiceInstaller(
                new Id("ddagentsysprobeservice"),
                Constants.SystemProbeServiceName,
                "Datadog System Probe",
                "Send network metrics to Datadog",
                "LocalSystem");

            var agentBinDir = new Dir(new Id("AGENT"), "agent",
                    new Dir("dist",
                        new Files($@"{InstallerSource}\bin\agent\dist\*")
                    ),
                    new WixSharp.File(_agentBinaries.TrayId, _agentBinaries.Tray),
                    new WixSharp.File(_agentBinaries.ProcessAgent, processAgentService),
                    new EventSource
                    {
                        Name = Constants.ProcessAgentServiceName,
                        Log = "Application",
                        EventMessageFile = $"[AGENT]{Path.GetFileName(_agentBinaries.ProcessAgent)}",
                        AttributesDefinition = "SupportsErrors=yes; SupportsInformationals=yes; SupportsWarnings=yes; KeyPath=yes"
                    },
                    new WixSharp.File(_agentBinaries.SystemProbe, systemProbeService),
                    new EventSource
                    {
                        Name = Constants.SystemProbeServiceName,
                        Log = "Application",
                        EventMessageFile = $"[AGENT]{Path.GetFileName(_agentBinaries.SystemProbe)}",
                        AttributesDefinition = "SupportsErrors=yes; SupportsInformationals=yes; SupportsWarnings=yes; KeyPath=yes"
                    },
                    new WixSharp.File(_agentBinaries.TraceAgent, traceAgentService),
                    new EventSource
                    {
                        Name = Constants.TraceAgentServiceName,
                        Log = "Application",
                        EventMessageFile = $"[AGENT]{Path.GetFileName(_agentBinaries.TraceAgent)}",
                        AttributesDefinition = "SupportsErrors=yes; SupportsInformationals=yes; SupportsWarnings=yes; KeyPath=yes"
                    }
            );
            var securityAgentService = GenerateDependentServiceInstaller(
                new Id("ddagentsecurityservice"),
                Constants.SecurityAgentServiceName,
                "Datadog Security Agent",
                "Send Security events to Datadog",
                "[DDAGENTUSER_PROCESSED_FQ_NAME]",
                "[DDAGENTUSER_PROCESSED_PASSWORD]");
            agentBinDir.AddFile(new WixSharp.File(_agentBinaries.SecurityAgent, securityAgentService));

            agentBinDir.Add(new EventSource
            {
                Name = Constants.SecurityAgentServiceName,
                Log = "Application",
                EventMessageFile = $"[AGENT]{Path.GetFileName(_agentBinaries.SecurityAgent)}",
                AttributesDefinition = "SupportsErrors=yes; SupportsInformationals=yes; SupportsWarnings=yes; KeyPath=yes"
            }
            );
            var targetBinFolder = new Dir(new Id("BIN"), "bin",
                new WixSharp.File(_agentBinaries.Agent, agentService),
                // Temporary binary for extracting the embedded Python - will be deleted
                // by the CustomAction
                new WixSharp.File(new Id("sevenzipr"), Path.Combine(BinSource, "7zr.exe")),
                // Each EventSource must have KeyPath=yes to avoid having the parent directory placed in the CreateFolder table.
                // The EventSource supports being a KeyPath.
                // https://wixtoolset.org/docs/v3/xsd/util/eventsource/
                new EventSource
                {
                    Name = Constants.AgentServiceName,
                    Log = "Application",
                    EventMessageFile = $"[BIN]{Path.GetFileName(_agentBinaries.Agent)}",
                    AttributesDefinition = "SupportsErrors=yes; SupportsInformationals=yes; SupportsWarnings=yes; KeyPath=yes"
                },
                agentBinDir,
                new WixSharp.File(_agentBinaries.LibDatadogAgentThree)
            );

            return targetBinFolder;
        }

        private Dir CreateAppDataFolder()
        {
            var appData = new Dir(new Id("APPLICATIONDATADIRECTORY"), "Datadog",
                new DirFiles($@"{EtcSource}\*.yaml.example"),
                new Dir("checks.d"),
                new Dir("run"),
                new Dir("logs"),
                new Dir(new Id("EXAMPLECONFSLOCATION"), "conf.d",
                    new Files($@"{EtcSource}\extra_package_files\EXAMPLECONFSLOCATION\*")
                ));

            appData.AddDir(new Dir(new Id("security.d"),
                                    "runtime-security.d",
                                    new WixSharp.File($@"{EtcSource}\runtime-security.d\default.policy.example")
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
