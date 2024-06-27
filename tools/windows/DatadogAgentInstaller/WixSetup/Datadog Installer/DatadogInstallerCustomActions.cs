using Datadog.CustomActions;
using WixSharp;

namespace WixSetup.Datadog_Installer
{
    public class DatadogInstallerCustomActions
    {
        public ManagedAction RunAsAdmin { get; }
        public ManagedAction ReadConfig { get; }
        public ManagedAction WriteConfig { get; }
        public ManagedAction ReadWindowsVersion { get; }
        public ManagedAction OpenMsiLog { get; }

        public DatadogInstallerCustomActions()
        {
            RunAsAdmin = new CustomAction<PrerequisitesCustomActions>(
                new Id(nameof(RunAsAdmin)),
                PrerequisitesCustomActions.EnsureAdminCaller,
                Return.check,
                When.After,
                Step.AppSearch,
                Condition.Always,
                Sequence.InstallExecuteSequence | Sequence.InstallUISequence
            );

            ReadWindowsVersion = new CustomAction<InstallStateCustomActions>(
                new Id(nameof(ReadWindowsVersion)),
                InstallStateCustomActions.ReadWindowsVersion,
                Return.check,
                When.After,
                new Step(RunAsAdmin.Id),
                Condition.Always,
                Sequence.InstallExecuteSequence | Sequence.InstallUISequence
            )
            {
                Execute = Execute.firstSequence
            };

            ReadConfig = new CustomAction<ConfigCustomActions>(
                new Id(nameof(ReadConfig)),
                ConfigCustomActions.ReadConfig,
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

            WriteConfig = new CustomAction<ConfigCustomActions>(
                new Id(nameof(WriteConfig)),
                ConfigCustomActions.WriteConfig,
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

            OpenMsiLog = new CustomAction<MsiLogCustomActions>(
                new Id(nameof(OpenMsiLog)),
                MsiLogCustomActions.OpenMsiLog
            )
            {
                // Not run in a sequence, run from button on fatalError dialog
                Sequence = Sequence.NotInSequence
            };
        }
    }
}
