using System;
using System.Security.Principal;
using AutoFixture.Xunit2;
using Datadog.CustomActions.Native;
using FluentAssertions;
using Microsoft.Deployment.WindowsInstaller;
using Moq;
using Xunit;

namespace CustomActions.Tests.ProcessUserCustomActions
{
    public class UserCustomActionsTests
    {
        public ProcessUserCustomActionsTestSetup Test { get; } = new();

        /// <summary>
        /// Base case, installing with default credentials
        /// on a workstation (NOT domain controller).
        /// </summary>
        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_With_Default_Credentials()
        {
            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDAGENTUSER_FOUND", "false").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_SID" && string.IsNullOrEmpty(kvp.Value)).And
                .Contain("DDAGENTUSER_PROCESSED_NAME", "ddagentuser").And
                .Contain("DDAGENTUSER_PROCESSED_DOMAIN", Environment.MachineName).And
                .Contain("DDAGENTUSER_PROCESSED_FQ_NAME", $"{Environment.MachineName}\\ddagentuser").And
                .Contain("DDAGENTUSER_RESET_PASSWORD", "yes").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && !string.IsNullOrEmpty(kvp.Value));
        }

        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_With_Dot_Credentials()
        {
            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns(".\\ddagentuser");

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDAGENTUSER_FOUND", "false").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_SID" && string.IsNullOrEmpty(kvp.Value)).And
                .Contain("DDAGENTUSER_PROCESSED_NAME", "ddagentuser").And
                .Contain("DDAGENTUSER_PROCESSED_DOMAIN", Environment.MachineName).And
                .Contain("DDAGENTUSER_PROCESSED_FQ_NAME", $"{Environment.MachineName}\\ddagentuser").And
                .Contain("DDAGENTUSER_RESET_PASSWORD", "yes").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && !string.IsNullOrEmpty(kvp.Value));
        }

        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_With_Non_User_Account(string userDomain, string userName)
        {
            Test.WithLocalUser(userDomain, userName, SID_NAME_USE.SidTypeDomain);

            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns($"{userDomain}\\{userName}");

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Failure);

            Test.Properties
                .Should()
                .BeEmpty();
        }

        /// <summary>
        /// Test when the user tries to use "LocalSystem"
        /// </summary>
        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_With_Local_System()
        {
            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns("LocalSystem");

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDAGENTUSER_FOUND", "true").And
                .Contain("DDAGENTUSER_IS_SERVICE_ACCOUNT", "true").And
                .Contain("DDAGENTUSER_SID", new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null).Value).And
                .Contain("DDAGENTUSER_PROCESSED_NAME", "SYSTEM").And
                .Contain("DDAGENTUSER_PROCESSED_DOMAIN", "NT AUTHORITY").And
                .Contain("DDAGENTUSER_PROCESSED_FQ_NAME", "NT AUTHORITY\\SYSTEM").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_RESET_PASSWORD" && string.IsNullOrEmpty(kvp.Value)).And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && string.IsNullOrEmpty(kvp.Value));
        }

        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_With_Failing_IsDomainAccount(string userDomain, string userName)
        {
            Test.WithLocalUser(userDomain, userName)
                .NativeMethods.Setup(n => n.IsDomainAccount(It.IsAny<SecurityIdentifier>())).Throws<Exception>();

            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns($"{userDomain}\\{userName}");

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Failure);

            Test.Properties.Should()
                .Contain("DDAGENTUSER_FOUND", "true");
        }

        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_With_Agent_User_Equal_Current_User(string userDomain, string userName)
        {
            var userSID = new SecurityIdentifier("S-1-0-5");
            Test.WithLocalUser(userDomain, userName, SID_NAME_USE.SidTypeUser, userSID)
                .WithCurrentUser(userName, userSID);

            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns($"{userDomain}\\{userName}");

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Failure);

            Test.Properties.Should()
                .Contain("DDAGENTUSER_FOUND", "true").And
                .Contain("DDAGENTUSER_SID", userSID.ToString());
        }

        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_With_Local_System_And_Current_User_Local_System()
        {
            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns("LocalSystem");

            Test.WithCurrentUser("SYSTEM", new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null));

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDAGENTUSER_FOUND", "true").And
                .Contain("DDAGENTUSER_SID", new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null).Value).And
                .Contain("DDAGENTUSER_PROCESSED_NAME", "SYSTEM").And
                .Contain("DDAGENTUSER_PROCESSED_DOMAIN", "NT AUTHORITY").And
                .Contain("DDAGENTUSER_PROCESSED_FQ_NAME", "NT AUTHORITY\\SYSTEM").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_RESET_PASSWORD" && string.IsNullOrEmpty(kvp.Value)).And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && string.IsNullOrEmpty(kvp.Value));
        }

        [Fact]
        public void ProcessDdAgentUserCredentials_Catch_Semicolon_In_Password()
        {
            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns("ddagentuser");
            Test.Session
                .Setup(session => session["DDAGENTUSER_PASSWORD"]).Returns("password;123");

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Failure);
        }

        [Fact]
        public void ProcessDdAgentUserCredentials_Handles_Lanmanserver_Not_Availabile()
        {
            // IsDomainController throws an exception if the Lanmanserver service is not available
            Test.NativeMethods
                .Setup(n => n.IsDomainController()).Throws<Exception>();

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);
        }
    }
}
