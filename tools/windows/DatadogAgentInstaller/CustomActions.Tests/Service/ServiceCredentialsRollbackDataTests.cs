using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Datadog.CustomActions.Rollback;
using FluentAssertions;
using Moq;
using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.IO;
using System.Security.Principal;
using Xunit;
using CustomActions.Tests.ProcessUserCustomActions;

namespace CustomActions.Tests.Service
{
    public class ServiceCredentialsRollbackDataTests
    {
        private const string ServiceName = "datadogagent";
        private const string DomainUser = "TESTDOMAIN\\ddagentuser";
        private const string OldPassword = "old-scm-password";

        private readonly Mock<ISession> _session = new();
        private readonly Mock<IServiceController> _serviceController = new();
        private readonly Mock<INativeMethods> _nativeMethods = new();
        private readonly Mock<IFileSystemServices> _fileSystemServices = new();

        private ServiceCredentialsRollbackData Capture(string serviceName = ServiceName)
        {
            return ServiceCredentialsRollbackData.Capture(
                serviceName, _serviceController.Object, _nativeMethods.Object, _session.Object);
        }

        private void Restore(ServiceCredentialsRollbackData rollback)
        {
            rollback.Restore(_session.Object, _fileSystemServices.Object, _serviceController.Object);
        }

        [Theory]
        [InlineData("LocalSystem")]
        [InlineData(@"NT AUTHORITY\SYSTEM")]
        [InlineData("LocalService")]
        [InlineData(@"NT AUTHORITY\LocalService")]
        [InlineData("NetworkService")]
        [InlineData(@"NT AUTHORITY\NetworkService")]
        public void Capture_RestoresWellKnownAccountWithEmptyPassword(string account)
        {
            _serviceController.Setup(c => c.GetServiceStartName(ServiceName)).Returns(account);

            var rollback = Capture();
            rollback.Should().NotBeNull();
            Restore(rollback);

            _serviceController.Verify(c => c.SetCredentials(ServiceName, account, ""), Times.Once);
            _nativeMethods.Verify(n => n.StoreSecret(It.IsAny<string>(), It.IsAny<string>()), Times.Never);
            _nativeMethods.Verify(n => n.FetchScmServicePassword(It.IsAny<string>()), Times.Never);
        }

        [Fact]
        public void Capture_StoresScmPasswordInLsa_AndRestoresIt()
        {
            _serviceController.Setup(c => c.GetServiceStartName(ServiceName)).Returns(DomainUser);
            _nativeMethods.Setup(n => n.FetchScmServicePassword(ServiceName)).Returns(OldPassword);
            _nativeMethods.Setup(n => n.FetchSecret(ServiceCredentialsRollbackData.SecretKey(ServiceName)))
                .Returns(OldPassword);

            var rollback = Capture();
            rollback.Should().NotBeNull();
            _nativeMethods.Verify(
                n => n.StoreSecret(ServiceCredentialsRollbackData.SecretKey(ServiceName), OldPassword),
                Times.Once);

            Restore(rollback);

            _serviceController.Verify(c => c.SetCredentials(ServiceName, DomainUser, OldPassword), Times.Once);
            _nativeMethods.Verify(
                n => n.RemoveSecret(ServiceCredentialsRollbackData.SecretKey(ServiceName)),
                Times.Once);
        }

        [Fact]
        public void Capture_ReturnsNull_WhenScmPasswordUnavailable()
        {
            _serviceController.Setup(c => c.GetServiceStartName(ServiceName)).Returns(DomainUser);
            _nativeMethods.Setup(n => n.FetchScmServicePassword(ServiceName)).Returns((string)null);

            Capture().Should().BeNull();
        }

        [Fact]
        public void Capture_ReturnsNull_WhenScmPasswordFetchFails()
        {
            _serviceController.Setup(c => c.GetServiceStartName(ServiceName)).Returns(DomainUser);
            _nativeMethods.Setup(n => n.FetchScmServicePassword(ServiceName))
                .Throws(new Win32Exception(5));

            Capture().Should().BeNull();
        }

        [Fact]
        public void Capture_ReturnsNull_WhenAccountNameMissing()
        {
            _serviceController.Setup(c => c.GetServiceStartName(ServiceName)).Returns((string)null);

            Capture().Should().BeNull();
        }

        [Fact]
        public void Capture_ReturnsNull_WhenStoreSecretFails()
        {
            _serviceController.Setup(c => c.GetServiceStartName(ServiceName)).Returns(DomainUser);
            _nativeMethods.Setup(n => n.FetchScmServicePassword(ServiceName)).Returns(OldPassword);
            _nativeMethods.Setup(n => n.StoreSecret(It.IsAny<string>(), It.IsAny<string>()))
                .Throws(new Win32Exception(5));

            Capture().Should().BeNull();
        }

        [Fact]
        public void Restore_LeavesCredentialsUnchanged_WhenRollbackSecretMissing()
        {
            _serviceController.Setup(c => c.GetServiceStartName(ServiceName)).Returns(DomainUser);
            _nativeMethods.Setup(n => n.FetchScmServicePassword(ServiceName)).Returns(OldPassword);
            _nativeMethods.Setup(n => n.FetchSecret(ServiceCredentialsRollbackData.SecretKey(ServiceName)))
                .Throws(new Win32Exception(2));

            var rollback = Capture();
            Restore(rollback);

            _serviceController.Verify(
                c => c.SetCredentials(It.IsAny<string>(), It.IsAny<string>(), It.IsAny<string>()),
                Times.Never);
            _nativeMethods.Verify(
                n => n.RemoveSecret(ServiceCredentialsRollbackData.SecretKey(ServiceName)),
                Times.Never);
        }
    }

    public class ServiceCredentialsRollbackConfigureTests
    {
        private const string DomainUserName = "TESTDOMAIN\\ddagentuser";
        private static readonly SecurityIdentifier DomainUserSid =
            new SecurityIdentifier("S-1-5-21-1-2-3-1001");

        public ServiceCustomActionsTestSetup Test { get; } = new();

        [Fact]
        public void ConfigureServiceUsers_SnapshotsAgentCredentials_WhenPasswordProvided()
        {
            const string oldPassword = "old-scm-password";
            Test.Session.Setup(s => s["DDAGENTUSER_PROCESSED_PASSWORD"]).Returns("new-password");
            Test.NativeMethods.Setup(n => n.IsServiceAccount(It.IsAny<SecurityIdentifier>())).Returns(false);
            Test.ServiceController.Setup(c => c.GetServiceStartName(Constants.AgentServiceName))
                .Returns(DomainUserName);
            Test.NativeMethods.Setup(n => n.FetchScmServicePassword(Constants.AgentServiceName))
                .Returns(oldPassword);

            Test.Create("ConfigureServicesCredentialsTest").ConfigureServiceUsers(DomainUserName, DomainUserSid);

            Test.NativeMethods.Verify(
                n => n.StoreSecret(
                    ServiceCredentialsRollbackData.SecretKey(Constants.AgentServiceName),
                    oldPassword),
                Times.Once);
            Test.ServiceController.Verify(
                c => c.SetCredentials(Constants.AgentServiceName, DomainUserName, "new-password"),
                Times.Once);
        }

        [Fact]
        public void ConfigureServiceUsers_DoesNotSnapshot_WhenRollbackStoreIsDisabled()
        {
            Test.Session.Setup(s => s["DDAGENTUSER_PROCESSED_PASSWORD"]).Returns("new-password");
            Test.NativeMethods.Setup(n => n.IsServiceAccount(It.IsAny<SecurityIdentifier>())).Returns(false);
            Test.ServiceController.Setup(c => c.GetServiceStartName(Constants.AgentServiceName))
                .Returns(DomainUserName);

            Test.Create().ConfigureServiceUsers(DomainUserName, DomainUserSid);

            Test.NativeMethods.Verify(n => n.FetchScmServicePassword(It.IsAny<string>()), Times.Never);
            Test.NativeMethods.Verify(n => n.StoreSecret(It.IsAny<string>(), It.IsAny<string>()), Times.Never);
        }

        [Fact]
        public void ConfigureServiceUsers_DoesNotSnapshotLocalSystemServices()
        {
            Test.Session.Setup(s => s["DDAGENTUSER_PROCESSED_PASSWORD"]).Returns("new-password");
            Test.NativeMethods.Setup(n => n.IsServiceAccount(It.IsAny<SecurityIdentifier>())).Returns(false);
            Test.ServiceController.Setup(c => c.GetServiceStartName(Constants.AgentServiceName))
                .Returns(DomainUserName);
            Test.NativeMethods.Setup(n => n.FetchScmServicePassword(Constants.AgentServiceName))
                .Returns("old-password");

            Test.Create("ConfigureServicesCredentialsTest").ConfigureServiceUsers(DomainUserName, DomainUserSid);

            Test.NativeMethods.Verify(
                n => n.FetchScmServicePassword(Constants.ProcmgrServiceName),
                Times.Never);
        }

        [Fact]
        public void ConfigureServices_DiscardsStaleCredentialRollbackSecretsBeforeLookup()
        {
            var removeSecretCalls = new List<string>();
            var lookupCalled = false;

            Test.Session.Setup(s => s["DDAGENTUSER_PROCESSED_FQ_NAME"]).Returns(DomainUserName);
            Test.NativeMethods
                .Setup(n => n.RemoveSecret(It.IsAny<string>()))
                .Callback<string>(key => removeSecretCalls.Add(key));
            Test.NativeMethods
                .Setup(n => n.LookupAccountName(
                    DomainUserName,
                    out It.Ref<string>.IsAny,
                    out It.Ref<string>.IsAny,
                    out It.Ref<SecurityIdentifier>.IsAny,
                    out It.Ref<SID_NAME_USE>.IsAny))
                .Callback(new LookupAccountNameDelegate((
                    string _,
                    out string user,
                    out string domain,
                    out SecurityIdentifier sid,
                    out SID_NAME_USE nameUse) =>
                {
                    lookupCalled = true;
                    user = "ddagentuser";
                    domain = "TESTDOMAIN";
                    sid = DomainUserSid;
                    nameUse = SID_NAME_USE.SidTypeUser;
                }))
                .Returns(false);

            Test.Create("ConfigureServicesStaleSecretsTest").ConfigureServices();

            lookupCalled.Should().BeTrue();
            removeSecretCalls.Should().BeEquivalentTo(new[]
            {
                ServiceCredentialsRollbackData.SecretKey(Constants.AgentServiceName),
                ServiceCredentialsRollbackData.SecretKey(Constants.TraceAgentServiceName),
                ServiceCredentialsRollbackData.SecretKey(Constants.PrivateActionRunnerServiceName),
                ServiceCredentialsRollbackData.SecretKey(Constants.SecurityAgentServiceName),
            });
        }

        [Fact]
        public void DiscardCredentialRollbackSecrets_RemovesSecretsForAgentUserServices()
        {
            var removeSecretCalls = new List<string>();
            Test.NativeMethods
                .Setup(n => n.RemoveSecret(It.IsAny<string>()))
                .Callback<string>(key => removeSecretCalls.Add(key));

            Test.Create().DiscardCredentialRollbackSecrets();

            removeSecretCalls.Should().BeEquivalentTo(new[]
            {
                ServiceCredentialsRollbackData.SecretKey(Constants.AgentServiceName),
                ServiceCredentialsRollbackData.SecretKey(Constants.TraceAgentServiceName),
                ServiceCredentialsRollbackData.SecretKey(Constants.PrivateActionRunnerServiceName),
                ServiceCredentialsRollbackData.SecretKey(Constants.SecurityAgentServiceName),
            });
        }
    }

    public class ServiceCredentialsRollbackStoreTests
    {
        private readonly Mock<ISession> _session = new();
        private readonly Mock<IServiceController> _serviceController = new();
        private readonly Mock<IFileSystemServices> _fileSystemServices = new();
        private readonly Mock<INativeMethods> _nativeMethods = new();

        [Fact]
        public void RollbackDataStore_RestoresWellKnownCredentials_AfterStoreAndLoad()
        {
            const string account = "LocalSystem";
            _serviceController.Setup(c => c.GetServiceStartName(Constants.AgentServiceName)).Returns(account);

            var storageName = $"credential-rollback-{Guid.NewGuid():N}";
            var store = new RollbackDataStore(
                _session.Object, storageName, _fileSystemServices.Object, _serviceController.Object);
            var snapshot = ServiceCredentialsRollbackData.Capture(
                Constants.AgentServiceName,
                _serviceController.Object,
                _nativeMethods.Object,
                _session.Object);
            snapshot.Should().NotBeNull();
            store.Add(snapshot);
            store.Store();

            _serviceController.Invocations.Clear();

            var restoreStore = new RollbackDataStore(
                _session.Object, storageName, _fileSystemServices.Object, _serviceController.Object);
            restoreStore.Restore();

            _serviceController.Verify(
                c => c.SetCredentials(Constants.AgentServiceName, account, ""), Times.Once);
        }

        [Fact]
        public void RollbackDataStore_PersistsDomainCredentialSnapshot_AfterStore()
        {
            const string domainUser = "TESTDOMAIN\\ddagentuser";
            const string oldPassword = "old-scm-password";
            _serviceController.Setup(c => c.GetServiceStartName(Constants.AgentServiceName)).Returns(domainUser);
            _nativeMethods.Setup(n => n.FetchScmServicePassword(Constants.AgentServiceName)).Returns(oldPassword);

            var storageName = $"credential-rollback-{Guid.NewGuid():N}";
            var store = new RollbackDataStore(
                _session.Object, storageName, _fileSystemServices.Object, _serviceController.Object);
            var snapshot = ServiceCredentialsRollbackData.Capture(
                Constants.AgentServiceName,
                _serviceController.Object,
                _nativeMethods.Object,
                _session.Object);
            snapshot.Should().NotBeNull();
            store.Add(snapshot);
            store.Store();

            var jsonPath = Path.Combine(
                Path.GetTempPath(),
                "datadog-installer",
                "rollback",
                $"{storageName}.json");
            var json = File.ReadAllText(jsonPath);
            // JSON escapes backslashes in account names (domain\user).
            json.Should().Contain(domainUser.Replace(@"\", @"\\"));
            json.Should().Contain(ServiceCredentialsRollbackData.SecretKey(Constants.AgentServiceName));
            json.Should().Contain("\"UseEmptyPassword\": false");
        }

        [Fact]
        public void RollbackDataStore_RestoresSnapshots_InReverseAddOrder()
        {
            var restoreOrder = new List<string>();
            _serviceController
                .Setup(c => c.SetCredentials(It.IsAny<string>(), It.IsAny<string>(), It.IsAny<string>()))
                .Callback<string, string, string>((serviceName, _, _) => restoreOrder.Add(serviceName));
            _serviceController.Setup(c => c.GetServiceStartName(Constants.AgentServiceName)).Returns("LocalSystem");
            _serviceController.Setup(c => c.GetServiceStartName(Constants.TraceAgentServiceName)).Returns("LocalSystem");

            var storageName = $"credential-rollback-{Guid.NewGuid():N}";
            var store = new RollbackDataStore(
                _session.Object, storageName, _fileSystemServices.Object, _serviceController.Object);

            var agentSnapshot = ServiceCredentialsRollbackData.Capture(
                Constants.AgentServiceName,
                _serviceController.Object,
                _nativeMethods.Object,
                _session.Object);
            var traceSnapshot = ServiceCredentialsRollbackData.Capture(
                Constants.TraceAgentServiceName,
                _serviceController.Object,
                _nativeMethods.Object,
                _session.Object);
            store.Add(agentSnapshot);
            store.Add(traceSnapshot);
            store.Store();

            var restoreStore = new RollbackDataStore(
                _session.Object, storageName, _fileSystemServices.Object, _serviceController.Object);
            restoreStore.Restore();

            restoreOrder.Should().Equal(
                Constants.TraceAgentServiceName,
                Constants.AgentServiceName);
        }
    }
}
