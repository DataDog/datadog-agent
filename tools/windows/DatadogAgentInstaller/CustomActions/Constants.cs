// ReSharper disable InconsistentNaming
namespace Datadog.CustomActions
{
    public class Constants
    {
        public const string AgentServiceName = "datadogagent";
        public const string TraceAgentServiceName = "datadog-trace-agent";
        public const string ProcessAgentServiceName = "datadog-process-agent";
        public const string SystemProbeServiceName = "datadog-system-probe";
        public const string NpmServiceName = "ddnpm";

        // Key under HKLM that contains our options
        public const string DatadogAgentRegistryKey = @"Software\Datadog\Datadog Agent";
    }
}
