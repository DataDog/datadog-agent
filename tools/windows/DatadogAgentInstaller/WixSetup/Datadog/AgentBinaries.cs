using WixSharp;

namespace WixSetup.Datadog
{
    public class AgentBinaries
    {
        private readonly string _binSource;
        public string Agent => $@"{_binSource}\agent\agent.exe";
        public string Tray => $@"{_binSource}\agent\ddtray.exe";
        public Id TrayId => new ("ddtray");
        public string ProcessAgent => $@"{_binSource}\agent\process-agent.exe";
        public string SystemProbe => $@"{_binSource}\agent\system-probe.exe";
        public string TraceAgent => $@"{_binSource}\agent\trace-agent.exe";
        // this will only be actually used when the procmon driver is present
        // if (!string.IsNullOrEmpty(Environment.GetEnvironmentVariable("WINDOWS_DDPROCMON_DRIVER")))
        public string SecurityAgent => $@"{_binSource}\agent\security-agent.exe";
        public string LibDatadogAgentThree => $@"{_binSource}\agent\libdatadog-agent-three.dll";

        public string[] PythonThreeBinaries;
        public string[] PythonTwoBinaries;

        public string LibDatadogAgentTwo => $@"{_binSource}\agent\libdatadog-agent-two.dll";

        public AgentBinaries(string binSource, string installerSource)
        {
            _binSource = binSource;
            PythonThreeBinaries = new[]
            {
                $@"{installerSource}\embedded3\python.exe",
                $@"{installerSource}\embedded3\python3.dll",
                $@"{installerSource}\embedded3\python39.dll",
                $@"{installerSource}\embedded3\pythonw.exe"
            };
            PythonTwoBinaries = new[]
            {
                $@"{installerSource}\embedded2\python.exe",
                $@"{installerSource}\embedded2\python27.dll",
                $@"{installerSource}\embedded2\pythonw.exe"
            };
        }
    }
}
