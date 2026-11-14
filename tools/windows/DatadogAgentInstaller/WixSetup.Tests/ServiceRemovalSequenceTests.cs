using FluentAssertions;
using WixSharp;
using WixSetup.Datadog_Agent;
using Xunit;

namespace WixSetup.Tests
{
    public class ServiceRemovalSequenceTests
    {
        [Fact]
        public void RemovalCredentialsPrecedeDeletionAndPermissionCleanupFollowsIt()
        {
            var actions = new AgentCustomActions();
            actions.PrepareServiceRemoval.When.Should().Be(When.Before);
            actions.PrepareServiceRemoval.Step.Should().Be(Step.InstallInitialize);
            actions.PrepareServiceRemoval.Condition.ToString().Should().Be(Conditions.RemovingForUpgrade.ToString());
            actions.RestoreServiceRemovalCredentials.When.Should().Be(When.Before);
            actions.RestoreServiceRemovalCredentials.Step.Should().Be(Step.DeleteServices);
            actions.RestoreServiceRemovalCredentials.Execute.Should().Be(Execute.rollback);
            actions.RestoreServiceRemovalCredentials.UsesProperties.Should().NotContain("Password");
            actions.RestoreServiceRemovalCredentials.UsesProperties.Should().NotContain("ServiceRemovalNames");
            actions.UninstallUser.When.Should().Be(When.After);
            actions.UninstallUser.Step.Should().Be(Step.DeleteServices);
            actions.UninstallUserRollback.Step.ToString().Should().Be(actions.UninstallUser.Id.ToString());
        }
    }
}
