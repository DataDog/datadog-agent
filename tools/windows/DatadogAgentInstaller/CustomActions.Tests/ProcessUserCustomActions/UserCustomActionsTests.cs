using System;
using System.Security.Principal;
using AutoFixture.Xunit2;
using Datadog.CustomActions.Native;
using FluentAssertions;
using WixToolset.Dtf.WindowsInstaller;
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

        [Fact]
        public void ProcessDdAgentUserCredentials_Reads_Scm_Password_When_Lsa_Missing_And_Account_Unchanged()
        {
            const string ddAgentUserName = "ddagentuser";
            const string domain = "EXAMPLE";

            var (agentPasswordKey, scmPasswordKey) = SetupUpgradePasswordFetch(
                requestedAccountName: $"{domain}\\{ddAgentUserName}",
                installedDomain: domain,
                installedUser: ddAgentUserName,
                configureDomainClient: () => Test.WithDomainClient(domain).WithDomainUser(ddAgentUserName),
                lsaFetchError: new Exception("not found"));

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && kvp.Value == "scm-password");
            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.FetchSecret(scmPasswordKey),
                Times.Once);
            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.FetchSecret(agentPasswordKey),
                Times.Once);
        }

        [Fact]
        public void ProcessDdAgentUserCredentials_Prefers_Lsa_Password_Over_Scm_On_Upgrade()
        {
            const string ddAgentUserName = "ddagentuser";
            const string domain = "EXAMPLE";

            var (agentPasswordKey, scmPasswordKey) = SetupUpgradePasswordFetch(
                requestedAccountName: $"{domain}\\{ddAgentUserName}",
                installedDomain: domain,
                installedUser: ddAgentUserName,
                configureDomainClient: () => Test.WithDomainClient(domain).WithDomainUser(ddAgentUserName),
                lsaPassword: "lsa-password");

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && kvp.Value == "lsa-password");
            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.FetchSecret(agentPasswordKey),
                Times.Once);
            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.FetchSecret(scmPasswordKey),
                Times.Never);
        }

        [Fact]
        public void ProcessDdAgentUserCredentials_Skips_Scm_Password_When_Agent_Service_Missing()
        {
            const string ddAgentUserName = "ddagentuser";
            var machineDomain = Environment.MachineName;

            var (agentPasswordKey, scmPasswordKey) = SetupUpgradePasswordFetch(
                requestedAccountName: $"{machineDomain}\\{ddAgentUserName}",
                installedDomain: machineDomain,
                installedUser: ddAgentUserName,
                configureDomainClient: () => Test.WithLocalUser(machineDomain, ddAgentUserName),
                lsaFetchError: new Exception("not found"),
                agentServiceExists: false);

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            // Local accounts with no fetched password get a newly generated one; the assertion
            // here is that SCM was not consulted when the agent service is absent.
            Test.Properties.Should()
                .Contain("DDAGENTUSER_RESET_PASSWORD", "yes").And
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && !string.IsNullOrEmpty(kvp.Value));
            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.FetchSecret(scmPasswordKey),
                Times.Never);
            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.FetchSecret(agentPasswordKey),
                Times.Once);
        }

        [Fact]
        public void ProcessDdAgentUserCredentials_Reads_Scm_Password_When_Domain_Alias_Matches_Installed_Account()
        {
            const string ddAgentUserName = "ddagentuser";
            var machineDomain = Environment.MachineName;

            var (agentPasswordKey, scmPasswordKey) = SetupUpgradePasswordFetch(
                requestedAccountName: $".\\{ddAgentUserName}",
                installedDomain: machineDomain,
                installedUser: ddAgentUserName,
                configureDomainClient: () => Test.WithLocalUser(machineDomain, ddAgentUserName),
                lsaFetchError: new Exception("not found"));

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && kvp.Value == "scm-password");
            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.FetchSecret(scmPasswordKey),
                Times.Once);
            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.FetchSecret(agentPasswordKey),
                Times.Once);
        }

        [Fact]
        public void ProcessDdAgentUserCredentials_Reads_Scm_Password_When_Requested_Account_Alias_Matches_Installed_Sid()
        {
            const string ddAgentUserName = "ddagentuser";
            const string netBiosDomain = "EXAMPLE";
            const string dnsDomain = "example.com";
            var userSid = new SecurityIdentifier("S-1-5-21-0000000000-0000000000-0000000000-1001");

            var (agentPasswordKey, scmPasswordKey) = SetupUpgradePasswordFetch(
                requestedAccountName: $"{dnsDomain}\\{ddAgentUserName}",
                installedDomain: netBiosDomain,
                installedUser: ddAgentUserName,
                configureDomainClient: () =>
                {
                    Test.WithDomainClient(netBiosDomain);
                    Test.WithLookupAccountName($"{dnsDomain}\\{ddAgentUserName}", ddAgentUserName, dnsDomain, userSid);
                    Test.WithLookupAccountName($"{netBiosDomain}\\{ddAgentUserName}", ddAgentUserName, netBiosDomain, userSid);
                },
                lsaFetchError: new Exception("not found"));

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && kvp.Value == "scm-password");
            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.FetchSecret(scmPasswordKey),
                Times.Once);
            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.FetchSecret(agentPasswordKey),
                Times.Once);
        }

        [Fact]
        public void ProcessDdAgentUserCredentials_Skips_Scm_Password_When_Installed_Account_Differs()
        {
            const string ddAgentUserName = "newuser";
            const string oldAgentUserName = "olduser";
            const string domain = "EXAMPLE";

            var (agentPasswordKey, scmPasswordKey) = SetupUpgradePasswordFetch(
                requestedAccountName: $"{domain}\\{ddAgentUserName}",
                installedDomain: domain,
                installedUser: oldAgentUserName,
                configureDomainClient: () => Test.WithDomainClient(domain).WithDomainUser(ddAgentUserName),
                lsaFetchError: new Exception("not found"));

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && string.IsNullOrEmpty(kvp.Value));
            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.FetchSecret(scmPasswordKey),
                Times.Never);
            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.FetchSecret(agentPasswordKey),
                Times.Once);
        }

        [Fact]
        public void ProcessDdAgentUserCredentials_Skips_Scm_Password_When_Registry_Metadata_Missing_And_Account_Requested()
        {
            const string ddAgentUserName = "ddagentuser";
            const string domain = "EXAMPLE";

            var (agentPasswordKey, scmPasswordKey) = SetupUpgradePasswordFetch(
                requestedAccountName: $"{domain}\\{ddAgentUserName}",
                configureDomainClient: () => Test.WithDomainClient(domain).WithDomainUser(ddAgentUserName),
                lsaFetchError: new Exception("not found"));

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && string.IsNullOrEmpty(kvp.Value));
            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.FetchSecret(scmPasswordKey),
                Times.Never);
            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.FetchSecret(agentPasswordKey),
                Times.Once);
        }

        [Fact]
        public void ProcessDdAgentUserCredentials_Reads_Scm_Password_When_Registry_Metadata_And_Account_Name_Missing()
        {
            const string ddAgentUserName = "ddagentuser";
            var machineDomain = Environment.MachineName;

            var (agentPasswordKey, scmPasswordKey) = SetupUpgradePasswordFetch(
                requestedAccountName: null,
                configureDomainClient: () => Test.WithLocalUser(machineDomain, ddAgentUserName),
                lsaFetchError: new Exception("not found"));

            Test.Create()
                .ProcessDdAgentUserCredentials()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain(kvp => kvp.Key == "DDAGENTUSER_PROCESSED_PASSWORD" && kvp.Value == "scm-password");
            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.FetchSecret(scmPasswordKey),
                Times.Once);
            Test.NativeMethods.Verify(
                nativeMethods => nativeMethods.FetchSecret(agentPasswordKey),
                Times.Once);
        }

        private (string agentPasswordKey, string scmPasswordKey) SetupUpgradePasswordFetch(
            string requestedAccountName,
            Action configureDomainClient,
            string installedDomain = null,
            string installedUser = null,
            string lsaPassword = null,
            Exception lsaFetchError = null,
            bool agentServiceExists = true,
            string scmPassword = "scm-password")
        {
            var agentPasswordKey = Datadog.CustomActions.ConfigureUserCustomActions.AgentPasswordPrivateDataKey();
            var scmPasswordKey = $"_SC_{Datadog.CustomActions.Constants.AgentServiceName}";

            configureDomainClient();
            if (agentServiceExists)
            {
                Test.WithDatadogAgentService();
            }

            if (requestedAccountName != null)
            {
                Test.Session
                    .Setup(session => session["DDAGENTUSER_NAME"]).Returns(requestedAccountName);
            }

            if (installedDomain != null)
            {
                Test.Session
                    .Setup(session => session["DDAGENT_installedDomain"]).Returns(installedDomain);
            }

            if (installedUser != null)
            {
                Test.Session
                    .Setup(session => session["DDAGENT_installedUser"]).Returns(installedUser);
            }

            var lsaSetup = Test.NativeMethods
                .Setup(nativeMethods => nativeMethods.FetchSecret(agentPasswordKey));
            if (lsaFetchError != null)
            {
                lsaSetup.Throws(lsaFetchError);
            }
            else
            {
                lsaSetup.Returns(lsaPassword);
            }

            Test.NativeMethods
                .Setup(nativeMethods => nativeMethods.FetchSecret(scmPasswordKey))
                .Returns(scmPassword);

            return (agentPasswordKey, scmPasswordKey);
        }
    }
}
