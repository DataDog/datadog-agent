using System;

namespace WixSetup.Datadog_Agent
{
    internal static class AgentFlavorFactory
    {
        public static IAgentFlavor New(AgentVersion agentVersion)
        {
            var flavor = Environment.GetEnvironmentVariable("AGENT_FLAVOR");

            return flavor switch
            {
                "fips" => new FIPSAgent(agentVersion),
                "base" => new BaseAgent(agentVersion),
                _ => new BaseAgent(agentVersion) // Default to BaseAgent if no valid value is provided
            };
        }
    }

    internal interface IAgentFlavor
    {
        string ProductFullName { get; }
        Guid UpgradeCode { get; }
        string ProductDescription { get; }
        string PackageOutFileName { get; }
    }

    internal class FIPSAgent : IAgentFlavor
    {
        private readonly AgentVersion _agentVersion;

        public FIPSAgent(AgentVersion agentVersion)
        {
            _agentVersion = agentVersion;
        }

        public string ProductFullName => "Datadog FIPS Agent";
        public Guid UpgradeCode => new("de421174-9615-4fe9-b8a8-2b3f123bdc4f");
        public string ProductDescription => $"Datadog FIPS Agent {_agentVersion.PackageVersion}";
        public string PackageOutFileName => $"datadog-fips-agent-{_agentVersion.PackageVersion}-1-x86_64";
    }

    internal class BaseAgent : IAgentFlavor
    {
        private readonly AgentVersion _agentVersion;

        public BaseAgent(AgentVersion agentVersion)
        {
            _agentVersion = agentVersion;
        }

        public string ProductFullName => "Datadog Agent";
        public Guid UpgradeCode => new("0c50421b-aefb-4f15-a809-7af256d608a5");
        public string ProductDescription => $"Datadog Agent {_agentVersion.PackageVersion}";
        public string PackageOutFileName => $"datadog-agent-{_agentVersion.PackageVersion}-1-x86_64";
    }
}
