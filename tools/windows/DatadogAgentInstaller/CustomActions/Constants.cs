// ReSharper disable InconsistentNaming
namespace Datadog.CustomActions
{
    public class Constants
    {
        // Main agent service - now runs dd-procmgrd.exe (process manager)
        // which manages all sub-agents via YAML configuration
        public const string AgentServiceName = "datadogagent";
        
        // Legacy service names - these are no longer registered as Windows Services
        // Sub-agents are now managed by the process manager via YAML configs
        // Keeping these constants for potential upgrade/migration scenarios
        public const string TraceAgentServiceName = "datadog-trace-agent";
        public const string ProcessAgentServiceName = "datadog-process-agent";
        public const string SystemProbeServiceName = "datadog-system-probe";
        public const string SecurityAgentServiceName = "datadog-security-agent";
        
        public const string InstallerServiceName = "Datadog Installer";
        public const string NpmServiceName = "ddnpm";
        public const string ProcmonServiceName = "ddprocmon";

        // Key under HKLM that contains our options
        public const string DatadogAgentRegistryKey = @"Software\Datadog\Datadog Agent";

        // Flavor names
        public const string FipsFlavor = "fips";
        public const string BaseFlavor = "base";
    }
}
