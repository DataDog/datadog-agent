using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using FluentAssertions;
using Moq;
using System;
using System.ComponentModel;
using System.Security.Principal;
using System.ServiceProcess;
using Xunit;

namespace CustomActions.Tests.Service
{
    /// <summary>
    /// Unit tests for the dd-procmgr-service start type handling in ConfigureServiceUsers, which must
    /// disable the service when the Agent user password is unavailable, and only then.
    /// </summary>
    public class ProcmgrStartTypeTests
    {
        private const string DomainUserName = "TESTDOMAIN\\ddagentuser";
        private const string ProcmgrServiceName = "dd-procmgr-service";

        private static readonly SecurityIdentifier DomainUserSid =
            new SecurityIdentifier("S-1-5-21-1-2-3-1001");

        private static readonly SecurityIdentifier LocalSystemSid =
            new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null);

        public ServiceCustomActionsTestSetup Test { get; } = new();

        private void GivenAgentUserPassword(string password)
        {
            Test.Session
                .Setup(session => session["DDAGENTUSER_PROCESSED_PASSWORD"])
                .Returns(password);
        }

        private void GivenServiceAccount(bool isServiceAccount)
        {
            Test.NativeMethods
                .Setup(n => n.IsServiceAccount(It.IsAny<SecurityIdentifier>()))
                .Returns(isServiceAccount);
        }

        private void VerifyProcmgrStartType(ServiceStartMode expected)
        {
            Test.ServiceController.Verify(
                c => c.SetStartType(ProcmgrServiceName, expected), Times.Once);
            Test.ServiceController.Verify(
                c => c.SetStartType(ProcmgrServiceName, It.IsNotIn(expected)), Times.Never);
        }

        [Fact]
        public void ConfigureServiceUsers_DisablesProcmgr_ForDomainAccountWithoutPassword()
        {
            GivenServiceAccount(false);
            GivenAgentUserPassword(null);

            Test.Create().ConfigureServiceUsers(DomainUserName, DomainUserSid);

            VerifyProcmgrStartType(ServiceStartMode.Disabled);
            // the empty password stored by InstallServices is left alone
            Test.ServiceController.Verify(
                c => c.SetCredentials(ProcmgrServiceName, DomainUserName, null), Times.Once);
        }

        [Fact]
        public void ConfigureServiceUsers_EnablesProcmgr_ForDomainAccountWithPassword()
        {
            GivenServiceAccount(false);
            GivenAgentUserPassword("a-real-password");

            Test.Create().ConfigureServiceUsers(DomainUserName, DomainUserSid);

            VerifyProcmgrStartType(ServiceStartMode.Manual);
        }

        [Fact]
        public void ConfigureServiceUsers_EnablesProcmgr_ForServiceAccount()
        {
            // gMSA: no password is needed or expected
            GivenServiceAccount(true);
            GivenAgentUserPassword(null);

            Test.Create().ConfigureServiceUsers("TESTDOMAIN\\ddagentuser$", DomainUserSid);

            VerifyProcmgrStartType(ServiceStartMode.Manual);
        }

        [Fact]
        public void ConfigureServiceUsers_EnablesProcmgr_ForLocalSystem()
        {
            GivenServiceAccount(true);
            GivenAgentUserPassword(null);

            Test.Create().ConfigureServiceUsers("LocalSystem", LocalSystemSid);

            VerifyProcmgrStartType(ServiceStartMode.Manual);
            Test.ServiceController.Verify(
                c => c.SetCredentials(ProcmgrServiceName, "LocalSystem", ""), Times.Once);
        }

        [Fact]
        public void ConfigureServiceUsers_IgnoresProcmgrServiceDoesNotExist()
        {
            GivenServiceAccount(false);
            GivenAgentUserPassword(null);
            Test.ServiceController
                .Setup(c => c.SetStartType(ProcmgrServiceName, It.IsAny<ServiceStartMode>()))
                .Throws(new InvalidOperationException("nope", new Win32Exception(1060)));

            var sut = Test.Create();

            sut.Invoking(s => s.ConfigureServiceUsers(DomainUserName, DomainUserSid))
                .Should().NotThrow();
        }

        /// <summary>
        /// Other failures must propagate so the install fails. A failed upgrade leaves the customer on a
        /// working version, which beats proceeding and locking out the account.
        /// </summary>
        [Fact]
        public void ConfigureServiceUsers_PropagatesOtherStartTypeFailures()
        {
            GivenServiceAccount(false);
            GivenAgentUserPassword(null);
            Test.ServiceController
                .Setup(c => c.SetStartType(ProcmgrServiceName, It.IsAny<ServiceStartMode>()))
                .Throws(new Win32Exception(5)); // ERROR_ACCESS_DENIED

            var sut = Test.Create();

            sut.Invoking(s => s.ConfigureServiceUsers(DomainUserName, DomainUserSid))
                .Should().Throw<Win32Exception>();
        }

        [Fact]
        public void ConfigureServiceUsers_DoesNotChangeStartTypeOfOtherServices()
        {
            GivenServiceAccount(false);
            GivenAgentUserPassword(null);

            Test.Create().ConfigureServiceUsers(DomainUserName, DomainUserSid);

            Test.ServiceController.Verify(
                c => c.SetStartType(It.IsNotIn(ProcmgrServiceName), It.IsAny<ServiceStartMode>()),
                Times.Never);
        }
    }
}
