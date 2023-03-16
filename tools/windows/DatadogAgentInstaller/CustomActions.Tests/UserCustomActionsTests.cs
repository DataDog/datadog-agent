using System;
using System.Collections.Generic;
using System.Security.Principal;
using AutoFixture.Xunit2;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using FluentAssertions;
using Microsoft.Deployment.WindowsInstaller;
using Moq;
using Xunit;

namespace CustomActions.Tests
{
    public class UserCustomActionsTests
    {
        /// <summary>
        /// Base case, installing with default credentials
        /// on a workstation (NOT domain controller).
        /// </summary>
        [Theory]
        [InlineAutoData]
        public void ProcessDdAgentUserCredentials_With_Default_Credentials()
        {
            var properties = new Dictionary<string, string>();
            var sessionMock = new Mock<ISession>();

            // Use default credentials

            sessionMock
                .SetupSet(s => s[It.IsAny<string>()] = It.IsAny<string>())
                .Callback((string key, string value) => properties[key] = value);

            var nativeMethodsMock = new Mock<INativeMethods>();
            nativeMethodsMock.Setup(n => n.LookupAccountName(
                    It.IsAny<string>(),
                    out It.Ref<string>.IsAny,
                    out It.Ref<string>.IsAny,
                    out It.Ref<SecurityIdentifier>.IsAny,
                    out It.Ref<SID_NAME_USE>.IsAny))
                .Callback(
                    (
                        string _,
                        out string user,
                        out string domain,
                        out SecurityIdentifier sid,
                        out SID_NAME_USE nameUse

                    ) =>
                    {
                        user = null;
                        domain = null;
                        sid = null;
                        nameUse = SID_NAME_USE.SidTypeUnknown;
                    })
                .Returns(false);

            var registryServicesMock = new Mock<IRegistryServices>();
            var directoryServicesMock = new Mock<IDirectoryServices>();
            var fileServicesMock = new Mock<IFileServices>();
            var serviceControllerMock = new Mock<IServiceController>();

            var sut = new UserCustomActions(
                sessionMock.Object,
                nativeMethodsMock.Object,
                registryServicesMock.Object,
                directoryServicesMock.Object,
                fileServicesMock.Object,
                serviceControllerMock.Object
            );

            sut.ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            properties.Should()
                .Contain("DDAGENTUSER_FOUND", "false").And
                .Contain("DDAGENTUSER_PROCESSED_NAME", "ddagentuser").And
                .Contain("DDAGENTUSER_PROCESSED_DOMAIN", Environment.MachineName).And
                .Contain("DDAGENTUSER_PROCESSED_FQ_NAME", $"{Environment.MachineName}\\ddagentuser").And
                .Contain("DDAGENTUSER_RESET_PASSWORD", "yes").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && !string.IsNullOrEmpty(kvp.Value));
        }

        /// <summary>
        /// Test when the user tries to use "LocalSystem"
        /// </summary>
        [Theory]
        [InlineAutoData]
        public void ProcessDdAgentUserCredentials_With_Local_System()
        {
            var properties = new Dictionary<string, string>();
            var sessionMock = new Mock<ISession>();

            sessionMock
                .Setup(session => session["DDAGENTUSER_NAME"])
                .Returns("LocalSystem");
            var userName = "SYSTEM";
            var userDomain = "NT AUTHORITY";
            var userSid = new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null);

            sessionMock
                .SetupSet(s => s[It.IsAny<string>()] = It.IsAny<string>())
                .Callback((string key, string value) => properties[key] = value);

            var nativeMethodsMock = new Mock<INativeMethods>();
            nativeMethodsMock.Setup(n => n.LookupAccountName(
                    It.IsAny<string>(),
                    out It.Ref<string>.IsAny,
                    out It.Ref<string>.IsAny,
                    out It.Ref<SecurityIdentifier>.IsAny,
                    out It.Ref<SID_NAME_USE>.IsAny))
                .Callback(
                    (
                        string _,
                        out string user,
                        out string domain,
                        out SecurityIdentifier sid,
                        out SID_NAME_USE nameUse

                    ) =>
                    {
                        user = userName;
                        domain = userDomain;
                        sid = userSid;
                        nameUse = SID_NAME_USE.SidTypeWellKnownGroup;
                    })
                .Returns(true);

            // IsServiceAccount should be true for LocalSystem
            nativeMethodsMock
                .Setup(n => n.IsServiceAccount(userSid))
                .Returns(true);

            var registryServicesMock = new Mock<IRegistryServices>();
            var directoryServicesMock = new Mock<IDirectoryServices>();
            var fileServicesMock = new Mock<IFileServices>();
            var serviceControllerMock = new Mock<IServiceController>();

            var sut = new UserCustomActions(
                sessionMock.Object,
                nativeMethodsMock.Object,
                registryServicesMock.Object,
                directoryServicesMock.Object,
                fileServicesMock.Object,
                serviceControllerMock.Object
            );

            sut.ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            properties.Should()
                .Contain("DDAGENTUSER_FOUND", "true").And
                .Contain("DDAGENTUSER_SID", userSid.Value).And
                .Contain("DDAGENTUSER_PROCESSED_NAME", userName).And
                .Contain("DDAGENTUSER_PROCESSED_DOMAIN", userDomain).And
                .Contain("DDAGENTUSER_PROCESSED_FQ_NAME", $"{userDomain}\\{userName}").And
                .NotContainKey("DDAGENTUSER_RESET_PASSWORD").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" &&
                                // !! The password should be null
                                string.IsNullOrEmpty(kvp.Value));
        }

        [Theory]
        [InlineAutoData]
        public void ProcessDdAgentUserCredentials_Dont_Run_Twice()
        {
            var properties = new Dictionary<string, string>();
            var sessionMock = new Mock<ISession>();
            sessionMock
                .Setup(session => session["DDAGENTUSER_PROCESSED_FQ_NAME"])
                .Returns("not empty");
            sessionMock.SetupSet(s => s[It.IsAny<string>()] = It.IsAny<string>())
                .Callback((string key, string value) => properties[key] = value);

            var nativeMethodsMock = new Mock<INativeMethods>();
            var registryServicesMock = new Mock<IRegistryServices>();
            var directoryServicesMock = new Mock<IDirectoryServices>();
            var fileServicesMock = new Mock<IFileServices>();
            var serviceControllerMock = new Mock<IServiceController>();

            var sut = new UserCustomActions(
                sessionMock.Object,
                nativeMethodsMock.Object,
                registryServicesMock.Object,
                directoryServicesMock.Object,
                fileServicesMock.Object,
                serviceControllerMock.Object
            );

            sut.ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            properties.Should().BeEmpty();

            // The only way we can verify that the code did not go further
            // is to check individual function call.

            sessionMock.Verify(session => session["DDAGENTUSER_PROCESSED_FQ_NAME"], Times.Once);

            // This is always called by SessionExtensions
            sessionMock.VerifyGet(session => session.Components);
            sessionMock.VerifyGet(session => session["INSTALLDIR"]);

            sessionMock.VerifyNoOtherCalls();
            nativeMethodsMock.VerifyNoOtherCalls();
            registryServicesMock.VerifyNoOtherCalls();
            directoryServicesMock.VerifyNoOtherCalls();
            fileServicesMock.VerifyNoOtherCalls();
            serviceControllerMock.VerifyNoOtherCalls();
        }

        [Theory]
        [InlineAutoData]
        public void ProcessDdAgentUserCredentials_Fails_With_No_Credentials_On_DomainController()
        {
            var sessionMock = new Mock<ISession>();
            var nativeMethodsMock = new Mock<INativeMethods>();
            nativeMethodsMock.Setup(n => n.IsDomainController()).Returns(true);
            var registryServicesMock = new Mock<IRegistryServices>();
            var directoryServicesMock = new Mock<IDirectoryServices>();
            var fileServicesMock = new Mock<IFileServices>();
            var serviceControllerMock = new Mock<IServiceController>();

            var sut = new UserCustomActions(
                sessionMock.Object,
                nativeMethodsMock.Object,
                registryServicesMock.Object,
                directoryServicesMock.Object,
                fileServicesMock.Object,
                serviceControllerMock.Object
            );

            sut.ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Failure);

            // The only way we can verify that the code did not go further
            // is to check individual function call.

            sessionMock.Verify(session => session["DDAGENTUSER_PROCESSED_FQ_NAME"], Times.Once);
            sessionMock.Verify(session => session["DDAGENTUSER_NAME"], Times.Once);
            sessionMock.Verify(session => session["DDAGENTUSER_PASSWORD"], Times.Once);
            sessionMock.Verify(session => session.Log(It.IsAny<string>(), It.IsAny<string>(), It.IsAny<string>(), It.IsAny<int>()));

            // This is always called by SessionExtensions
            sessionMock.VerifyGet(session => session.Components);
            sessionMock.VerifyGet(session => session["INSTALLDIR"]);

            sessionMock.VerifyNoOtherCalls();
            nativeMethodsMock.Verify(n => n.IsDomainController());
            nativeMethodsMock.VerifyNoOtherCalls();
            serviceControllerMock.Verify(n => n.GetServiceNames());
            serviceControllerMock.VerifyNoOtherCalls();
        }

        [Theory]
        [InlineAutoData]
        public void ProcessDdAgentUserCredentials_Succeeds_With_No_Credentials_But_Services_Exists_On_DomainController(
            string ddAgentUserName,
            string ddAgentUserDomain
            )
        {
            var properties = new Dictionary<string, string>();
            var sessionMock = new Mock<ISession>();

            // Should be read from registry in prod
            sessionMock
                .Setup(session => session["DDAGENTUSER_NAME"])
                .Returns($"{ddAgentUserDomain}\\{ddAgentUserName}");

            var userSid = new SecurityIdentifier("S-1-0-0");
            // Password can be empty on upgrades

            sessionMock.SetupSet(s => s[It.IsAny<string>()] = It.IsAny<string>())
                .Callback((string key, string value) => properties[key] = value);

            var nativeMethodsMock = new Mock<INativeMethods>();
            nativeMethodsMock.Setup(n => n.IsDomainController()).Returns(true);
            nativeMethodsMock.Setup(n => n.LookupAccountName(
                    It.IsAny<string>(),
                    out It.Ref<string>.IsAny,
                    out It.Ref<string>.IsAny,
                    out It.Ref<SecurityIdentifier>.IsAny,
                    out It.Ref<SID_NAME_USE>.IsAny))
                .Callback(
                    (
                        string _,
                        out string user,
                        out string domain,
                        out SecurityIdentifier sid,
                        out SID_NAME_USE nameUse

                    ) =>
                    {
                        user = ddAgentUserName;
                        domain = ddAgentUserDomain;
                        sid = userSid;
                        nameUse = SID_NAME_USE.SidTypeUser;
                    })
                .Returns(true);

            var registryServicesMock = new Mock<IRegistryServices>();
            var directoryServicesMock = new Mock<IDirectoryServices>();
            var fileServicesMock = new Mock<IFileServices>();
            var serviceControllerMock = new Mock<IServiceController>();
            serviceControllerMock.Setup(s => s.GetServiceNames()).Returns(new[] { "datadogagent" });

            var sut = new UserCustomActions(
                sessionMock.Object,
                nativeMethodsMock.Object,
                registryServicesMock.Object,
                directoryServicesMock.Object,
                fileServicesMock.Object,
                serviceControllerMock.Object
            );

            sut.ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            properties.Should()
                .Contain("DDAGENTUSER_FOUND", "true").And
                .Contain("DDAGENTUSER_SID", userSid.Value).And
                .Contain("DDAGENTUSER_PROCESSED_NAME", ddAgentUserName).And
                .Contain("DDAGENTUSER_PROCESSED_DOMAIN", ddAgentUserDomain).And
                .Contain("DDAGENTUSER_PROCESSED_FQ_NAME", $"{ddAgentUserDomain}\\{ddAgentUserName}").And
                .Contain("DDAGENTUSER_RESET_PASSWORD", "yes").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && !string.IsNullOrEmpty(kvp.Value));
        }

        [Theory]
        [InlineAutoData("gMSAAccount$", "aDomain", "bDomain")]
        [InlineAutoData("gMSAAccount$", "", "bDomain")]
        public void ProcessDdAgentUserCredentials_Succeeds_With_gMsaAccount_On_DomainController(
            string ddAgentUserName,
            string ddAgentUserDomain,
            string defaultDomain)
        {
            var properties = new Dictionary<string, string>();
            var sessionMock = new Mock<ISession>();

            sessionMock
                .Setup(session => session["DDAGENTUSER_NAME"])
                .Returns($"{ddAgentUserDomain}\\{ddAgentUserName}");

            var userSid = new SecurityIdentifier("S-1-0-0");

            sessionMock.SetupSet(s => s[It.IsAny<string>()] = It.IsAny<string>())
                .Callback((string key, string value) => properties[key] = value);

            var nativeMethodsMock = new Mock<INativeMethods>();
            nativeMethodsMock.Setup(n => n.IsDomainController()).Returns(true);
            nativeMethodsMock.Setup(n => n.IsServiceAccount(userSid)).Returns(true);
            nativeMethodsMock.Setup(n => n.IsDomainAccount(userSid)).Returns(true);
            nativeMethodsMock.Setup(n => n.GetComputerDomain()).Returns(defaultDomain);
            nativeMethodsMock.Setup(n => n.LookupAccountName(
                    It.IsAny<string>(),
                    out It.Ref<string>.IsAny,
                    out It.Ref<string>.IsAny,
                    out It.Ref<SecurityIdentifier>.IsAny,
                    out It.Ref<SID_NAME_USE>.IsAny))
                .Callback(
                    (
                        string _,
                        out string user,
                        out string domain,
                        out SecurityIdentifier sid,
                        out SID_NAME_USE nameUse

                    ) =>
                    {
                        user = ddAgentUserName;
                        domain = ddAgentUserDomain;
                        sid = userSid;
                        nameUse = SID_NAME_USE.SidTypeUser;
                    })
                .Returns(true);

            var registryServicesMock = new Mock<IRegistryServices>();
            var directoryServicesMock = new Mock<IDirectoryServices>();
            var fileServicesMock = new Mock<IFileServices>();
            var serviceControllerMock = new Mock<IServiceController>();

            var sut = new UserCustomActions(
                sessionMock.Object,
                nativeMethodsMock.Object,
                registryServicesMock.Object,
                directoryServicesMock.Object,
                fileServicesMock.Object,
                serviceControllerMock.Object
            );

            sut.ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            properties.Should()
                .Contain("DDAGENTUSER_FOUND", "true").And
                .Contain("DDAGENTUSER_SID", userSid.Value).And
                .Contain("DDAGENTUSER_PROCESSED_NAME", ddAgentUserName).And
                .Contain("DDAGENTUSER_PROCESSED_DOMAIN", string.IsNullOrEmpty(ddAgentUserDomain) ? defaultDomain : ddAgentUserDomain).And
                .Contain("DDAGENTUSER_PROCESSED_FQ_NAME", $"{(string.IsNullOrEmpty(ddAgentUserDomain) ? defaultDomain : ddAgentUserDomain)}\\{ddAgentUserName}").And
                .NotContainKey("DDAGENTUSER_RESET_PASSWORD").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" &&
                                // !! Password should be null
                                string.IsNullOrEmpty(kvp.Value));
        }
    }
}
