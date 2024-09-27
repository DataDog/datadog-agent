using System;
using System.Linq;

namespace WixSetup.Datadog_Agent
{
    public class AgentPython
    {
        public string[] Runtimes { get; }

        public bool IncludePython2 { get; }

        public AgentPython()
        {
            Runtimes = new[] { "3" };
            var pyRuntimesEnv = Environment.GetEnvironmentVariable("PY_RUNTIMES");
            Console.WriteLine($"Detected Python runtimes: {pyRuntimesEnv}");
            if (pyRuntimesEnv != null)
            {
                Runtimes = pyRuntimesEnv.Split(',');
                if (Runtimes.Any(runtime => runtime.Trim() == "2"))
                {
                    Console.WriteLine("-> Including Python 2 runtime");
                    IncludePython2 = true;
                }
            }
        }
    }
}
