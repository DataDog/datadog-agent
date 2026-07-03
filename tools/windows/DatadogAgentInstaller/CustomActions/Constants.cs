// ReSharper disable InconsistentNaming
namespace Datadog.CustomActions
{
    public class Constants
    {
        public const string AgentServiceName = "datadogagent";
        public const string TraceAgentServiceName = "datadog-trace-agent";
        public const string ProcessAgentServiceName = "datadog-process-agent";
        public const string SystemProbeServiceName = "datadog-system-probe";
        public const string SecurityAgentServiceName = "datadog-security-agent";
        public const string PrivateActionRunnerServiceName = "datadog-agent-action";
        public const string InstallerServiceName = "Datadog Installer";
        public const string NpmServiceName = "ddnpm";
        public const string ProcmonServiceName = "ddprocmon";
        public const string ProcmgrServiceName = "dd-procmgr-service";

        // Set to 1 when postinst creates processes.d/datadog-agent-action.yaml for the first time
        // during an MSI install/upgrade; used to scope upgrade rollback cleanup.
        public const string PARProcmgrConfigWrittenThisInstallValue = "PARProcmgrConfigWrittenThisInstall";

        // Set to 1 when postinst creates processes.d/datadog-agent-process.yaml for the first time
        // during an MSI install/upgrade; used to scope upgrade rollback cleanup.
        public const string ProcessProcmgrConfigWrittenThisInstallValue = "ProcessProcmgrConfigWrittenThisInstall";

        // Key under HKLM that contains our options
        public const string DatadogAgentRegistryKey = @"Software\Datadog\Datadog Agent";

        // Flavor names
        public const string FipsFlavor = "fips";
        public const string BaseFlavor = "base";
    }
}
