using Datadog.InstallerCustomActions;
using WixSharp;

namespace WixSetup.Datadog_Installer
{
    public class DatadogInstallerCustomActions
    {
        public ManagedAction RunAsAdmin { get; }
        public ManagedAction ReadConfig { get; }
        public ManagedAction ReadInstallState { get; }
        public ManagedAction WriteInstallState { get; }
        public ManagedAction RollbackWriteInstallState { get; }
        public ManagedAction DeleteInstallState { get; }
        public ManagedAction RollbackDeleteInstallState { get; }
        public ManagedAction OpenMsiLog { get; }
        public ManagedAction ProcessDdAgentUserCredentials { get; }

        public DatadogInstallerCustomActions()
        {
            RunAsAdmin = new CustomAction<CustomActions>(
                new Id(nameof(RunAsAdmin)),
                CustomActions.EnsureAdminCaller,
                Return.check,
                When.After,
                Step.AppSearch,
                Condition.Always,
                Sequence.InstallExecuteSequence | Sequence.InstallUISequence
            );

            ReadInstallState = new CustomAction<CustomActions>(
                new Id(nameof(ReadInstallState)),
                CustomActions.ReadInstallState,
                Return.check,
                When.After,
                new Step(RunAsAdmin.Id),
                Condition.Always,
                Sequence.InstallExecuteSequence | Sequence.InstallUISequence
            )
            {
                Execute = Execute.firstSequence
            };

            ProcessDdAgentUserCredentials = new CustomAction<CustomActions>(
                    new Id(nameof(ProcessDdAgentUserCredentials)),
                    CustomActions.ProcessDdAgentUserCredentials,
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

            ReadConfig = new CustomAction<CustomActions>(
                new Id(nameof(ReadConfig)),
                CustomActions.ReadConfig,
                Return.ignore,
                When.After,
                Step.CostFinalize,
                Condition.Always,
                Sequence.InstallExecuteSequence | Sequence.InstallUISequence
            )
            {
                Execute = Execute.firstSequence
            }
            .SetProperties("APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY]");

            OpenMsiLog = new CustomAction<CustomActions>(
                new Id(nameof(OpenMsiLog)),
                CustomActions.OpenMsiLog
            )
            {
                // Not run in a sequence, run from button on fatalError dialog
                Sequence = Sequence.NotInSequence
            };

            WriteInstallState = new CustomAction<CustomActions>(
                    new Id(nameof(WriteInstallState)),
                    CustomActions.WriteInstallState,
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
                               "DDAGENTUSER_PROCESSED_NAME=[DDAGENTUSER_PROCESSED_NAME], " +
                               "DDAGENT_installedDomain=[DDAGENT_installedDomain], " +
                               "DDAGENT_installedUser=[DDAGENT_installedUser]");

            RollbackWriteInstallState = new CustomAction<CustomActions>(
                    new Id(nameof(RollbackWriteInstallState)),
                    CustomActions.DeleteInstallState,
                    Return.check,
                    When.Before,
                    new Step(WriteInstallState.Id),
                    // Run unless we are being uninstalled.
                    Condition.NOT(Conditions.Uninstalling | Conditions.RemovingForUpgrade)
                )
            {
                Execute = Execute.rollback,
                Impersonate = false,
            }
                .SetProperties("DDAGENTUSER_PROCESSED_DOMAIN=[DDAGENTUSER_PROCESSED_DOMAIN], " +
                               "DDAGENTUSER_PROCESSED_NAME=[DDAGENTUSER_PROCESSED_NAME], " +
                               "DDAGENT_installedDomain=[DDAGENT_installedDomain], " +
                               "DDAGENT_installedUser=[DDAGENT_installedUser]");

            DeleteInstallState = new CustomAction<CustomActions>(
                new Id(nameof(DeleteInstallState)),
                CustomActions.DeleteInstallState,
                Return.check,
                // Since this CA removes registry values it must run before the built-in RemoveRegistryValues
                // so that the built-in registry keys can be removed if they are empty.
                When.Before,
                Step.RemoveRegistryValues,
                Conditions.Uninstalling | Conditions.RemovingForUpgrade
            )
            {
                Execute = Execute.deferred,
                Impersonate = false
            };


            RollbackDeleteInstallState = new CustomAction<CustomActions>(
                new Id(nameof(RollbackDeleteInstallState)),
                CustomActions.WriteInstallState,
                Return.check,
                When.Before,
                new Step(DeleteInstallState.Id),
                Conditions.Uninstalling | Conditions.RemovingForUpgrade
            )
            {
                Execute = Execute.rollback,
                Impersonate = false,
            }
                .SetProperties("DDAGENTUSER_PROCESSED_DOMAIN=[DDAGENTUSER_PROCESSED_DOMAIN], " +
                               "DDAGENTUSER_PROCESSED_NAME=[DDAGENTUSER_PROCESSED_NAME], " +
                               "DDAGENT_installedDomain=[DDAGENT_installedDomain], " +
                               "DDAGENT_installedUser=[DDAGENT_installedUser]");
        }
    }
}
