using FluentAssertions;
using WixSharp;
using WixSetup;
using WixSetup.Datadog_Agent;
using Xunit;

namespace WixSetup.Tests
{
    public class ServiceStartupSequenceTests
    {
        [Fact]
        public void ServiceProtectionUsesTheExistingStopAndStartPositions()
        {
            var actions = new AgentCustomActions();
            actions.StopDDServices.When.Should().Be(When.Before);
            actions.StopDDServices.Step.Should().Be(Step.StopServices);
            actions.StopDDServices.Execute.Should().Be(Execute.deferred);
            actions.StopDDServicesRollback.Step.ToString().Should().Be(actions.StopDDServices.Id.ToString());
            actions.StopDDServicesRollback.Execute.Should().Be(Execute.rollback);
            // Stop remains unconditional, but only removal needs the enable rollback.
            actions.StopDDServices.Condition.ToString().Should().Be(Condition.Always.ToString());
            actions.StopDDServicesRollback.Condition.ToString().Should().Be(
                (Conditions.Uninstalling | Conditions.RemovingForUpgrade).ToString());
            actions.StartDDServices.When.Should().Be(When.After);
            actions.StartDDServices.Step.Should().Be(Step.StartServices);
            actions.DDCreateFolders.Step.Should().Be(Step.InstallInitialize);
            actions.RestoreDaclRollback.Step.Should().Be(Step.InstallInitialize);
        }
    }
}
