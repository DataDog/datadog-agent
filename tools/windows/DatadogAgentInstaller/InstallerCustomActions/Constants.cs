// ReSharper disable InconsistentNaming
namespace Datadog.InstallerCustomActions
{
    public class Constants
    {
        public const string AgentServiceName = "datadogagent";
        public const string TraceAgentServiceName = "datadog-trace-agent";
        public const string ProcessAgentServiceName = "datadog-process-agent";
        public const string SystemProbeServiceName = "datadog-system-probe";
        public const string SecurityAgentServiceName = "datadog-security-agent";
        public const string NpmServiceName = "ddnpm";
        public const string ProcmonServiceName = "ddprocmon";

        // Key under HKLM that contains our options
        public const string DatadogInstallerRegistryKey = @"Software\Datadog\Datadog Installer";
    }
}
