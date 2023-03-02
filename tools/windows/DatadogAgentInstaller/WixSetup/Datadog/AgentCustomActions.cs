using System.Diagnostics.CodeAnalysis;
using Datadog.CustomActions;
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

        public ManagedAction WriteConfig { get; }

        public ManagedAction ReadRegistryProperties { get; }

        public ManagedAction ProcessDdAgentUserCredentials { get; }

        public ManagedAction PrepareDecompressPythonDistributions { get; }

        public ManagedAction DecompressPythonDistributions { get; }

        public ManagedAction CleanupOnRollback { get; }

        public ManagedAction CleanupOnUninstall { get; }

        public ManagedAction ConfigureUser { get; }

        public ManagedAction OpenMsiLog { get; }

        public ManagedAction SendFlare { get; }

        public ManagedAction WriteInstallInfo { get; }

        public ManagedAction ReportInstallFailure { get; }

        public ManagedAction ReportInstallSuccess { get; }

        public AgentCustomActions()
        {
            ReadRegistryProperties = new CustomAction<UserCustomActions>(
                new Id(nameof(ReadRegistryProperties)),
                UserCustomActions.ReadRegistryProperties,
                Return.ignore,
                // AppSearch is when RegistrySearch is run, so that will overwrite
                // any command line values.
                // Prefer using our CA over RegistrySearch.
                // It is executed on the Welcome screen of the installer.
                When.After,
                Step.AppSearch,
                Condition.NOT_BeingRemoved,
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
                Condition.NOT_BeingRemoved,
                // Run in either sequence so our CA is also run in non-UI installs
                Sequence.InstallExecuteSequence | Sequence.InstallUISequence
            )
            {
                // Ensure we only run in one sequence
                Execute = Execute.firstSequence
            }
            .SetProperties("APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY]");

            ReportInstallFailure = new CustomAction<Telemetry>(
                    new Id(nameof(ReportInstallFailure)),
                    Telemetry.ReportFailure,
                    Return.ignore,
                    When.Before,
                    Step.StartServices
            )
            {
                Execute = Execute.rollback
            }
                .SetProperties("APIKEY=[APIKEY], SITE=[SITE]");

            WriteConfig = new CustomAction<ConfigCustomActions>(
                new Id(nameof(WriteConfig)),
                ConfigCustomActions.WriteConfig,
                Return.check,
                When.After,
                new Step(ReportInstallFailure.Id),
                Conditions.FirstInstall
            )
            {
                Execute = Execute.deferred
            }
            .SetProperties(
                "APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY], " +
                "PROJECTLOCATION=[PROJECTLOCATION], " +
                "SYSPROBE_PRESENT=[SYSPROBE_PRESENT], " +
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
                // Only on first install otherwise we risk ruining the existing install
                Conditions.FirstInstall
            )
            {
                Execute = Execute.rollback
            }
            .SetProperties("PROJECTLOCATION=[PROJECTLOCATION], APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY]");

            DecompressPythonDistributions = new CustomAction<PythonDistributionCustomAction>(
                new Id(nameof(DecompressPythonDistributions)),
                PythonDistributionCustomAction.DecompressPythonDistributions,
                Return.check,
                When.After,
                new Step(CleanupOnRollback.Id),
                Conditions.FirstInstall | Conditions.Upgrading
            )
            {
                Execute = Execute.deferred
            }
            .SetProperties("PROJECTLOCATION=[PROJECTLOCATION], embedded2_SIZE=[embedded2_SIZE], embedded3_SIZE=[embedded3_SIZE]");

            PrepareDecompressPythonDistributions = new CustomAction<PythonDistributionCustomAction>(
                new Id(nameof(PrepareDecompressPythonDistributions)),
                PythonDistributionCustomAction.PrepareDecompressPythonDistributions,
                Return.ignore,
                When.Before,
                new Step(DecompressPythonDistributions.Id),
                Conditions.FirstInstall | Conditions.Upgrading,
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
                Execute = Execute.deferred
            }
            .SetProperties("PROJECTLOCATION=[PROJECTLOCATION], APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY]");

            ConfigureUser = new CustomAction<UserCustomActions>(
                    new Id(nameof(ConfigureUser)),
                    UserCustomActions.ConfigureUser,
                    Return.check,
                    When.After,
                    new Step(DecompressPythonDistributions.Id),
                    Condition.NOT(Conditions.Uninstalling)
            )
            {
                Execute = Execute.deferred
            }
            .SetProperties("APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY], " +
                           "PROJECTLOCATION=[PROJECTLOCATION], " +
                           "DDAGENTUSER_PROCESSED_FQ_NAME=[DDAGENTUSER_PROCESSED_FQ_NAME], " +
                           "DDAGENTUSER_SID=[DDAGENTUSER_SID]");

            ProcessDdAgentUserCredentials = new CustomAction<UserCustomActions>(
                new Id(nameof(ProcessDdAgentUserCredentials)),
                UserCustomActions.ProcessDdAgentUserCredentials,
                Return.check,
                // Run at end of "config phase", right before the "make changes" phase.
                When.Before,
                Step.InstallInitialize,
                // Run unless we are being uninstalled.
                // This CA produces properties used for services, accounts, and permissions.
                Condition.NOT(Conditions.Uninstalling)
            )
            .SetProperties("DDAGENTUSER_NAME=[DDAGENTUSER_NAME], DDAGENTUSER_PASSWORD=[DDAGENTUSER_PASSWORD]")
            .HideTarget(true);

            OpenMsiLog = new CustomAction<UserCustomActions>(
                new Id(nameof(OpenMsiLog)),
                UserCustomActions.OpenMsiLog
            )
            {
                Sequence = Sequence.NotInSequence
            };

            SendFlare = new CustomAction<Flare>(
                new Id(nameof(SendFlare)),
                Flare.SendFlare
            )
            {
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
                Execute = Execute.deferred
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
            .SetProperties("APIKEY=[APIKEY], SITE=[SITE]");
        }
    }
}
