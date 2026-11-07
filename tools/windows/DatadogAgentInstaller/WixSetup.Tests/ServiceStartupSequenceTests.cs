using FluentAssertions;
using WixSharp;
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
            // Both must run in the outgoing product, including upgrade removal.
            actions.StopDDServicesRollback.Condition.ToString().Should().Be(actions.StopDDServices.Condition.ToString());
            actions.PrepareServiceStartupRollback.When.Should().Be(When.Before);
            actions.PrepareServiceStartupRollback.Step.Should().Be(Step.InstallInitialize);
            actions.PrepareServiceStartupRollback.Execute.Should().Be(Execute.immediate);
            actions.StartDDServices.When.Should().Be(When.After);
            actions.StartDDServices.Step.Should().Be(Step.StartServices);
            actions.DDCreateFolders.Step.Should().Be(Step.InstallInitialize);
            actions.RestoreDaclRollback.Step.Should().Be(Step.InstallInitialize);
        }
    }
}
