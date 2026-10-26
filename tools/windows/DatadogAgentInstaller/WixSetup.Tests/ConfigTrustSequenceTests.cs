using FluentAssertions;
using WixSharp;
using WixSetup.Datadog_Agent;
using Xunit;

namespace WixSetup.Tests
{
    public class ConfigTrustSequenceTests
    {
        [Fact]
        public void Execution_Trust_Check_Is_Unconditional_And_Precedes_The_Transaction()
        {
            var actions = new AgentCustomActions();

            actions.EnsureSecureConfigRoot.Sequence.Should().Be(Sequence.InstallExecuteSequence);
            actions.EnsureSecureConfigRoot.Return.Should().Be(Return.check);
            actions.EnsureSecureConfigRoot.Execute.Should().Be(Execute.immediate);
            actions.EnsureSecureConfigRoot.Condition.ToString().Should().Be(Condition.Always.ToString());
            actions.EnsureSecureConfigRoot.When.Should().Be(When.After);
            actions.EnsureSecureConfigRoot.Step.Should().Be(Step.InstallValidate);
            actions.ReadConfig.Sequence.ToString().Should().Be(
                (Sequence.InstallExecuteSequence | Sequence.InstallUISequence).ToString());
            actions.ReadConfig.Execute.Should().Be(Execute.firstSequence);
            actions.ReadConfig.When.Should().Be(When.After);
            actions.ReadConfig.Step.Should().Be(Step.CostFinalize);
        }
    }
}
