using System;
using System.Security.Principal;
using CustomActions.Tests.ProcessUserCustomActions;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Moq;
using WixToolset.Dtf.WindowsInstaller;
using Xunit;

namespace CustomActions.Tests.Service
{
    public class ServiceRemovalTests : ServiceCustomActionsTestSetup
    {
        private const string Account = "TEST\\ddagentuser";
        private const string Password = "original-password";
        private static readonly SecurityIdentifier AccountSid = new("S-1-5-21-1-2-3-1001");

        public ServiceRemovalTests()
        {
            var installed = new Mock<IRegistryKey>();
            installed.Setup(k => k.GetValue("installedDomain")).Returns("TEST");
            installed.Setup(k => k.GetValue("installedUser")).Returns("ddagentuser");
            RegistryServices.Setup(r => r.OpenRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey))
                .Returns(installed.Object);
            GivenAccount(Account, AccountSid);
            GivenService(Constants.AgentServiceName, Account);
            GivenService(Constants.TraceAgentServiceName, Account);
            GivenService(Constants.ProcmgrServiceName, "LocalSystem");
            NativeMethods.Setup(n => n.FetchSecret(ConfigureUserCustomActions.AgentPasswordPrivateDataKey())).Returns(Password);
        }

        private void GivenAccount(string account, SecurityIdentifier accountSid)
        {
            NativeMethods.Setup(n => n.LookupAccountName(account,
                    out It.Ref<string>.IsAny, out It.Ref<string>.IsAny,
                    out It.Ref<SecurityIdentifier>.IsAny, out It.Ref<SID_NAME_USE>.IsAny))
                .Callback(new LookupAccountNameDelegate((string _, out string user, out string domain,
                    out SecurityIdentifier sid, out SID_NAME_USE nameUse) =>
                {
                    user = "ddagentuser";
                    domain = "TEST";
                    sid = accountSid;
                    nameUse = SID_NAME_USE.SidTypeUser;
                })).Returns(true);
        }

        private void GivenService(string name, string account)
        {
            var service = new Mock<IRegistryKey>();
            service.Setup(k => k.GetValue("ObjectName")).Returns(account);
            RegistryServices.Setup(r => r.OpenRegistryKey(Registries.LocalMachine, $@"SYSTEM\CurrentControlSet\Services\{name}"))
                .Returns(service.Object);
        }

        [Fact]
        public void StoredCredentialsAllowDeletionAndAreCapturedForRollback()
        {
            Session.Object["DDAGENTUSER_PASSWORD"] = "incoming-password-for-another-account";
            Assert.Equal(ActionResult.Success, Create().PrepareServiceRemoval());
            Assert.Equal("1", Session.Object["CanDeleteServices"]);
            Assert.Equal(Account, Session.Object["ServiceRemovalAccount"]);
            Assert.Equal(Password, Session.Object["ServiceRemovalPassword"]);
            Assert.Equal("datadogagent,datadog-trace-agent", Session.Object["ServiceRemovalNames"]);
            // The incoming install can overwrite LSA; rollback must use the captured value.
            NativeMethods.Setup(n => n.FetchSecret(It.IsAny<string>())).Throws<Exception>();
            Assert.Equal(ActionResult.Success, Create().RestoreServiceRemovalCredentials());
            ServiceController.Verify(c => c.SetCredentials(Constants.AgentServiceName, Account, Password), Times.Once);
            ServiceController.Verify(c => c.SetCredentials(Constants.TraceAgentServiceName, Account, Password), Times.Once);
            ServiceController.Verify(c => c.SetCredentials(Constants.ProcmgrServiceName, It.IsAny<string>(), It.IsAny<string>()), Times.Never);
            Session.Verify(s => s.Log(It.Is<string>(message => message.Contains(Password))), Times.Never);
        }

        [Theory]
        [InlineData(null)]
        [InlineData("")]
        [InlineData("unsupported;password")]
        public void UnavailableOrUnencodablePasswordPreservesServices(string password)
        {
            Session.Object["CanDeleteServices"] = "1";
            NativeMethods.Setup(n => n.FetchSecret(It.IsAny<string>())).Returns(password);
            Assert.Equal(ActionResult.Success, Create().PrepareServiceRemoval());
            Assert.Equal("", Session.Object["CanDeleteServices"]);
            Assert.Equal("", Session.Object["ServiceRemovalPassword"]);
        }

        [Fact]
        public void SecretLookupFailurePreservesServices()
        {
            NativeMethods.Setup(n => n.FetchSecret(It.IsAny<string>())).Throws<UnauthorizedAccessException>();
            Assert.Equal(ActionResult.Success, Create().PrepareServiceRemoval());
            Assert.Equal("", Session.Object["CanDeleteServices"]);
        }

        [Fact]
        public void ManagedAccountDoesNotRequirePassword()
        {
            NativeMethods.Setup(n => n.IsServiceAccount(AccountSid)).Returns(true);
            Assert.Equal(ActionResult.Success, Create().PrepareServiceRemoval());
            Assert.Equal("1", Session.Object["CanDeleteServices"]);
            NativeMethods.Verify(n => n.FetchSecret(It.IsAny<string>()), Times.Never);
            Create().RestoreServiceRemovalCredentials();
            ServiceController.Verify(c => c.SetCredentials(Constants.AgentServiceName, Account, null), Times.Once);
        }

        [Fact]
        public void BuiltInServicesDoNotRequirePassword()
        {
            GivenService(Constants.AgentServiceName, "LocalSystem");
            GivenService(Constants.TraceAgentServiceName, "LocalSystem");
            Assert.Equal(ActionResult.Success, Create().PrepareServiceRemoval());
            Assert.Equal("1", Session.Object["CanDeleteServices"]);
            Assert.Equal("", Session.Object["ServiceRemovalNames"]);
            NativeMethods.Verify(n => n.FetchSecret(It.IsAny<string>()), Times.Never);
        }

        [Fact]
        public void DifferentServiceAccountPreservesAllServices()
        {
            GivenAccount("TEST\\other", new SecurityIdentifier("S-1-5-21-1-2-3-1002"));
            GivenService(Constants.TraceAgentServiceName, "TEST\\other");
            Assert.Equal(ActionResult.Success, Create().PrepareServiceRemoval());
            Assert.Equal("", Session.Object["CanDeleteServices"]);
        }

        [Fact]
        public void MissingInstallationMetadataPreservesServices()
        {
            RegistryServices.Setup(r => r.OpenRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey))
                .Returns((IRegistryKey)null);
            Assert.Equal(ActionResult.Success, Create().PrepareServiceRemoval());
            Assert.Equal("", Session.Object["CanDeleteServices"]);
        }

        [Fact]
        public void RollbackContinuesAfterOneServiceFails()
        {
            Create().PrepareServiceRemoval();
            ServiceController.Setup(c => c.SetCredentials(Constants.AgentServiceName, Account, Password)).Throws<Exception>();
            Assert.Equal(ActionResult.Success, Create().RestoreServiceRemovalCredentials());
            ServiceController.Verify(c => c.SetCredentials(Constants.TraceAgentServiceName, Account, Password), Times.Once);
        }
    }
}
