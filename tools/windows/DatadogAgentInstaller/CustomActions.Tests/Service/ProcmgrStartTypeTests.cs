using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Moq;
using System.Security.Principal;
using System.ServiceProcess;
using Xunit;

namespace CustomActions.Tests.Service
{
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

        private void VerifyProcmgrStartTypeUnchanged()
        {
            Test.ServiceController.Verify(
                c => c.SetStartType(ProcmgrServiceName, It.IsAny<ServiceStartMode>()), Times.Never);
        }

        [Fact]
        public void ConfigureServiceUsers_KeepsProcmgrStartupUnchanged_ForDomainAccountWithoutPassword()
        {
            GivenServiceAccount(false);
            GivenAgentUserPassword(null);

            Test.Create().ConfigureServiceUsers(DomainUserName, DomainUserSid);

            VerifyProcmgrStartTypeUnchanged();
            Test.ServiceController.Verify(
                c => c.SetCredentials(ProcmgrServiceName, "LocalSystem", ""), Times.Once);
        }

        [Fact]
        public void ConfigureServiceUsers_KeepsProcmgrStartupUnchanged_ForDomainAccountWithPassword()
        {
            GivenServiceAccount(false);
            GivenAgentUserPassword("a-real-password");

            Test.Create().ConfigureServiceUsers(DomainUserName, DomainUserSid);

            VerifyProcmgrStartTypeUnchanged();
            Test.ServiceController.Verify(
                c => c.SetCredentials(ProcmgrServiceName, "LocalSystem", ""), Times.Once);
        }

        [Fact]
        public void ConfigureServiceUsers_KeepsProcmgrStartupUnchanged_ForServiceAccount()
        {
            GivenServiceAccount(true);
            GivenAgentUserPassword(null);

            Test.Create().ConfigureServiceUsers("TESTDOMAIN\\ddagentuser$", DomainUserSid);

            VerifyProcmgrStartTypeUnchanged();
            Test.ServiceController.Verify(
                c => c.SetCredentials(ProcmgrServiceName, "LocalSystem", ""), Times.Once);
        }

        [Fact]
        public void ConfigureServiceUsers_KeepsProcmgrStartupUnchanged_ForLocalSystem()
        {
            GivenServiceAccount(true);
            GivenAgentUserPassword(null);

            Test.Create().ConfigureServiceUsers("LocalSystem", LocalSystemSid);

            VerifyProcmgrStartTypeUnchanged();
            Test.ServiceController.Verify(
                c => c.SetCredentials(ProcmgrServiceName, "LocalSystem", ""), Times.Once);
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
