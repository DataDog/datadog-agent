using Datadog.CustomActions;
using FluentAssertions;
using Moq;
using Xunit;

namespace CustomActions.Tests.ConfigureUserCustomActions
{
    public class ConfigureUserPasswordMigrationTests
    {
        public ConfigureUserCustomActionsTestSetup Test { get; } = new();

        [Fact]
        public void ScmServicePasswordSecretName_UsesScmPrefix()
        {
            ConfigureUserCustomActions.ScmServicePasswordSecretName(Constants.AgentServiceName)
                .Should()
                .Be("_SC_datadogagent");
        }

        [Fact]
        public void MigrateAgentPasswordFromScmIfNeeded_Stores_Scm_Password_When_Agent_Secret_Missing()
        {
            var agentKey = ConfigureUserCustomActions.AgentPasswordPrivateDataKey();
            var scmKey = ConfigureUserCustomActions.ScmServicePasswordSecretName(Constants.AgentServiceName);

            Test.ServiceController
                .Setup(controller => controller.ServiceExists(Constants.AgentServiceName))
                .Returns(true);
            Test.NativeMethods
                .Setup(nativeMethods => nativeMethods.FetchSecret(agentKey))
                .Throws(new System.Exception("not found"));
            Test.NativeMethods
                .Setup(nativeMethods => nativeMethods.FetchSecret(scmKey))
                .Returns("domain-password");

            Test.Create().MigrateAgentPasswordFromScmIfNeeded(agentKey);

            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.StoreSecret(agentKey, "domain-password"),
                Times.Once);
        }

        [Fact]
        public void MigrateAgentPasswordFromScmIfNeeded_Skips_When_Agent_Secret_Exists()
        {
            var agentKey = ConfigureUserCustomActions.AgentPasswordPrivateDataKey();

            Test.ServiceController
                .Setup(controller => controller.ServiceExists(Constants.AgentServiceName))
                .Returns(true);
            Test.NativeMethods
                .Setup(nativeMethods => nativeMethods.FetchSecret(agentKey))
                .Returns("existing-password");

            Test.Create().MigrateAgentPasswordFromScmIfNeeded(agentKey);

            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.StoreSecret(It.IsAny<string>(), It.IsAny<string>()),
                Times.Never);
        }

        [Fact]
        public void MigrateAgentPasswordFromScmIfNeeded_Skips_For_Service_Accounts()
        {
            Test.Session.Object["DDAGENTUSER_IS_SERVICE_ACCOUNT"] = "true";

            Test.Create().MigrateAgentPasswordFromScmIfNeeded(
                ConfigureUserCustomActions.AgentPasswordPrivateDataKey());

            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.FetchSecret(It.IsAny<string>()),
                Times.Never);
        }
    }
}
