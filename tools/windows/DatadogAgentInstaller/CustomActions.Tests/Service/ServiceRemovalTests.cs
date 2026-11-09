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
            NativeMethods.Setup(n => n.FetchSecret(Datadog.CustomActions.ConfigureUserCustomActions.AgentPasswordPrivateDataKey())).Returns(Password);
            NativeMethods.Setup(n => n.FetchSecret(AgentCredentials.AccountSidKey)).Returns(AccountSid.Value);
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
        public void RollbackReadsCurrentPasswordWithoutSavingOrResettingIt()
        {
            Assert.Equal(ActionResult.Success, Create().PrepareServiceRemoval());
            Assert.Equal("1", Session.Object["CanDeleteServices"]);
            Assert.Equal(Account, Session.Object["ServiceRemovalAccount"]);
            Assert.Equal(AccountSid.Value, Session.Object["ServiceRemovalSid"]);
            Assert.Equal("datadogagent,datadog-trace-agent", Session.Object["ServiceRemovalNames"]);
            // Normal installation may have rotated the same account's password.
            const string currentPassword = "new;password";
            NativeMethods.Setup(n => n.FetchSecret(Datadog.CustomActions.ConfigureUserCustomActions.AgentPasswordPrivateDataKey())).Returns(currentPassword);
            Assert.Equal(ActionResult.Success, Create().RestoreServiceRemovalCredentials());
            NativeMethods.Verify(n => n.SetUserPassword(It.IsAny<string>(), It.IsAny<string>()), Times.Never);
            NativeMethods.Verify(n => n.StoreSecret(It.IsAny<string>(), It.IsAny<string>()), Times.Never);
            ServiceController.Verify(c => c.SetCredentials(Constants.AgentServiceName, Account, currentPassword), Times.Once);
            ServiceController.Verify(c => c.SetCredentials(Constants.TraceAgentServiceName, Account, currentPassword), Times.Once);
            Session.Verify(s => s.Log(
                It.Is<string>(message => message.Contains(currentPassword)),
                It.IsAny<string>(), It.IsAny<string>(), It.IsAny<int>()), Times.Never);
            Session.VerifySet(s => s[It.IsAny<string>()] = It.Is<string>(value => value == Password || value == currentPassword), Times.Never);
        }

        [Theory]
        [InlineData(false, null)]
        [InlineData(false, "")]
        [InlineData(true, null)]
        [InlineData(true, "")]
        public void MissingPasswordPreservesLocalAndDomainServices(bool domain, string password)
        {
            NativeMethods.Setup(n => n.IsDomainAccount(AccountSid)).Returns(domain);
            NativeMethods.Setup(n => n.FetchSecret(Datadog.CustomActions.ConfigureUserCustomActions.AgentPasswordPrivateDataKey())).Returns(password);
            Assert.Equal(ActionResult.Success, Create().PrepareServiceRemoval());
            Assert.Equal("", Session.Object["CanDeleteServices"]);
        }

        [Theory]
        [InlineData(null)]
        [InlineData("S-1-5-21-1-2-3-1002")]
        public void UnassociatedPasswordPreservesServices(string sid)
        {
            NativeMethods.Setup(n => n.FetchSecret(AgentCredentials.AccountSidKey)).Returns(sid);
            Create().PrepareServiceRemoval();
            Assert.Equal("", Session.Object["CanDeleteServices"]);
        }

        [Theory]
        [InlineData(null)] // switched to a passwordless account, or secret was removed
        [InlineData("S-1-5-21-1-2-3-1002")] // switched to another user
        public void RollbackDoesNotUseAnotherAccountsPassword(string sid)
        {
            Create().PrepareServiceRemoval();
            NativeMethods.Setup(n => n.FetchSecret(AgentCredentials.AccountSidKey)).Returns(sid);
            Assert.Equal(ActionResult.Success, Create().RestoreServiceRemovalCredentials());
            ServiceController.Verify(c => c.SetCredentials(It.IsAny<string>(), It.IsAny<string>(), It.IsAny<string>()), Times.Never);
            Session.Verify(s => s.Log(
                It.Is<string>(message => message.Contains("repair may be required")),
                It.IsAny<string>(), It.IsAny<string>(), It.IsAny<int>()), Times.Once);
        }

        [Fact]
        public void RollbackToleratesMissingPassword()
        {
            Create().PrepareServiceRemoval();
            NativeMethods.Setup(n => n.FetchSecret(Datadog.CustomActions.ConfigureUserCustomActions.AgentPasswordPrivateDataKey())).Returns((string)null);
            Assert.Equal(ActionResult.Success, Create().RestoreServiceRemovalCredentials());
            ServiceController.Verify(c => c.SetCredentials(It.IsAny<string>(), It.IsAny<string>(), It.IsAny<string>()), Times.Never);
        }

        [Fact]
        public void PasswordUpdateInvalidatesAssociationUntilBothWritesSucceed()
        {
            var order = new MockSequence();
            var native = new Mock<INativeMethods>(MockBehavior.Strict);
            native.InSequence(order).Setup(n => n.StoreSecret(AgentCredentials.AccountSidKey, "unassociated"));
            native.InSequence(order).Setup(n => n.StoreSecret(Datadog.CustomActions.ConfigureUserCustomActions.AgentPasswordPrivateDataKey(), Password));
            native.InSequence(order).Setup(n => n.StoreSecret(AgentCredentials.AccountSidKey, AccountSid.Value));
            AgentCredentials.StorePassword(native.Object, AccountSid, Password);
            native.VerifyAll();
        }

        [Fact]
        public void FailedPasswordWriteDoesNotAssociateCredentialsWithNewAccount()
        {
            NativeMethods.Setup(n => n.StoreSecret(Datadog.CustomActions.ConfigureUserCustomActions.AgentPasswordPrivateDataKey(), Password)).Throws<Exception>();
            Assert.Throws<Exception>(() => AgentCredentials.StorePassword(NativeMethods.Object, AccountSid, Password));
            NativeMethods.Verify(n => n.StoreSecret(AgentCredentials.AccountSidKey, "unassociated"), Times.Once);
            NativeMethods.Verify(n => n.StoreSecret(AgentCredentials.AccountSidKey, AccountSid.Value), Times.Never);
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
        public void DomainPasswordIsNeverResetDuringRollback()
        {
            NativeMethods.Setup(n => n.IsDomainAccount(AccountSid)).Returns(true);
            Create().PrepareServiceRemoval();
            Create().RestoreServiceRemovalCredentials();
            NativeMethods.Verify(n => n.SetUserPassword(It.IsAny<string>(), It.IsAny<string>()), Times.Never);
            ServiceController.Verify(c => c.SetCredentials(Constants.AgentServiceName, Account, Password), Times.Once);
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
