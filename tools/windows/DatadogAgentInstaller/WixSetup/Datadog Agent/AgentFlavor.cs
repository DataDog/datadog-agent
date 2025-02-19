using Datadog.CustomActions;
using System;

namespace WixSetup.Datadog_Agent
{
    internal static class AgentFlavorFactory
    {
        private const string FipsFlavor = "fips";
        private const string BaseFlavor = "base";

        public static string[] GetAllAgentFlavors()
        {
            return new[]
            {
                BaseFlavor,
                FipsFlavor
            };
        }

        public static IAgentFlavor New(AgentVersion agentVersion)
        {
            var flavor = Environment.GetEnvironmentVariable("AGENT_FLAVOR");
            return New(flavor, agentVersion);
        }

        public static IAgentFlavor New(string flavor, AgentVersion agentVersion)
        {
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
        string FlavorName { get; }
        string ProductFullName { get; }
        Guid UpgradeCode { get; }
        string ProductDescription { get; }
        string PackageOutFileName { get; }
        // https://github.com/openssl/openssl/blob/master/NOTES-WINDOWS.md#installation-directories
        string OpenSSLWinCtx { get; }
    }

    internal class FIPSAgent : IAgentFlavor
    {
        private readonly AgentVersion _agentVersion;
        private readonly string _agentNameSuffix;

        public FIPSAgent(AgentVersion agentVersion)
        {
            _agentVersion = agentVersion;
            _agentNameSuffix = Environment.GetEnvironmentVariable("AGENT_PRODUCT_NAME_SUFFIX");
        }

        public string FlavorName => Constants.FipsFlavor;
        public string ProductFullName => "Datadog FIPS Agent";
        public Guid UpgradeCode => new("de421174-9615-4fe9-b8a8-2b3f123bdc4f");
        public string ProductDescription => $"Datadog FIPS Agent {_agentVersion.PackageVersion}";
        public string PackageOutFileName => $"datadog-fips-agent-{_agentNameSuffix}{_agentVersion.PackageVersion}-1-x86_64";
        public string OpenSSLWinCtx => "datadog-fips-agent";
    }

    internal class BaseAgent : IAgentFlavor
    {
        private readonly AgentVersion _agentVersion;
        private readonly string _agentNameSuffix;

        public BaseAgent(AgentVersion agentVersion)
        {
            _agentVersion = agentVersion;
            _agentNameSuffix = Environment.GetEnvironmentVariable("AGENT_PRODUCT_NAME_SUFFIX");
        }

        public string FlavorName => Constants.BaseFlavor;
        public string ProductFullName => "Datadog Agent";
        public Guid UpgradeCode => new("0c50421b-aefb-4f15-a809-7af256d608a5");
        public string ProductDescription => $"Datadog Agent {_agentVersion.PackageVersion}";
        public string PackageOutFileName => $"datadog-agent-{_agentNameSuffix}{_agentVersion.PackageVersion}-1-x86_64";
        public string OpenSSLWinCtx => "datadog-agent";
    }
}
