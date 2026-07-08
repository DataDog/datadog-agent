using WixSharp;

namespace WixSetup.Datadog_Agent
{
    public class AgentBinaries
    {
        private readonly string _binSource;
        public string Agent => $@"{_binSource}\agent.exe";
        public string Tray => $@"{_binSource}\ddtray.exe";
        public Id TrayId => new("ddtray");
        public string ProcessAgent => $@"{_binSource}\process-agent.exe";
        public string PrivateActionRunner => $@"{_binSource}\privateactionrunner.exe";
        public string SystemProbe => $@"{_binSource}\system-probe.exe";
        public string TraceAgent => $@"{_binSource}\trace-agent.exe";
        public string SecretGenericConnector => $@"{_binSource}\secret-generic-connector.exe";
        // this will only be actually used when the procmon driver is present
        // if (!string.IsNullOrEmpty(Environment.GetEnvironmentVariable("WINDOWS_DDPROCMON_DRIVER")))
        public string SecurityAgent => $@"{_binSource}\security-agent.exe";
        public string LibDatadogAgentThree => $@"{_binSource}\libdatadog-agent-three.dll";
        public string DatadogInterop => $@"{_binSource}\libdatadog-interop.dll";
        public string DdCompilePolicy => $@"{_binSource}\dd-compile-policy.exe";
        public string ProcmgrService => $@"{_binSource}\dd-procmgrd.exe";
        public string Procmgr => $@"{_binSource}\dd-procmgr.exe";
        public string AgentDataPlane => $@"{_binSource}\agent-data-plane.exe";

        // Note: the AI Usage Chrome native messaging host is no longer shipped by the MSI.
        // It is delivered as the "ai-usage" fleet installer extension, gated on EUDM
        // (see pkg/fleet/installer/packages/datadog_agent_ai_usage_windows.go).

        public AgentBinaries(string binSource, string installerSource)
        {
            _binSource = binSource;
        }
    }
}
