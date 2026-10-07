using WixSharp;
using Xunit;
using WixSetup.Datadog_Agent;

namespace WixSetup.Tests
{
    public class ParConfigMigrationSequenceTests
    {
        [Fact]
        public void CaptureIsElevatedAndRollbackPrecedesIt()
        {
            var actions = new AgentCustomActions();
            Assert.Equal(Execute.deferred, actions.CapturePARConfig.Execute);
            Assert.False(actions.CapturePARConfig.Impersonate);
            Assert.Equal(Step.InstallInitialize.ToString(), actions.CapturePARConfig.Step.ToString());
            Assert.Equal(When.After, actions.CapturePARConfig.When);
            Assert.Equal(Execute.rollback, actions.RollbackPARConfig.Execute);
            Assert.Equal(actions.CapturePARConfig.Id.ToString(), actions.RollbackPARConfig.Step.ToString());
            Assert.Equal(When.Before, actions.RollbackPARConfig.When);
        }

        [Fact]
        public void RestoreFollowsAccountSetupAndFolderSetupFollowsRemoval()
        {
            var actions = new AgentCustomActions();
            Assert.Equal(actions.ConfigureUser.Id.ToString(), actions.RestorePARConfig.Step.ToString());
            Assert.Equal(When.After, actions.RestorePARConfig.When);
            Assert.Equal(Return.check, actions.RestorePARConfig.Return);
            Assert.Equal(Step.RemoveExistingProducts.ToString(), actions.DDCreateFolders.Step.ToString());
            Assert.Equal(Execute.commit, actions.CleanupPARConfig.Execute);
        }
    }
}
