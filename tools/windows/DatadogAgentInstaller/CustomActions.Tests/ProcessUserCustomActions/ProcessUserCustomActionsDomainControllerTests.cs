using AutoFixture.Xunit2;
using FluentAssertions;
using Microsoft.Deployment.WindowsInstaller;
using Xunit;

namespace CustomActions.Tests.ProcessUserCustomActions
{
    public class ProcessUserCustomActionsDomainControllerTests : BaseProcessUserCustomActionsDomainTests
    {
        public ProcessUserCustomActionsDomainControllerTests()
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
                .BeEmpty();
        }

        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_Succeeds_With_Creating_DomainUser_On_Domain_Controllers(
            string ddAgentUserName,
            string ddAgentUserPassword)
        {
            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns($"{Domain}\\{ddAgentUserName}");
            Test.Session
                .Setup(session => session["DDAGENTUSER_PASSWORD"]).Returns(ddAgentUserPassword);

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDAGENTUSER_FOUND", "false").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_SID" && string.IsNullOrEmpty(kvp.Value)).And
                .Contain("DDAGENTUSER_PROCESSED_NAME", ddAgentUserName).And
                .Contain("DDAGENTUSER_PROCESSED_DOMAIN", Domain).And
                .Contain("DDAGENTUSER_PROCESSED_FQ_NAME", $"{Domain}\\{ddAgentUserName}").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_RESET_PASSWORD" && string.IsNullOrEmpty(kvp.Value)).And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && kvp.Value == ddAgentUserPassword);
        }

        [Theory]
        [AutoData]
        public void ProcessDdAgentUserCredentials_Fails_With_Creating_DomainUser_On_ReadOnly_Domain_Controllers(
            string ddAgentUserName,
            string ddAgentUserPassword)
        {
            Test.WithReadOnlyDomainController();

            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns($"{Domain}\\{ddAgentUserName}");
            Test.Session
                .Setup(session => session["DDAGENTUSER_PASSWORD"]).Returns(ddAgentUserPassword);

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
        public void ProcessDdAgentUserCredentials_Fails_With_Creating_DomainUser_On_ReadOnly_Domain_Controllers_Sanity(
            string ddAgentUserName,
            string ddAgentUserPassword)
        {
            // Sanity check logic when IsDomainController() returns false and IsReadOnlyDomainController() returns true.
            // This should never happen but we should make sure we still treat the host as a read-only domain controller.
            Test.NativeMethods.Setup(n => n.IsDomainController()).Returns(false);
            Test.NativeMethods.Setup(n => n.IsReadOnlyDomainController()).Returns(true);

            Test.Session
                .Setup(session => session["DDAGENTUSER_NAME"]).Returns($"{Domain}\\{ddAgentUserName}");
            Test.Session
                .Setup(session => session["DDAGENTUSER_PASSWORD"]).Returns(ddAgentUserPassword);

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
        public void ProcessDdAgentUserCredentials_Succeeds_With_Existing_DomainUser_On_ReadOnly_Domain_Controllers(
            string ddAgentUserName,
            string ddAgentUserPassword)
        {
            Test.WithReadOnlyDomainController();
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
                .Contain(kvp => kvp.Key == "DDAGENTUSER_RESET_PASSWORD" && string.IsNullOrEmpty(kvp.Value)).And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && !string.IsNullOrEmpty(kvp.Value));
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
                .Contain(kvp => kvp.Key == "DDAGENTUSER_RESET_PASSWORD" && string.IsNullOrEmpty(kvp.Value)).And
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
                .Contain(kvp => kvp.Key == "DDAGENTUSER_RESET_PASSWORD" && string.IsNullOrEmpty(kvp.Value)).And
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
                .Contain(kvp => kvp.Key == "DDAGENTUSER_RESET_PASSWORD" && string.IsNullOrEmpty(kvp.Value)).And
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
                .Contain(kvp => kvp.Key == "DDAGENTUSER_RESET_PASSWORD" && string.IsNullOrEmpty(kvp.Value)).And
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

            // services don't exist so password is required
            Test.Properties.Should()
                .OnlyContain(kvp => (kvp.Key == "DDAGENTUSER_FOUND" && kvp.Value == "true") ||
                                    (kvp.Key == "DDAGENTUSER_SID" && !string.IsNullOrEmpty(kvp.Value)));
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

            // Domain controller requires username be present
            Test.Properties.Should()
                .BeEmpty();
        }
    }
}
