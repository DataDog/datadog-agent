using System;
using AutoFixture.Xunit2;
using FluentAssertions;
using Microsoft.Deployment.WindowsInstaller;
using Xunit;

namespace CustomActions.Tests.ProcessUserCustomActions
{
    public class ProcessUserCustomActionsDomainClientTests : BaseProcessUserCustomActionsDomainTests
    {
        public ProcessUserCustomActionsDomainClientTests()
        {
            Test.WithDomainClient();
        }

        /// <summary>
        /// Same as <see cref="UserCustomActionsTests.ProcessDdAgentUserCredentials_With_Default_Credentials"/> but on a
        /// Domain Client.
        /// </summary>
        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_Succeeds_With_Default_Credentials_But_Non_Existing_User_On_DomainClient()
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

        /// <summary>
        /// Upgrading an existing installation with a local user without explicitly specifying
        /// a username/password should work.
        /// </summary>
        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_Succeeds_With_Default_Credentials_For_Existing_User_On_DomainClient()
        {
            Test.WithLocalUser(Environment.MachineName, "ddagentuser");

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDAGENTUSER_FOUND", "true").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_SID" && !string.IsNullOrEmpty(kvp.Value)).And
                .Contain("DDAGENTUSER_PROCESSED_NAME", "ddagentuser").And
                .Contain("DDAGENTUSER_PROCESSED_DOMAIN", Environment.MachineName).And
                .Contain("DDAGENTUSER_PROCESSED_FQ_NAME", $"{Environment.MachineName}\\ddagentuser").And
                .Contain("DDAGENTUSER_RESET_PASSWORD", "yes").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && !string.IsNullOrEmpty(kvp.Value));
        }

        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_Succeeds_With_Custom_Existing_Domain_User_On_DomainClient(
            string ddAgentUserName,
            string ddAgentUserPassword)
        {
            Test.WithDomainUser(ddAgentUserName);

            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns($"{Domain}@{ddAgentUserName}");
            Test.Session
                .Setup(session => session["DDAGENTUSER_PASSWORD"]).Returns(ddAgentUserPassword);

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDAGENTUSER_FOUND", "true").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_SID" && !string.IsNullOrEmpty(kvp.Value)).And
                .Contain("DDAGENTUSER_PROCESSED_NAME", ddAgentUserName).And
                .Contain("DDAGENTUSER_PROCESSED_DOMAIN", Domain).And
                .Contain("DDAGENTUSER_PROCESSED_FQ_NAME", $"{Domain}\\{ddAgentUserName}").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_RESET_PASSWORD" && string.IsNullOrEmpty(kvp.Value)).And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && kvp.Value == ddAgentUserPassword);
        }

        /// <summary>
        /// The domain user exists but no password was specified, so the install should fail.
        /// </summary>
        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_Fails_With_Custom_Existing_Domain_User_And_No_Password_On_DomainClient(
            string ddAgentUserName)
        {
            Test.WithDomainUser(ddAgentUserName);

            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns($"{Domain}\\{ddAgentUserName}");

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Failure);

            Test.Properties.Should()
                .OnlyContain(kvp => (kvp.Key == "DDAGENTUSER_FOUND" && kvp.Value == "true") ||
                                    (kvp.Key == "DDAGENTUSER_SID" && !string.IsNullOrEmpty(kvp.Value)));
        }

        /// <summary>
        /// The domain user does not exists and we are on a domain client, so the install should fail.
        /// </summary>
        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_Fails_With_Custom_Non_Existing_Domain_User_On_DomainClient(string ddAgentUserName)
        {
            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns($"{Domain}\\{ddAgentUserName}");

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Failure);

            Test.Properties.Should()
                .OnlyContain(kvp => (kvp.Key == "DDAGENTUSER_FOUND" && kvp.Value == "false") ||
                                    (kvp.Key == "DDAGENTUSER_SID" && string.IsNullOrEmpty(kvp.Value)));
        }

        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_With_Custom_Non_Existing_Local_User_On_DomainClient(string ddAgentUserName)
        {
            Test.WithDomainUser(ddAgentUserName);

            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns($"{Environment.MachineName}\\{ddAgentUserName}");

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDAGENTUSER_FOUND", "false").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_SID" && string.IsNullOrEmpty(kvp.Value)).And
                .Contain("DDAGENTUSER_PROCESSED_NAME", ddAgentUserName).And
                .Contain("DDAGENTUSER_PROCESSED_DOMAIN", Environment.MachineName).And
                .Contain("DDAGENTUSER_PROCESSED_FQ_NAME", $"{Environment.MachineName}\\{ddAgentUserName}").And
                .Contain("DDAGENTUSER_RESET_PASSWORD", "yes").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && !string.IsNullOrEmpty(kvp.Value));
        }

        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_With_Custom_Non_Existing_Local_User_With_Missing_Domain_Part_On_DomainClient(string ddAgentUserName)
        {
            Test.WithDomainUser(ddAgentUserName);

            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns(ddAgentUserName);

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDAGENTUSER_FOUND", "false").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_SID" && string.IsNullOrEmpty(kvp.Value)).And
                .Contain("DDAGENTUSER_PROCESSED_NAME", ddAgentUserName).And
                .Contain("DDAGENTUSER_PROCESSED_DOMAIN", Environment.MachineName).And
                .Contain("DDAGENTUSER_PROCESSED_FQ_NAME", $"{Environment.MachineName}\\{ddAgentUserName}").And
                .Contain("DDAGENTUSER_RESET_PASSWORD", "yes").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && !string.IsNullOrEmpty(kvp.Value));
        }
    }
}
