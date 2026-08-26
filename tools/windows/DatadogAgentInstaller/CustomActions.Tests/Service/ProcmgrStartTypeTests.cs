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

        private void VerifyAgentUserCredentialsUnchanged()
        {
            foreach (var serviceName in new[]
                     {
                         Constants.AgentServiceName,
                         Constants.TraceAgentServiceName,
                         Constants.SecurityAgentServiceName,
                     })
            {
                Test.ServiceController.Verify(
                    c => c.SetCredentials(serviceName, It.IsAny<string>(), It.IsAny<string>()),
                    Times.Never);
            }
        }

        [Fact]
        public void ConfigureServiceUsers_PreservesAgentServiceCredentials_WhenPasswordNotProvided()
        {
            GivenServiceAccount(false);
            GivenAgentUserPassword(null);

            Test.Create().ConfigureServiceUsers(DomainUserName, DomainUserSid);

            VerifyAgentUserCredentialsUnchanged();
        }

        [Fact]
        public void ConfigureServiceUsers_EnablesProcmgr_ForDomainAccountWithoutPassword()
        {
            GivenServiceAccount(false);
            GivenAgentUserPassword(null);

            Test.Create().ConfigureServiceUsers(DomainUserName, DomainUserSid);

            VerifyProcmgrStartType(ServiceStartMode.Manual);
            Test.ServiceController.Verify(
                c => c.SetCredentials(ProcmgrServiceName, "LocalSystem", ""), Times.Once);
        }

        [Fact]
        public void ConfigureServiceUsers_EnablesProcmgr_ForDomainAccountWithPassword()
        {
            GivenServiceAccount(false);
            GivenAgentUserPassword("a-real-password");

            Test.Create().ConfigureServiceUsers(DomainUserName, DomainUserSid);

            VerifyProcmgrStartType(ServiceStartMode.Manual);
            Test.ServiceController.Verify(
                c => c.SetCredentials(ProcmgrServiceName, "LocalSystem", ""), Times.Once);
        }

        [Fact]
        public void ConfigureServiceUsers_EnablesProcmgr_ForServiceAccount()
        {
            GivenServiceAccount(true);
            GivenAgentUserPassword(null);

            Test.Create().ConfigureServiceUsers("TESTDOMAIN\\ddagentuser$", DomainUserSid);

            VerifyProcmgrStartType(ServiceStartMode.Manual);
            Test.ServiceController.Verify(
                c => c.SetCredentials(ProcmgrServiceName, "LocalSystem", ""), Times.Once);
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

        private void GivenScmServicePassword(string password)
        {
            Test.NativeMethods
                .Setup(n => n.FetchScmServicePassword(Constants.AgentServiceName))
                .Returns(password);
        }

        private void GivenServiceExists(string serviceName, bool exists = true)
        {
            Test.ServiceController
                .Setup(c => c.ServiceExists(serviceName))
                .Returns(exists);
        }

        private void GivenServiceStartName(string serviceName, string account)
        {
            Test.ServiceController
                .Setup(c => c.GetServiceStartName(serviceName))
                .Returns(account);
        }

        private void GivenPasswordNotProvidedDomainUpgrade()
        {
            GivenServiceAccount(false);
            GivenAgentUserPassword(null);
        }

        [Fact]
        public void ConfigureServiceUsers_ConfiguresLocalSystemNonCoreAgentUserServices_WhenPasswordNotProvided()
        {
            const string scmPassword = "scm-stored-password";
            GivenPasswordNotProvidedDomainUpgrade();
            GivenServiceExists(Constants.PrivateActionRunnerServiceName);
            GivenServiceStartName(Constants.PrivateActionRunnerServiceName, "LocalSystem");
            GivenScmServicePassword(scmPassword);

            Test.Create().ConfigureServiceUsers(DomainUserName, DomainUserSid);

            VerifyAgentUserCredentialsUnchanged();
            Test.ServiceController.Verify(
                c => c.SetCredentials(Constants.PrivateActionRunnerServiceName, DomainUserName, scmPassword),
                Times.Once);
        }

        [Fact]
        public void ConfigureServiceUsers_SkipsNonCoreAgentUserServicesAlreadyOnAgentUser_WhenPasswordNotProvided()
        {
            GivenPasswordNotProvidedDomainUpgrade();
            GivenServiceExists(Constants.TraceAgentServiceName);
            GivenServiceStartName(Constants.TraceAgentServiceName, DomainUserName);
            GivenScmServicePassword("scm-stored-password");

            Test.Create().ConfigureServiceUsers(DomainUserName, DomainUserSid);

            VerifyAgentUserCredentialsUnchanged();
            Test.ServiceController.Verify(
                c => c.SetCredentials(Constants.TraceAgentServiceName, It.IsAny<string>(), It.IsAny<string>()),
                Times.Never);
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
