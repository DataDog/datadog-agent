using System;
using System.Linq;

namespace WixSetup.Datadog
{
    public class AgentPython
    {
        public string[] Runtimes { get; }

        public bool IncludePython2 { get; }

        public AgentPython()
        {
            Runtimes = new[] { "3" };
            var pyRuntimesEnv = Environment.GetEnvironmentVariable("PY_RUNTIMES");
            if (pyRuntimesEnv != null)
            {
                Runtimes = pyRuntimesEnv.Split();
                if (Runtimes.Any(runtime => runtime == "2"))
                {
                    IncludePython2 = true;
                }
            }
        }
    }
}
