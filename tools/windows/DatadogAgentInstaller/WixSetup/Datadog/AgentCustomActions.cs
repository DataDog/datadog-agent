using System.Diagnostics.CodeAnalysis;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Microsoft.Deployment.WindowsInstaller;
using WixSharp;

namespace WixSetup.Datadog
{
    public class AgentCustomActions
    {
        [SuppressMessage("ReSharper", "InconsistentNaming")]
        private static readonly Condition Being_Reinstalled = Condition.Create("(REINSTALL<>\"\")");

        [SuppressMessage("ReSharper", "InconsistentNaming")]
        private static readonly Condition NOT_Being_Reinstalled = Condition.NOT(Being_Reinstalled);

        public ManagedAction ReadConfig { get; }

        public ManagedAction PatchInstaller { get; set; }

        public ManagedAction WriteConfig { get; }

        public ManagedAction ReadInstallState { get; }

        public ManagedAction WriteInstallState { get; }

        public ManagedAction UninstallWriteInstallState { get; }

        public ManagedAction ProcessDdAgentUserCredentials { get; }

        public ManagedAction ProcessDdAgentUserCredentialsUI { get; }

        public ManagedAction PrepareDecompressPythonDistributions { get; }

        public ManagedAction DecompressPythonDistributions { get; }

        public ManagedAction CleanupOnRollback { get; }

        public ManagedAction CleanupOnUninstall { get; }

        public ManagedAction ConfigureUser { get; }

        public ManagedAction ConfigureUserRollback { get; }

        public ManagedAction UninstallUser { get; }

        public ManagedAction UninstallUserRollback { get; }

        public ManagedAction OpenMsiLog { get; }

        public ManagedAction SendFlare { get; }

        public ManagedAction WriteInstallInfo { get; }

        public ManagedAction ReportInstallFailure { get; }

        public ManagedAction ReportInstallSuccess { get; }

        public ManagedAction EnsureNpmServiceDepdendency { get; }

        public ManagedAction ConfigureServiceUsers { get; }

        public ManagedAction StopDDServices { get; }

        public ManagedAction StartDDServices { get; }

        public ManagedAction StartDDServicesRollback { get; }

        /// <summary>
        /// Registers and sequences our custom actions
        /// </summary>
        /// <remarks>
        /// Please refer to https://learn.microsoft.com/en-us/windows/win32/msi/sequencing-custom-actions
        /// </remarks>
        public AgentCustomActions()
        {
            ReadInstallState = new CustomAction<InstallStateCustomActions>(
                new Id(nameof(ReadInstallState)),
                InstallStateCustomActions.ReadInstallState,
                Return.check,
                // AppSearch is when ReadInstallState is run, so that will overwrite
                // any command line values.
                // Prefer using our CA over RegistrySearch.
                // It is executed on the Welcome screen of the installer.
                When.After,
                Step.AppSearch,
                // Creates properties used by both install+uninstall
                Condition.Always,
                // Run in either sequence so our CA is also run in non-UI installs
                Sequence.InstallExecuteSequence | Sequence.InstallUISequence
            )
            {
                // Ensure we only run in one sequence
                Execute = Execute.firstSequence
            };

            // We need to explicitly set the ID since that we are going to reference before the Build* call.
            // See <see cref="WixSharp.WixEntity.Id" /> for more information.
            ReadConfig = new CustomAction<ConfigCustomActions>(
                    new Id(nameof(ReadConfig)),
                    ConfigCustomActions.ReadConfig,
                    Return.ignore,
                    When.After,
                    // Must execute after CostFinalize since we depend
                    // on APPLICATIONDATADIRECTORY being set.
                    Step.CostFinalize,
                    // Not needed during uninstall, but since it runs before InstallValidate the recommended
                    // REMOVE=ALL condition does not work, so always run it.
                    Condition.Always,
                    // Run in either sequence so our CA is also run in non-UI installs
                    Sequence.InstallExecuteSequence | Sequence.InstallUISequence
                )
                {
                    // Ensure we only run in one sequence
                    Execute = Execute.firstSequence
                }
                .SetProperties("APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY]");

            PatchInstaller = new CustomAction<PatchInstallerCustomAction>(
                new Id(nameof(PatchInstaller)),
                PatchInstallerCustomAction.Patch,
                Return.ignore,
                When.After,
                Step.InstallFiles,
                Conditions.Upgrading
            )
            {
                Execute = Execute.deferred,
                Impersonate = false
            };

            ReportInstallFailure = new CustomAction<Telemetry>(
                    new Id(nameof(ReportInstallFailure)),
                    Telemetry.ReportFailure,
                    Return.ignore,
                    When.After,
                    Step.InstallInitialize
                )
                {
                    Execute = Execute.rollback,
                    Impersonate = false
                }
                .SetProperties("APIKEY=[APIKEY], SITE=[SITE]")
                .HideTarget(true);

            EnsureNpmServiceDepdendency = new CustomAction<ServiceCustomAction>(
                new Id(nameof(EnsureNpmServiceDepdendency)),
                ServiceCustomAction.EnsureNpmServiceDependency,
                Return.check,
                When.After,
                Step.InstallServices,
                Conditions.FirstInstall | Conditions.Upgrading | Conditions.Maintenance
            )
            {
                Execute = Execute.deferred,
                Impersonate = false
            };

            WriteConfig = new CustomAction<ConfigCustomActions>(
                    new Id(nameof(WriteConfig)),
                    ConfigCustomActions.WriteConfig,
                    Return.check,
                    When.Before,
                    Step.InstallServices,
                    Conditions.FirstInstall
                )
                {
                    Execute = Execute.deferred,
                    Impersonate = false
                }
                .SetProperties(
                    "APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY], " +
                    "PROJECTLOCATION=[PROJECTLOCATION], " +
                    "SYSPROBE_PRESENT=[SYSPROBE_PRESENT], " +
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
                    "EC2_USE_WINDOWS_PREFIX_DETECTION=[EC2_USE_WINDOWS_PREFIX_DETECTION]")
                .HideTarget(true);

            // Cleanup leftover files on rollback
            // must be before the DecompressPythonDistributions custom action.
            // That way, if DecompressPythonDistributions fails, this will get executed.
            CleanupOnRollback = new CustomAction<CleanUpFilesCustomAction>(
                    new Id(nameof(CleanupOnRollback)),
                    CleanUpFilesCustomAction.CleanupFiles,
                    Return.check,
                    When.After,
                    new Step(WriteConfig.Id),
                    Conditions.FirstInstall | Conditions.Upgrading | Conditions.Maintenance
                )
                {
                    Execute = Execute.rollback,
                    Impersonate = false
                }
                .SetProperties(
                    "PROJECTLOCATION=[PROJECTLOCATION], APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY]");

            DecompressPythonDistributions = new CustomAction<PythonDistributionCustomAction>(
                    new Id(nameof(DecompressPythonDistributions)),
                    PythonDistributionCustomAction.DecompressPythonDistributions,
                    Return.check,
                    When.After,
                    new Step(CleanupOnRollback.Id),
                    Conditions.FirstInstall | Conditions.Upgrading | Conditions.Maintenance
                )
                {
                    Execute = Execute.deferred,
                    Impersonate = false
                }
                .SetProperties(
                    "PROJECTLOCATION=[PROJECTLOCATION], embedded2_SIZE=[embedded2_SIZE], embedded3_SIZE=[embedded3_SIZE]");

            PrepareDecompressPythonDistributions = new CustomAction<PythonDistributionCustomAction>(
                new Id(nameof(PrepareDecompressPythonDistributions)),
                PythonDistributionCustomAction.PrepareDecompressPythonDistributions,
                Return.ignore,
                When.Before,
                new Step(DecompressPythonDistributions.Id),
                Conditions.FirstInstall | Conditions.Upgrading | Conditions.Maintenance,
                Sequence.InstallExecuteSequence
            )
            {
                Execute = Execute.immediate
            };

            // Cleanup leftover files on uninstall
            CleanupOnUninstall = new CustomAction<CleanUpFilesCustomAction>(
                    new Id(nameof(CleanupOnUninstall)),
                    CleanUpFilesCustomAction.CleanupFiles,
                    Return.check,
                    When.Before,
                    Step.RemoveFiles,
                    Conditions.Uninstalling
                )
                {
                    Execute = Execute.deferred,
                    Impersonate = false
                }
                .SetProperties(
                    "PROJECTLOCATION=[PROJECTLOCATION], APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY]");

            ConfigureUser = new CustomAction<ConfigureUserCustomActions>(
                    new Id(nameof(ConfigureUser)),
                    ConfigureUserCustomActions.ConfigureUser,
                    Return.check,
                    When.After,
                    new Step(DecompressPythonDistributions.Id),
                    Condition.NOT(Conditions.Uninstalling | Conditions.RemovingForUpgrade)
                )
                {
                    Execute = Execute.deferred,
                    Impersonate = false
                }
                .SetProperties("APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY], " +
                               "PROJECTLOCATION=[PROJECTLOCATION], " +
                               "DDAGENTUSER_PROCESSED_NAME=[DDAGENTUSER_PROCESSED_NAME], " +
                               "DDAGENTUSER_PROCESSED_FQ_NAME=[DDAGENTUSER_PROCESSED_FQ_NAME], " +
                               "DDAGENTUSER_PROCESSED_PASSWORD=[DDAGENTUSER_PROCESSED_PASSWORD], " +
                               "DDAGENTUSER_FOUND=[DDAGENTUSER_FOUND], " +
                               "DDAGENTUSER_SID=[DDAGENTUSER_SID], " +
                               "DDAGENTUSER_RESET_PASSWORD=[DDAGENTUSER_RESET_PASSWORD], " +
                               "WIX_UPGRADE_DETECTED=[WIX_UPGRADE_DETECTED]")
                .HideTarget(true);

            ConfigureUserRollback = new CustomAction<ConfigureUserCustomActions>(
                    new Id(nameof(ConfigureUserRollback)),
                    ConfigureUserCustomActions.ConfigureUserRollback,
                    Return.check,
                    When.Before,
                    new Step(ConfigureUser.Id),
                    Condition.NOT(Conditions.Uninstalling | Conditions.RemovingForUpgrade)
                )
                {
                    Execute = Execute.rollback,
                    Impersonate = false,
                };

            UninstallUser = new CustomAction<ConfigureUserCustomActions>(
                    new Id(nameof(UninstallUser)),
                    ConfigureUserCustomActions.UninstallUser,
                    Return.check,
                    When.After,
                    Step.StopServices,
                    Conditions.Uninstalling | Conditions.RemovingForUpgrade
                )
                {
                    Execute = Execute.deferred,
                    Impersonate = false
                }
                .SetProperties("APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY], " +
                               "PROJECTLOCATION=[PROJECTLOCATION], " +
                               "DDAGENTUSER_NAME=[DDAGENTUSER_NAME]");

            UninstallUserRollback = new CustomAction<ConfigureUserCustomActions>(
                    new Id(nameof(UninstallUserRollback)),
                    ConfigureUserCustomActions.UninstallUserRollback,
                    Return.check,
                    When.Before,
                    new Step(UninstallUser.Id),
                    Conditions.Uninstalling | Conditions.RemovingForUpgrade
                )
                {
                    Execute = Execute.rollback,
                    Impersonate = false,
                };

            ProcessDdAgentUserCredentials = new CustomAction<ProcessUserCustomActions>(
                    new Id(nameof(ProcessDdAgentUserCredentials)),
                    ProcessUserCustomActions.ProcessDdAgentUserCredentials,
                    Return.check,
                    // Run at end of "config phase", right before the "make changes" phase.
                    // Ensure no actions that modify the input properties are run after this action.
                    When.Before,
                    Step.InstallInitialize,
                    // Run unless we are being uninstalled.
                    // This CA produces properties used for services, accounts, and permissions.
                    Condition.NOT(Conditions.Uninstalling | Conditions.RemovingForUpgrade)
                )
                .SetProperties("DDAGENTUSER_NAME=[DDAGENTUSER_NAME], " +
                               "DDAGENTUSER_PASSWORD=[DDAGENTUSER_PASSWORD], " +
                               "DDAGENTUSER_PROCESSED_FQ_NAME=[DDAGENTUSER_PROCESSED_FQ_NAME]")
                .HideTarget(true);

            ProcessDdAgentUserCredentialsUI = new CustomAction<ProcessUserCustomActions>(
                new Id(nameof(ProcessDdAgentUserCredentialsUI)),
                ProcessUserCustomActions.ProcessDdAgentUserCredentialsUI
            )
            {
                // Not run in a sequence, run when Next is clicked on ddagentuserdlg
                Sequence = Sequence.NotInSequence
            };

            OpenMsiLog = new CustomAction<MsiLogCustomActions>(
                new Id(nameof(OpenMsiLog)),
                MsiLogCustomActions.OpenMsiLog
            )
            {
                // Not run in a sequence, run from button on fatalError dialog
                Sequence = Sequence.NotInSequence
            };

            SendFlare = new CustomAction<Flare>(
                new Id(nameof(SendFlare)),
                Flare.SendFlare
            )
            {
                // Not run in a sequence, run from button on fatalError dialog
                Sequence = Sequence.NotInSequence
            };

            WriteInstallInfo = new CustomAction<InstallInfoCustomActions>(
                    new Id(nameof(WriteInstallInfo)),
                    InstallInfoCustomActions.WriteInstallInfo,
                    Return.ignore,
                    When.Before,
                    Step.StartServices,
                    // Include "Being_Reinstalled" so that if customer changes install method
                    // the install_info reflects that.
                    Conditions.FirstInstall | Conditions.Upgrading
                )
                {
                    Execute = Execute.deferred,
                    Impersonate = false
                }
                .SetProperties("APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY]," +
                               "OVERRIDE_INSTALLATION_METHOD=[OVERRIDE_INSTALLATION_METHOD]");

            // Hitting this CustomAction always means the install succeeded
            // because when an install fails, it rollbacks from the `InstallFinalize`
            // step.
            ReportInstallSuccess = new CustomAction<Telemetry>(
                    new Id(nameof(ReportInstallSuccess)),
                    Telemetry.ReportSuccess,
                    Return.ignore,
                    When.After,
                    Step.InstallFinalize,
                    Conditions.FirstInstall | Conditions.Upgrading
                )
                .SetProperties("APIKEY=[APIKEY], SITE=[SITE]")
                .HideTarget(true);

            // Enables the user to change the service accounts during upgrade/change
            // Relies on StopDDServices/StartDDServices to ensure the services are restarted
            // so that the new configuration is used.
            ConfigureServiceUsers = new CustomAction<ServiceCustomAction>(
                    new Id(nameof(ConfigureServiceUsers)),
                    ServiceCustomAction.ConfigureServiceUsers,
                    Return.check,
                    When.After,
                    Step.InstallServices,
                    Condition.NOT(Conditions.Uninstalling | Conditions.RemovingForUpgrade)
                )
                {
                    Execute = Execute.deferred,
                    Impersonate = false
                }
                .SetProperties("DDAGENTUSER_PROCESSED_PASSWORD=[DDAGENTUSER_PROCESSED_PASSWORD], " +
                               "DDAGENTUSER_PROCESSED_FQ_NAME=[DDAGENTUSER_PROCESSED_FQ_NAME]")
                .HideTarget(true);

            // WiX built-in StopServices only stops services if the component is changing.
            // This means that the services associated with MainApplication won't be restarted
            // during change operations.
            StopDDServices = new CustomAction<ServiceCustomAction>(
                new Id(nameof(StopDDServices)),
                ServiceCustomAction.StopDDServices,
                Return.check,
                When.Before,
                Step.StopServices
            )
            {
                Execute = Execute.deferred,
                Impersonate = false
            };

            // WiX built-in StartServices only starts services if the component is changing.
            // This means that the services associated with MainApplication won't be restarted
            // during change operations.
            StartDDServices = new CustomAction<ServiceCustomAction>(
                new Id(nameof(StartDDServices)),
                ServiceCustomAction.StartDDServices,
                Return.check,
                When.After,
                Step.StartServices,
                Condition.NOT(Conditions.Uninstalling | Conditions.RemovingForUpgrade)
            )
            {
                Execute = Execute.deferred,
                Impersonate = false
            };

            // Rollback StartDDServices stops the the services so that any file locks are released.
            StartDDServicesRollback = new CustomAction<ServiceCustomAction>(
                new Id(nameof(StartDDServicesRollback)),
                ServiceCustomAction.StartDDServicesRollback,
                Return.ignore,
                // Must be sequenced before the action it will rollback for
                When.Before,
                new Step(StartDDServices.Id),
                // Must have same condition as the action it will rollback for
                Condition.NOT(Conditions.Uninstalling | Conditions.RemovingForUpgrade)
            )
            {
                Execute = Execute.rollback,
                Impersonate = false
            };

            WriteInstallState = new CustomAction<InstallStateCustomActions>(
                    new Id(nameof(WriteInstallState)),
                    InstallStateCustomActions.WriteInstallState,
                    Return.check,
                    When.Before,
                    Step.StartServices,
                    // Run unless we are being uninstalled.
                    Condition.NOT(Conditions.Uninstalling | Conditions.RemovingForUpgrade)
                )
                {
                    Execute = Execute.deferred,
                    Impersonate = false
                }
                .SetProperties("DDAGENTUSER_PROCESSED_DOMAIN=[DDAGENTUSER_PROCESSED_DOMAIN], " +
                               "DDAGENTUSER_PROCESSED_NAME=[DDAGENTUSER_PROCESSED_NAME]");

            UninstallWriteInstallState = new CustomAction<InstallStateCustomActions>(
                    new Id(nameof(UninstallWriteInstallState)),
                    InstallStateCustomActions.UninstallWriteInstallState,
                    Return.check,
                    // Since this CA removes registry values it must run before the built-in RemoveRegistryValues
                    // so that the built-in registry keys can be removed if they are empty.
                    When.Before,
                    Step.RemoveRegistryValues,
                    // Run only on full uninstall
                    Conditions.Uninstalling
                )
                {
                    Execute = Execute.deferred,
                    Impersonate = false
                };
        }
    }
}
