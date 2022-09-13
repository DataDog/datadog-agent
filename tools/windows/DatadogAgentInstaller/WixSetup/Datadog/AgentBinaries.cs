namespace WixSetup.Datadog
{
    public class AgentBinaries
    {
        private readonly string _binSource;
        public string Agent => $@"{_binSource}\agent\agent.exe";
        public string Tray => $@"{_binSource}\agent\ddtray.exe";
        public string ProcessAgent => $@"{_binSource}\agent\process-agent.exe";
        public string SecurityAgent => $@"{_binSource}\agent\security-agent.exe";
        public string SystemProbe => $@"{_binSource}\agent\system-probe.exe";
        public string TraceAgent => $@"{_binSource}\agent\trace-agent.exe";
        public string LibDatadogAgentThree => $@"{_binSource}\agent\libdatadog-agent-three.dll";
        public string LibDatadogAgentTwo => $@"{_binSource}\agent\libdatadog-agent-two.dll";

        public AgentBinaries(string binSource)
        {
            _binSource = binSource;
        }
    }
}
