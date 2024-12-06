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
        public string SystemProbe => $@"{_binSource}\system-probe.exe";
        public string TraceAgent => $@"{_binSource}\trace-agent.exe";
        // this will only be actually used when the procmon driver is present
        // if (!string.IsNullOrEmpty(Environment.GetEnvironmentVariable("WINDOWS_DDPROCMON_DRIVER")))
        public string SecurityAgent => $@"{_binSource}\security-agent.exe";
        public string LibDatadogAgentThree => $@"{_binSource}\libdatadog-agent-three.dll";

        public AgentBinaries(string binSource, string installerSource)
        {
            _binSource = binSource;
        }
    }
}
