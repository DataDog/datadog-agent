using AutoFixture.Xunit2;
using FluentAssertions;
using Microsoft.Deployment.WindowsInstaller;
using Xunit;

namespace CustomActions.Tests.UserCustomActions
{
    public class UserCustomActionsDomainControllerTests : BaseUserCustomActionsDomainTests
    {
        public UserCustomActionsDomainControllerTests()
        {
            Test.WithDomainController();
        }

        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_Fails_With_No_Credentials_On_DomainController()
        {
            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Failure);

            Test.Properties.Should()
                .OnlyContain(kvp => kvp.Key == "DDAGENTUSER_FOUND" && kvp.Value == "false");
        }

        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_Succeeds_With_Existing_DomainUser_On_Domain_Controllers(
            string ddAgentUserName,
            string ddAgentUserPassword)
        {
            Test.WithDomainUser(ddAgentUserName);

            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns($"{Domain}\\{ddAgentUserName}");
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
                .NotContainKey("DDAGENTUSER_RESET_PASSWORD").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && !string.IsNullOrEmpty(kvp.Value));
        }

        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_Succeeds_With_gMsaAccount_On_Domain_Controllers(
            string ddAgentUserName)
        {
            Test.WithManagedServiceAccount(ddAgentUserName);

            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns($"{Domain}\\{ddAgentUserName}");
            // Note: no password specified

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
                .NotContainKey("DDAGENTUSER_RESET_PASSWORD").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" &&
                                // !! Password should be null
                                string.IsNullOrEmpty(kvp.Value));
        }

        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_Succeeds_With_gMsaAccount_And_Password_On_Domain_Controllers(
            string ddAgentUserName,
            string ddAgentUserPassword)
        {
            Test.WithManagedServiceAccount(ddAgentUserName);

            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns($"{Domain}\\{ddAgentUserName}");
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
                .NotContainKey("DDAGENTUSER_RESET_PASSWORD").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" &&
                                // !! Password should be null
                                string.IsNullOrEmpty(kvp.Value));
        }

        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_Succeeds_On_Upgrade_With_No_Credentials_But_Services_Exists_On_DomainController(
            string ddAgentUserName
        )
        {
            Test.WithDomainUser(ddAgentUserName)
                .WithDatadogAgentService();

            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns($"{Domain}\\{ddAgentUserName}");

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
                .NotContainKey("DDAGENTUSER_RESET_PASSWORD").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" &&
                                // !! Password should be null
                                string.IsNullOrEmpty(kvp.Value));
        }

        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_Fails_With_No_Credentials_And_Services_Do_Not_Exists_On_DomainController(
            string ddAgentUserName
        )
        {
            Test.WithDomainUser(ddAgentUserName);

            Test.Session
                // On upgrade this comes from the registry
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns($"{Domain}\\{ddAgentUserName}");

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Failure);

            // The install will proceed with the default `ddagentuser` in the machine domain
            Test.Properties.Should()
                .OnlyContain(kvp => (kvp.Key == "DDAGENTUSER_FOUND" && kvp.Value == "true") ||
                                     kvp.Key == "DDAGENTUSER_SID");
        }

        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_Fails_With_No_Credentials_But_Services_Exists_On_DomainController()
        {
            Test.WithDomainUser()
                .WithDatadogAgentService();

            // Maybe the registry was corrupted, or the services were installed
            // by another means than the installer. In any case, the DDAGENTUSER_NAME
            // is not present.

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Failure);

            // The install will proceed with the default `ddagentuser` in the machine domain
            Test.Properties.Should()
                .OnlyContain(kvp => kvp.Key == "DDAGENTUSER_FOUND" && kvp.Value == "false");
        }

        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_Fails_With_gMsaAccount_Missing_Domain_Part_On_DomainController(
            string ddAgentUserName)
        {
            Test.WithManagedServiceAccount(ddAgentUserName);

            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns($"\\{ddAgentUserName}");

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Failure);

            Test.Properties.Should()
                .OnlyContain(kvp => kvp.Key == "DDAGENTUSER_FOUND" && kvp.Value == "false");
        }
    }
}
