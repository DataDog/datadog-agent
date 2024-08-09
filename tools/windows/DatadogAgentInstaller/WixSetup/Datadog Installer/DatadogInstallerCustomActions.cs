using Datadog.InstallerCustomActions;
using WixSharp;

namespace WixSetup.Datadog_Installer
{
    public class DatadogInstallerCustomActions
    {
        public ManagedAction RunAsAdmin { get; }
        public ManagedAction ReadConfig { get; }
        public ManagedAction WriteConfig { get; }
        public ManagedAction ReadInstallState { get; }
        public ManagedAction WriteInstallState { get; }
        public ManagedAction OpenMsiLog { get; }

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

            WriteConfig = new CustomAction<CustomActions>(
                new Id(nameof(WriteConfig)),
                CustomActions.WriteConfig,
                Return.check,
                When.Before,
                Step.InstallServices,
                Conditions.FirstInstall | Conditions.Upgrading | Conditions.Maintenance
            )
            {
                Execute = Execute.deferred,
                Impersonate = false
            }
            .SetProperties(
                "APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY]," +
                "APIKEY=[APIKEY], " +
                "SITE=[SITE]")
            .HideTarget(true);

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
                               "DDAGENTUSER_PROCESSED_NAME=[DDAGENTUSER_PROCESSED_NAME]");
        }
    }
}
