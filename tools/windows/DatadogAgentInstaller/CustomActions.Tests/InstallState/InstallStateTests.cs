using System;
using System.Security.Principal;
using AutoFixture.Xunit2;
using Datadog.CustomActions;
using Datadog.CustomActions.Native;
using FluentAssertions;
using WixToolset.Dtf.WindowsInstaller;
using Xunit;

namespace CustomActions.Tests.InstallState
{
    public class InstallStateTests
    {
        public InstallStateTestSetup Test { get; } = new();

        [Theory]
        [AutoData]
        public void ReadInstallState_Default_Values()
        {
            Test.Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .NotContainKeys(
                    "DDAGENTUSER_NAME",
                    "DDAGENT_installedDomain",
                    "DDAgent_installedUser",
                    "PROJECTLOCATION",
                    "APPLICATIONDATADIRECTORY",
                    "DDAGENT_WINDOWSBUILD").And
                .Contain("DDDRIVERROLLBACK_NPM", "1").And
                .Contain("DDDRIVERROLLBACK_PROCMON", "1");
        }

        [Theory]
        [AutoData]
        public void ReadInstallState_Can_Read_Registry_Keys()
        {
            Test.WithRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey, new()
            {
                ["installedDomain"] = "testDomain",
                ["installedUser"] = "testUser",
                ["InstallPath"] = @"C:\datadog",
                ["ConfigRoot"] = @"D:\data"
            })
                .WithAgentServiceAccount(@"testDomain\testUser")
                .WithRegistryKey(Registries.LocalMachine, @"Software\Microsoft\Windows NT\CurrentVersion", new()
                {
                    ["CurrentBuild"] = "z_1234567890",
                })
                .Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDAGENTUSER_NAME", @"testDomain\testUser").And
                .Contain("DDAGENT_installedDomain", "testDomain").And
                .Contain("DDAGENT_installedUser", "testUser").And
                .Contain("PROJECTLOCATION", @"C:\datadog").And
                .Contain("APPLICATIONDATADIRECTORY", @"D:\data").And
                .Contain("DDAGENT_WINDOWSBUILD", "z_1234567890");
        }

        [Fact]
        public void ReadInstallState_Leaves_Account_Unset_When_Service_Runs_As_Another_Account()
        {
            Test.WithRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey, new()
                {
                    ["installedDomain"] = "HOST",
                    ["installedUser"] = "Administrator",
                })
                .WithAgentServiceAccount(@"HOST\svcmon")
                .WithAccount(@"HOST\Administrator", "HOST", "Administrator")
                .WithAccount(@"HOST\svcmon", "HOST", "svcmon")
                .Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .NotContainKey("DDAGENTUSER_NAME").And
                .Contain("DDAGENTUSER_STORED_MISMATCH", "true").And
                .Contain("DDAGENT_installedDomain", "HOST").And
                .Contain("DDAGENT_installedUser", "Administrator");
        }

        [Fact]
        public void ReadInstallState_Leaves_Account_Unset_When_Stored_Account_Does_Not_Resolve()
        {
            Test.WithRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey, new()
                {
                    ["installedDomain"] = "HOST",
                    ["installedUser"] = "missing",
                })
                .WithAgentServiceAccount(@"HOST\svcmon")
                .WithAccount(@"HOST\svcmon", "HOST", "svcmon")
                .Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .NotContainKey("DDAGENTUSER_NAME").And
                .Contain("DDAGENTUSER_STORED_MISMATCH", "true");
        }

        [Fact]
        public void ReadInstallState_Uses_Default_Account_When_Service_Runs_As_It()
        {
            Test.WithRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey, new()
                {
                    ["installedDomain"] = "HOST",
                    ["installedUser"] = "Administrator",
                })
                .WithAgentServiceAccount(@".\ddagentuser")
                .WithAccount(@"HOST\Administrator", "HOST", "Administrator")
                .Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDAGENTUSER_NAME", $"{Environment.MachineName}\\ddagentuser").And
                .NotContainKey("DDAGENTUSER_STORED_MISMATCH");
        }

        [Fact]
        public void ReadInstallState_Leaves_Account_Unset_When_Service_Account_Cannot_Be_Read()
        {
            Test.WithRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey, new()
                {
                    ["installedDomain"] = "HOST",
                    ["installedUser"] = "Administrator",
                });
            Test.RegistryServices
                .Setup(r => r.OpenRegistryKey(Registries.LocalMachine,
                    $@"SYSTEM\CurrentControlSet\Services\{Constants.AgentServiceName}"))
                .Throws(new System.UnauthorizedAccessException());

            Test.Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .NotContainKey("DDAGENTUSER_NAME").And
                .Contain("DDAGENTUSER_STORED_MISMATCH", "true");
        }

        [Fact]
        public void ReadInstallState_Leaves_Account_Unset_When_Service_Is_Not_Installed()
        {
            Test.WithRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey, new()
                {
                    ["installedDomain"] = "HOST",
                    ["installedUser"] = "Administrator",
                })
                .Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .NotContainKey("DDAGENTUSER_NAME").And
                .Contain("DDAGENTUSER_STORED_MISMATCH", "true");
        }

        [Fact]
        public void GetPreviousAgentUser_Ignores_Stored_Well_Known_Group()
        {
            Test.WithRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey, new()
                {
                    ["installedDomain"] = "NT AUTHORITY",
                    ["installedUser"] = "INTERACTIVE",
                })
                .WithAccount(@"NT AUTHORITY\INTERACTIVE", "NT AUTHORITY", "INTERACTIVE",
                    new SecurityIdentifier(WellKnownSidType.InteractiveSid, null), SID_NAME_USE.SidTypeWellKnownGroup);

            InstallStateCustomActions.GetPreviousAgentUser(Test.Session.Object, Test.RegistryServices.Object,
                    Test.NativeMethods.Object)
                .Should().BeNull();
        }

        [Fact]
        public void GetPreviousAgentUser_Ignores_Stored_Group()
        {
            Test.WithRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey, new()
                {
                    ["installedDomain"] = "BUILTIN",
                    ["installedUser"] = "Administrators",
                })
                .WithAccount(@"BUILTIN\Administrators", "BUILTIN", "Administrators",
                    new SecurityIdentifier(WellKnownSidType.BuiltinAdministratorsSid, null), SID_NAME_USE.SidTypeAlias);

            InstallStateCustomActions.GetPreviousAgentUser(Test.Session.Object, Test.RegistryServices.Object,
                    Test.NativeMethods.Object)
                .Should().BeNull();
        }

        /// <summary>
        /// Stored names of built-in accounts are localized, the service manager's are not.
        /// </summary>
        [Fact]
        public void ReadInstallState_Keeps_Localized_Built_In_Account_When_Service_Runs_As_It()
        {
            var systemSid = new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null);
            Test.WithRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey, new()
                {
                    ["installedDomain"] = "NT-AUTORITÄT",
                    ["installedUser"] = "SYSTEM",
                })
                .WithAgentServiceAccount("LocalSystem")
                .WithAccount(@"NT-AUTORITÄT\SYSTEM", "NT-AUTORITÄT", "SYSTEM", systemSid)
                .Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDAGENTUSER_NAME", @"NT-AUTORITÄT\SYSTEM").And
                .NotContainKey("DDAGENTUSER_STORED_MISMATCH");
        }

        [Theory]
        [InlineData("HOST", "ddagentuser", @"host\DDAGENTUSER")]
        [InlineData("NT AUTHORITY", "SYSTEM", "LocalSystem")]
        [InlineData("NT AUTHORITY", "LOCAL SERVICE", @"NT AUTHORITY\LocalService")]
        [InlineData("NT AUTHORITY", "NETWORK SERVICE", @"NT AUTHORITY\NetworkService")]
        [InlineData("NT AUTHORITY", "LOCAL SERVICE", "LocalService")]
        [InlineData("NT AUTHORITY", "NETWORK SERVICE", "NetworkService")]
        [InlineData("CONTOSO", "ddagentuser", "ddagentuser@contoso.com")]
        public void ReadInstallState_Keeps_Stored_Account_When_Service_Runs_As_It(string domain, string user,
            string serviceAccount)
        {
            Test.WithRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey, new()
                {
                    ["installedDomain"] = domain,
                    ["installedUser"] = user,
                })
                .WithAgentServiceAccount(serviceAccount)
                .WithAccount($"{domain}\\{user}", domain, user)
                .WithAccount("ddagentuser@contoso.com", "CONTOSO", "ddagentuser")
                .Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDAGENTUSER_NAME", $"{domain}\\{user}").And
                .NotContainKey("DDAGENTUSER_STORED_MISMATCH");
        }

        [Fact]
        public void ReadInstallState_Keeps_Stored_Account_When_Service_Uses_Local_Machine_Alias()
        {
            var machine = Environment.MachineName;
            Test.WithRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey, new()
                {
                    ["installedDomain"] = machine,
                    ["installedUser"] = "ddagentuser",
                })
                .WithAgentServiceAccount(@".\ddagentuser")
                .WithAccount($"{machine}\\ddagentuser", machine, "ddagentuser")
                .Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDAGENTUSER_NAME", $"{machine}\\ddagentuser").And
                .NotContainKey("DDAGENTUSER_STORED_MISMATCH");
        }

        [Fact]
        public void ReadInstallState_Command_Line_Account_Takes_Precedence_Over_Service_Account()
        {
            Test.Session.Setup(session => session["DDAGENTUSER_NAME"]).Returns(@"HOST\custom");
            Test.WithRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey, new()
                {
                    ["installedDomain"] = "HOST",
                    ["installedUser"] = "ddagentuser",
                })
                .WithAgentServiceAccount(@"HOST\other")
                .Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should().NotContainKey("DDAGENTUSER_NAME");
        }

        [Theory]
        [InlineData("7.53", "", "")]
        [InlineData("6.52", "", "")]
        [InlineData("7.51", "", "1")]
        [InlineData("6.58", "1", "1")]
        [InlineData("7.56", "1", "1")]
        public void ReadDD_Driver_Rollback_Upgrade(string version, string NPMExpectedRollback, string procmonExopectedRollback)
        {
            var productCode = "{123-456-789}";
            Test.Session.Setup(session => session["WIX_UPGRADE_DETECTED"]).Returns(productCode);
            Test.NativeMethods.Setup(n => n.GetVersionString(productCode)).Returns(version);

            Test.Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDDRIVERROLLBACK_NPM", NPMExpectedRollback).And
                .Contain("DDDRIVERROLLBACK_PROCMON", procmonExopectedRollback);
        }

        // This test covers the scenario where we fail to read the version we are upgrading from.
        // This shouldn't normally happen, we've seen the following cases:
        // - WIX_UGPRADE_DETECTED contains multiple product codes
        // - GetVersionString failes with "Unknown property"
        [Theory]
        [AutoData]
        public void ReadDD_Driver_Rollback_Multiple_Product_Codes()
        {
            var productCode = "{123-456-789};{123-456-790}";
            Test.Session.Setup(session => session["WIX_UPGRADE_DETECTED"]).Returns(productCode);
            Test.NativeMethods.Setup(n => n.GetVersionString(productCode)).Throws(new System.Exception("GetVersionString failed"));

            Test.Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDDRIVERROLLBACK_NPM", "1").And
                .Contain("DDDRIVERROLLBACK_PROCMON", "1");
        }

    }
}
